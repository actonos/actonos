package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

// ExecuteStep runs a single cognitive iteration of the ReAct state machine.
func (e *Engine) ExecuteStep(ctx context.Context, agentID string, userMessage string) (*llm.Response, error) {
	return e.ExecuteStepWithHistory(ctx, agentID, userMessage, nil)
}

// maxAutonomousPlanStepsPerPulse is how many dependency-ready DAG steps a
// single heartbeat/chat turn may drain before yielding. Steps are still
// persisted after each one; approval, failure, or an empty turn stop the drain.
const maxAutonomousPlanStepsPerPulse = 8

const planStepNoHandoffNudge = "No operator is waiting. Do not ask permission and do not offer the next step. Execute the current plan step now with tools and produce its deliverable."

// ExecuteAutonomousGoal drains dependency-ready plan steps until the DAG is
// done, a step needs approval, a step fails, or the per-pulse cap is hit.
func (e *Engine) ExecuteAutonomousGoal(ctx context.Context, agentID, goal string, history []llm.Message) (*llm.Response, error) {
	if e.planner == nil {
		return e.ExecuteStepWithHistory(ctx, agentID, goal, history)
	}
	taskID, _ := ctx.Value("task_id").(string)
	if taskID == "" || e.taskMgr == nil {
		return e.ExecuteStepWithHistory(ctx, agentID, goal, history)
	}
	task, err := e.taskMgr.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return e.ExecuteStepWithHistory(ctx, agentID, goal, history)
	}

	var last *llm.Response
	plan := task.Plan
	sameStepRetries := 0
	for i := 0; i < maxAutonomousPlanStepsPerPulse; i++ {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		var readyID string
		if plan != nil {
			if ready, _ := e.planner.NextReadyStep(plan); ready != nil {
				readyID = ready.ID
			}
		}
		resp, nextPlan, stepErr := e.ExecuteBurstPlanSteps(ctx, agentID, goal, plan, history)
		if nextPlan != nil {
			plan = nextPlan
			e.writeTaskPlan(ctx, task, plan)
			task.Plan = plan
		}
		last = resp
		if stepErr != nil {
			return resp, stepErr
		}
		if plan != nil && plan.AllStepsCompleted() {
			return resp, nil
		}
		if resp != nil && IsCannedOrEmptyCompletion(resp.Content) {
			return resp, nil
		}
		if e.planner == nil || plan == nil {
			return resp, nil
		}
		ready, readyErr := e.planner.NextReadyStep(plan)
		if readyErr != nil {
			return resp, readyErr
		}
		if ready == nil {
			return resp, nil
		}
		if resp != nil && strings.TrimSpace(resp.Content) != "" {
			history = append(history, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		}
		if readyID != "" && ready.ID == readyID {
			sameStepRetries++
			if sameStepRetries > 1 {
				return resp, nil
			}
			history = append(history, llm.Message{Role: llm.RoleUser, Content: planStepNoHandoffNudge})
			continue
		}
		sameStepRetries = 0
	}
	return last, nil
}

// ExecuteBurstPlanSteps runs ready plan steps, executing up to 3 independent steps concurrently if multiple steps are ready.
func (e *Engine) ExecuteBurstPlanSteps(ctx context.Context, agentID, goal string, plan *TaskPlan, history []llm.Message) (*llm.Response, *TaskPlan, error) {
	if e.planner == nil {
		resp, err := e.ExecuteStepWithHistory(ctx, agentID, goal, history)
		return resp, plan, err
	}
	ctx = e.withSkillCatalogForID(ctx, agentID)
	if plan == nil || len(plan.Steps) == 0 {
		agents, err := e.agentMgr.List(ctx)
		if err != nil {
			return nil, plan, fmt.Errorf("listing agents for planning: %w", err)
		}
		var modelCascade []string
		if agent, getErr := e.agentMgr.Get(ctx, agentID); getErr == nil && agent != nil {
			if agent.ModelConfig.PrimaryModel != "" {
				modelCascade = append(modelCascade, agent.ModelConfig.PrimaryModel)
			}
			if agent.ModelConfig.FallbackModel != "" {
				modelCascade = append(modelCascade, agent.ModelConfig.FallbackModel)
			}
		}
		built, err := e.planner.DecomposeGoal(ctx, goal, agents, modelCascade...)
		if err != nil {
			return nil, plan, fmt.Errorf("decomposing autonomous goal: %w", err)
		}
		plan = built
		e.persistTaskPlan(ctx, plan)
	}

	readySteps, err := e.planner.ReadySteps(plan)
	if err != nil {
		return nil, plan, err
	}
	if len(readySteps) == 0 && plan.reopenApprovalFailedSteps() {
		e.persistTaskPlan(ctx, plan)
		readySteps, err = e.planner.ReadySteps(plan)
		if err != nil {
			return nil, plan, err
		}
	}
	if len(readySteps) == 0 {
		if plan.AllStepsCompleted() {
			return &llm.Response{Content: plan.CompletionSummary()}, plan, nil
		}
		return &llm.Response{Content: "Plan cannot advance; remaining steps are blocked on unfinished or failed dependencies."}, plan, nil
	}

	if len(readySteps) == 1 {
		return e.ExecuteNextPlanStep(ctx, agentID, goal, plan, history)
	}

	maxBurstConcurrency := 3
	if e.agentMgr != nil {
		if ag, err := e.agentMgr.Get(ctx, agentID); err == nil && ag != nil && ag.DelegationScope.MaxConcurrentRuns > 0 {
			maxBurstConcurrency = ag.DelegationScope.MaxConcurrentRuns
		}
	}
	numToRun := len(readySteps)
	if numToRun > maxBurstConcurrency {
		numToRun = maxBurstConcurrency
	}
	if numToRun <= 1 {
		return e.ExecuteNextPlanStep(ctx, agentID, goal, plan, history)
	}
	batch := readySteps[:numToRun]

	type stepExecResult struct {
		step    *PlanStep
		resp    *llm.Response
		content string
		err     error
	}

	results := make([]stepExecResult, len(batch))
	var wg sync.WaitGroup

	for idx, s := range batch {
		wg.Add(1)
		go func(i int, st *PlanStep) {
			defer wg.Done()
			r, cont, stErr := e.executeIndividualPlanStep(ctx, agentID, goal, st, history)
			results[i] = stepExecResult{
				step:    st,
				resp:    r,
				content: cont,
				err:     stErr,
			}
		}(idx, s)
	}
	wg.Wait()

	var firstErr error
	var totalTokens int
	var combinedContent strings.Builder

	for _, res := range results {
		if res.resp != nil {
			totalTokens += res.resp.Usage.TotalTokens
			if res.resp.Content != "" {
				if combinedContent.Len() > 0 {
					combinedContent.WriteString("\n\n")
				}
				combinedContent.WriteString(fmt.Sprintf("[%s]: %s", res.step.ID, res.resp.Content))
			}
		}

		if res.err != nil {
			var approvalErr *tools.ApprovalRequiredError
			if errors.As(res.err, &approvalErr) {
				plan.MarkStep(res.step.ID, StepStatusPaused, approvalPausedMarker+" "+res.err.Error())
				if firstErr == nil {
					firstErr = res.err
				}
			} else if isTransientExecutionError(res.err) {
				plan.MarkStep(res.step.ID, StepStatusPending, "Transient error (will retry): "+res.err.Error())
				if firstErr == nil {
					firstErr = res.err
				}
			} else {
				plan.MarkStep(res.step.ID, StepStatusFailed, res.err.Error())
				if firstErr == nil {
					firstErr = res.err
				}
			}
			continue
		}

		if res.content != "" {
			plan.MarkStep(res.step.ID, "completed", res.content)
		}
	}

	e.persistTaskPlan(ctx, plan)

	combinedResp := &llm.Response{
		Content: combinedContent.String(),
		Usage:   llm.Usage{TotalTokens: totalTokens},
	}
	return combinedResp, plan, firstErr
}

// ExecuteNextPlanStep decomposes (once) and runs the next dependency-ready step.
func (e *Engine) ExecuteNextPlanStep(ctx context.Context, agentID, goal string, plan *TaskPlan, history []llm.Message) (*llm.Response, *TaskPlan, error) {
	if e.planner == nil {
		resp, err := e.ExecuteStepWithHistory(ctx, agentID, goal, history)
		return resp, plan, err
	}
	ctx = e.withSkillCatalogForID(ctx, agentID)
	if plan == nil || len(plan.Steps) == 0 {
		agents, err := e.agentMgr.List(ctx)
		if err != nil {
			return nil, plan, fmt.Errorf("listing agents for planning: %w", err)
		}
		var modelCascade []string
		if agent, getErr := e.agentMgr.Get(ctx, agentID); getErr == nil && agent != nil {
			if agent.ModelConfig.PrimaryModel != "" {
				modelCascade = append(modelCascade, agent.ModelConfig.PrimaryModel)
			}
			if agent.ModelConfig.FallbackModel != "" {
				modelCascade = append(modelCascade, agent.ModelConfig.FallbackModel)
			}
		}
		built, err := e.planner.DecomposeGoal(ctx, goal, agents, modelCascade...)
		if err != nil {
			return nil, plan, fmt.Errorf("decomposing autonomous goal: %w", err)
		}
		plan = built
		e.persistTaskPlan(ctx, plan)
	}
	step, err := e.planner.NextReadyStep(plan)
	if err != nil {
		return nil, plan, err
	}
	if step == nil && plan.reopenApprovalFailedSteps() {
		e.persistTaskPlan(ctx, plan)
		step, err = e.planner.NextReadyStep(plan)
		if err != nil {
			return nil, plan, err
		}
	}
	if step == nil {
		if plan.AllStepsCompleted() {
			return &llm.Response{Content: plan.CompletionSummary()}, plan, nil
		}
		// Deadlocked or failed dependencies: do not re-run the entire goal.
		return &llm.Response{Content: "Plan cannot advance; remaining steps are blocked on unfinished or failed dependencies."}, plan, nil
	}

	resp, content, execErr := e.executeIndividualPlanStep(ctx, agentID, goal, step, history)
	if execErr != nil {
		var approvalErr *tools.ApprovalRequiredError
		if errors.As(execErr, &approvalErr) {
			plan.MarkStep(step.ID, StepStatusPaused, approvalPausedMarker+" "+execErr.Error())
			e.persistTaskPlan(ctx, plan)
			return resp, plan, execErr
		}
		if isTransientExecutionError(execErr) {
			plan.MarkStep(step.ID, StepStatusPending, "Transient error (will retry): "+execErr.Error())
			e.persistTaskPlan(ctx, plan)
			return resp, plan, execErr
		}
		plan.MarkStep(step.ID, StepStatusFailed, execErr.Error())
		e.persistTaskPlan(ctx, plan)
		return resp, plan, execErr
	}
	if content == "" {
		// Leave the step pending so drain/next pulse retries it instead of rubber-stamping.
		e.persistTaskPlan(ctx, plan)
		return resp, plan, nil
	}
	plan.MarkStep(step.ID, "completed", content)
	e.persistTaskPlan(ctx, plan)
	return resp, plan, nil
}

func (e *Engine) executeIndividualPlanStep(ctx context.Context, agentID, goal string, step *PlanStep, history []llm.Message) (*llm.Response, string, error) {
	execAgentID := agentID
	if step.AgentRole != "" && step.AgentRole != "general" && e.agentMgr != nil {
		if _, lookupErr := e.agentMgr.Get(ctx, step.AgentRole); lookupErr == nil {
			execAgentID = step.AgentRole
		} else if e.swarm != nil {
			spawnTitle := step.Title
			if spawnTitle == "" {
				spawnTitle = step.ID
			}
			spawnPrompt := step.Description
			if step.Acceptance != "" {
				spawnPrompt = step.Description + "\nAcceptance: " + step.Acceptance
			}
			resultCh, spawnErr := e.swarm.SpawnSubAgent(ctx, agentID, SubTask{
				Title:           spawnTitle,
				Prompt:          spawnPrompt,
				AssignedAgentID: step.AgentRole,
			})
			if spawnErr == nil {
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case result := <-resultCh:
					return &llm.Response{Content: result.Output, Usage: llm.Usage{TotalTokens: result.TokensUsed}}, result.Output, nil
				}
			}
		}
	}
	stepBrief := step.Description
	if strings.TrimSpace(step.Title) != "" && !strings.EqualFold(strings.TrimSpace(step.Title), strings.TrimSpace(step.Description)) {
		stepBrief = step.Title + " — " + step.Description
	}
	prompt := BuildPlanStepPrompt(step.ID, goal, stepBrief, step.AgentRole, step.Acceptance, SkillCatalogFrom(ctx)...)
	resp, execErr := e.ExecuteStepWithHistory(ctx, execAgentID, prompt, history)
	if execErr != nil {
		return resp, "", execErr
	}
	content := ""
	if resp != nil {
		content = resp.Content
	}
	if !planStepShouldComplete(step, content, resp) {
		return resp, "", nil
	}
	return resp, content, nil
}

func planStepShouldComplete(step *PlanStep, content string, resp *llm.Response) bool {
	if IsCannedOrEmptyCompletion(content) {
		return false
	}
	var calls []llm.ToolCall
	if resp != nil {
		calls = resp.ToolCalls
	}
	wrote := HasDeliverableWrite(calls)
	if step != nil && step.requiresArtifact() && !wrote && len(calls) == 0 {
		return false
	}
	return strings.TrimSpace(content) != "" || wrote
}

// persistTaskPlan writes the durable DAG onto the mission identified by ctx task_id.
func (e *Engine) persistTaskPlan(ctx context.Context, plan *TaskPlan) {
	if e.taskMgr == nil || plan == nil {
		return
	}
	taskID, _ := ctx.Value("task_id").(string)
	if taskID == "" {
		return
	}
	task, err := e.taskMgr.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	e.writeTaskPlan(ctx, task, plan)
}

func (e *Engine) writeTaskPlan(ctx context.Context, task *AutonomousTask, plan *TaskPlan) {
	if e.taskMgr == nil || task == nil || plan == nil {
		return
	}
	task.Plan = plan
	if plan.AllStepsCompleted() {
		if task.Status != "cancelled" && task.Status != "blocked" {
			task.Status = "completed"
			task.Progress = 100
		}
	} else {
		if task.Status == "completed" {
			task.Status = "in_progress"
			task.CompletedAt = nil
		}
		if pct := plan.ProgressPercent(); pct > task.Progress {
			task.Progress = pct
		}
	}
	_ = e.taskMgr.UpdateTask(ctx, *task)
}

func (e *Engine) attachAutonomousPlan(ctx context.Context, messages []llm.Message, goal string, history []llm.Message) []llm.Message {
	if e.planner == nil || len(history) > 0 || !strings.Contains(goal, "[AUTONOMOUS") {
		return messages
	}
	agents, err := e.agentMgr.List(ctx)
	if err != nil {
		return messages
	}
	plan, err := e.planner.DecomposeGoal(ctx, goal, agents)
	if err != nil || plan == nil {
		return messages
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return messages
	}
	planMessage := llm.Message{
		Role:    llm.RoleSystem,
		Content: "Durable execution plan. Execute only dependency-ready steps and verify each result before advancing:\n" + string(encoded),
	}
	return append(messages[:len(messages)-1], planMessage, messages[len(messages)-1])
}

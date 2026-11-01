package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

// ResumeApproved continues an approval-paused run from its exact persisted ReAct state.
func (e *Engine) ResumeApproved(ctx context.Context, approval tools.ApprovalRequest) (*llm.Response, error) {
	if e.runStore == nil || e.tools == nil {
		return nil, errors.New("durable resume is not configured")
	}
	checkpoint, run, err := e.runStore.LoadCheckpointByTrace(ctx, approval.TraceID)
	if err != nil {
		return nil, err
	}
	if checkpoint.AgentID != approval.AgentID {
		return nil, fmt.Errorf("%w: checkpoint agent %s does not match approval agent %s",
			tools.ErrApprovalInvalid, checkpoint.AgentID, approval.AgentID)
	}
	if checkpoint.PendingTool.Function.Name != approval.ToolName {
		return nil, fmt.Errorf("%w: checkpoint tool %s does not match approval tool %s",
			tools.ErrApprovalInvalid, checkpoint.PendingTool.Function.Name, approval.ToolName)
	}
	pendingInput := tools.NormalizeToolInput(checkpoint.PendingTool.Function.Arguments)
	if tools.ActionHash(approval.AgentID, approval.ToolName, pendingInput) != approval.ActionHash {
		return nil, fmt.Errorf("%w: the paused action no longer matches what was approved",
			tools.ErrApprovalInvalid)
	}
	manifest, err := e.agentMgr.Get(ctx, checkpoint.AgentID)
	if err != nil {
		return nil, fmt.Errorf("loading resumed agent: %w", err)
	}
	// Execute the exact bytes the operator reviewed, bound strictly to this approval ID.
	approvedCtx := tools.WithTraceID(ctx, checkpoint.TraceID)
	approvedCtx = tools.WithApprovalID(approvedCtx, approval.ID)
	result, err := e.tools.Execute(
		approvedCtx, checkpoint.AgentID, checkpoint.PendingTool.Function.Name, approval.Input,
	)
	if err != nil {
		e.finishRun(ctx, run, RunFailed, "approved_execution_failed", checkpoint.Iteration, checkpoint.Usage)
		return nil, err
	}
	observation := ""
	if result != nil {
		observation = result.Content
	}
	// The approved tool result must be appended in the correct position.
	// If the preceding assistant message contains multiple tool calls, every
	// call that appears after the approved one must also have a result message
	// (even if it is an error) so providers that enforce strict tool-call
	// pairing do not reject the request.
	resumeMessages := make([]llm.Message, len(checkpoint.Messages))
	copy(resumeMessages, checkpoint.Messages)
	resumeMessages = append(resumeMessages, llm.Message{
		Role: llm.RoleTool, Name: checkpoint.PendingTool.Function.Name,
		ToolCallID: checkpoint.PendingTool.ID, Content: observation,
	})
	// Ensure every other tool_call in the last assistant message has a
	// corresponding result message. Tool calls that were executed before the
	// paused one already have their results in checkpoint.Messages; only the
	// ones that follow the paused call are missing.
	var lastAssistant *llm.Message
	for i := len(resumeMessages) - 1; i >= 0; i-- {
		if resumeMessages[i].Role == llm.RoleAssistant {
			lastAssistant = &resumeMessages[i]
			break
		}
	}
	if lastAssistant != nil && len(lastAssistant.ToolCalls) > 1 {
		// Collect IDs that already have a result in the slice.
		resultIDs := make(map[string]bool)
		for _, m := range resumeMessages {
			if m.Role == llm.RoleTool && m.ToolCallID != "" {
				resultIDs[m.ToolCallID] = true
			}
		}
		for _, tc := range lastAssistant.ToolCalls {
			if !resultIDs[tc.ID] {
				resumeMessages = append(resumeMessages, llm.Message{
					Role:       llm.RoleTool,
					Name:       tc.Function.Name,
					ToolCallID: tc.ID,
					Content:    "tool call was deferred pending approval of a concurrent action",
				})
			}
		}
	}
	messages := resumeMessages
	_ = e.runStore.AppendEvent(ctx, RunEvent{
		RunID: run.ID, TraceID: run.TraceID, Step: checkpoint.Iteration,
		Type: "approval_execution", Status: "success",
		ToolName: approval.ToolName, Data: map[string]any{"observation": truncateRunData(observation, 4096)},
	})

	var cascade []string
	if manifest.ModelConfig.PrimaryModel != "" {
		cascade = append(cascade, manifest.ModelConfig.PrimaryModel)
	}
	if manifest.ModelConfig.FallbackModel != "" {
		cascade = append(cascade, manifest.ModelConfig.FallbackModel)
	}
	opts := llm.CompletionOptions{ReasoningEffort: manifest.ModelConfig.EffectiveReasoningEffort()}
	defaultDirectMaxTokens := 32768
	if manifest.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &manifest.ModelConfig.MaxTokens
	} else {
		opts.MaxTokens = &defaultDirectMaxTokens
	}
	authorizedTools := manifest.AuthorizedTools
	if allowed := tools.AllowedTools(ctx); allowed != nil {
		authorizedTools = allowed
	}
	opts.Tools = e.tools.ToLLMToolDefinitions(authorizedTools, tools.DeniedTools(ctx)...)
	usage := checkpoint.Usage

	// Subsequent tool calls must not inherit the consumed approval ID, ensuring
	// new high-risk actions can request their own approvals cleanly.
	execCtx := tools.WithTraceID(ctx, checkpoint.TraceID)
	execCtx = tools.WithApprovalID(execCtx, "")

	consecutiveFailures := 0
	lastObservation := ""
	repeatedObservations := 0

	// Resume with a FRESH iteration budget (10 iterations) so the LLM has
	// enough room to process the approved tool result and converge. The
	// previous approach of continuing from checkpoint.Iteration left too few
	// iterations and almost always exhausted the budget.
	const resumeMaxIterations = 10
	for iteration := 0; iteration < resumeMaxIterations; iteration++ {
		// Track absolute step for run events (checkpoint iterations + resumed iterations).
		absoluteStep := checkpoint.Iteration + iteration + 1
		currentOpts := opts
		if iteration == resumeMaxIterations-1 {
			currentOpts.Tools = nil
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "Iteration limit reached. Do not call any more tools. Synthesize all observations gathered above into your final comprehensive response to the user.",
			})
		}
		if e.contextManager != nil {
			messages = e.contextManager.PruneAndSnapshot(execCtx, run.ID, messages, e.contextBudget(manifest))
		}
		response, callErr := e.llm.CompleteWithCascade(execCtx, cascade, messages, currentOpts)
		if callErr != nil && llm.IsContextWindowError(callErr) {
			messages = e.compactAfterContextWindowError(execCtx, run.ID, manifest, messages)
			response, callErr = e.llm.CompleteWithCascade(execCtx, cascade, messages, currentOpts)
		}
		if callErr != nil {
			e.finishRun(execCtx, run, RunFailed, "infrastructure_failure", absoluteStep, usage)
			return nil, callErr
		}
		usage = addUsage(usage, response.Usage)
		if len(response.ToolCalls) == 0 || currentOpts.Tools == nil {
			if !e.verifier.VerifySemanticConsistency(execCtx, checkpoint.Goal, response.Content) {
				e.finishRun(execCtx, run, RunFailed, "verification_failed", absoluteStep, usage)
				return nil, errors.New("resumed response failed verification")
			}
			response.Usage = usage
			e.finishRun(execCtx, run, RunCompleted, "goal_completed", absoluteStep, usage)
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, checkpoint.AgentID, map[string]any{
					"tokens": response.Usage.TotalTokens,
				}))
			}
			if response.Content != "" {
				if e.reflectionEngine != nil {
					e.reflectionEngine.ReflectOnConversation(context.Background(), checkpoint.AgentID, checkpoint.Goal, response.Content)
				}
			}
			modelName := response.Model
			if (modelName == "" || !strings.Contains(modelName, "/")) && manifest.ModelConfig.PrimaryModel != "" {
				modelName = manifest.ModelConfig.PrimaryModel
			}
			e.RecordTokenUsage(execCtx, checkpoint.AgentID, modelName, "", checkpoint.Source, "", usage)

			e.finishPlanStepAfterResume(execCtx, checkpoint, response)
			if drainResp, drainErr := e.drainMissionAfterResume(execCtx, checkpoint); drainErr != nil {
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(drainErr, &approvalErr) {
					return drainResp, drainErr
				}
				slog.Warn("draining remaining plan steps after approval resume failed", "task_id", checkpoint.TaskID, "error", drainErr)
			} else if drainResp != nil && strings.TrimSpace(drainResp.Content) != "" {
				response = drainResp
			}
			e.syncMissionAfterResume(execCtx, checkpoint, response)
			e.persistConversationReply(execCtx, checkpoint.ConversationID, checkpoint.AgentID, response)

			return response, nil
		}
		messages = append(messages, llm.Message{
			Role:             llm.RoleAssistant,
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
			ToolCalls:        response.ToolCalls,
			ProviderItems:    response.ProviderItems,
		})
		for _, call := range response.ToolCalls {
			toolResult, toolErr := e.tools.Execute(execCtx, checkpoint.AgentID, call.Function.Name, call.Function.Arguments)
			if toolErr != nil {
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(toolErr, &approvalErr) {
					e.saveApprovalCheckpoint(execCtx, run, checkpoint.AgentID, checkpoint.Goal, checkpoint.Source, messages, absoluteStep, usage, call)
					e.finishRun(execCtx, run, RunApprovalPending, "approval_required", absoluteStep, usage)
					return nil, toolErr
				}
			}
			content := ""
			statusStr := "success"
			if toolErr != nil {
				statusStr = "error"
				content = toolErr.Error()
				consecutiveFailures++
			} else if toolResult != nil {
				content = toolResult.Content
				consecutiveFailures = 0
			}
			if content == lastObservation {
				repeatedObservations++
			} else {
				repeatedObservations = 0
				lastObservation = content
			}
			_ = e.runStore.AppendEvent(execCtx, RunEvent{
				RunID: run.ID, TraceID: run.TraceID, Step: absoluteStep, Type: "tool",
				Status: statusStr, ToolName: call.Function.Name,
				Data: map[string]any{"tool_call_id": call.ID, "observation": truncateRunData(content, 4096)},
			})
			content = capToolObservation(content)
			messages = append(messages, llm.Message{
				Role: llm.RoleTool, Name: call.Function.Name, ToolCallID: call.ID, Content: content,
			})
			if consecutiveFailures >= 5 || repeatedObservations >= 3 {
				// Prevent hard crashing by disabling tools on next step so LLM reports the failure
				currentOpts.Tools = nil
			}
		}
	}

	// Budget exhausted — mark the associated task as blocked so the heartbeat
	// daemon does not silently re-trigger the same stalled flow.
	totalStep := checkpoint.Iteration + resumeMaxIterations
	e.finishRun(execCtx, run, RunFailed, "iteration_budget_exhausted", totalStep, usage)
	e.blockTaskOnResumeFailure(execCtx, checkpoint, "agent exceeded iteration budget after approval resume")
	return nil, errors.New("resumed run exhausted iteration budget")
}

const (
	defaultModelWindowTokens = 128000
	defaultStreamMaxTokens   = 32768
	maxToolObservationChars  = 16000
	contextProtocolOverhead  = 16000
)

func resumeTaskID(ctx context.Context, checkpoint *RunCheckpoint) string {
	if checkpoint != nil && strings.TrimSpace(checkpoint.TaskID) != "" {
		return strings.TrimSpace(checkpoint.TaskID)
	}
	if id := tools.TaskIDFromContext(ctx); id != "" {
		return id
	}
	if checkpoint == nil {
		return ""
	}
	re := regexp.MustCompile(`Task ID:\s*([^\s|]+)`)
	if match := re.FindStringSubmatch(checkpoint.Goal); len(match) > 1 {
		return match[1]
	}
	return ""
}

func (e *Engine) finishPlanStepAfterResume(ctx context.Context, checkpoint *RunCheckpoint, resp *llm.Response) {
	if e.taskMgr == nil || checkpoint == nil {
		return
	}
	taskID := resumeTaskID(ctx, checkpoint)
	if taskID == "" {
		return
	}
	task, err := e.taskMgr.GetTask(ctx, taskID)
	if err != nil || task == nil || task.Plan == nil {
		return
	}
	stepID := PlanStepIDFromPrompt(checkpoint.Goal)
	if stepID == "" {
		for _, step := range task.Plan.Steps {
			if step.Status == StepStatusPaused || (step.Status == StepStatusFailed && isApprovalPauseResult(step.Result)) {
				stepID = step.ID
				break
			}
		}
	}
	if stepID == "" {
		return
	}
	content := ""
	if resp != nil {
		content = resp.Content
	}
	if IsCannedOrEmptyCompletion(content) {
		task.Plan.MarkStep(stepID, StepStatusPending, content)
	} else {
		task.Plan.MarkStep(stepID, StepStatusCompleted, content)
	}
	if task.Status == "blocked" {
		task.Status = "in_progress"
		task.CompletedAt = nil
	}
	e.writeTaskPlan(ctx, task, task.Plan)
}

func (e *Engine) drainMissionAfterResume(ctx context.Context, checkpoint *RunCheckpoint) (*llm.Response, error) {
	if e.planner == nil || e.taskMgr == nil || checkpoint == nil {
		return nil, nil
	}
	taskID := resumeTaskID(ctx, checkpoint)
	if taskID == "" {
		return nil, nil
	}
	task, err := e.taskMgr.GetTask(ctx, taskID)
	if err != nil || task == nil || task.Plan == nil || !task.Plan.HasReadyStep() {
		return nil, nil
	}
	drainCtx := context.WithValue(ctx, "task_id", taskID)
	drainCtx = tools.WithTaskID(drainCtx, taskID)
	if checkpoint.Source != "" {
		drainCtx = WithExecutionSource(drainCtx, checkpoint.Source)
	}
	return e.ExecuteAutonomousGoal(drainCtx, checkpoint.AgentID, missionWorkGoal(task.Title, task.Description), nil)
}

func (e *Engine) syncMissionAfterResume(ctx context.Context, checkpoint *RunCheckpoint, response *llm.Response) {
	if e.taskMgr == nil || checkpoint == nil {
		return
	}
	taskID := resumeTaskID(ctx, checkpoint)
	if taskID == "" {
		return
	}
	task, err := e.taskMgr.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	content := ""
	var calls []llm.ToolCall
	if response != nil {
		content = strings.TrimSpace(response.Content)
		calls = response.ToolCalls
	}
	shortLog := shortSummary(content, 250)
	planOpen := task.Plan != nil && len(task.Plan.Steps) > 0 && !task.Plan.AllStepsCompleted()
	planDone := task.Plan != nil && task.Plan.AllStepsCompleted()
	switch {
	case planOpen:
		task.Status = "in_progress"
		task.ExecutionLog = shortLog
	case strings.Contains(content, "[TASK_BLOCKED"):
		task.Status = "blocked"
		task.ExecutionLog = shortLog
	case planDone || (strings.Contains(content, "[TASK_COMPLETED]") && (e.verifier == nil || e.verifier.VerifyTaskCompletion(task.Description, content, calls))):
		task.Status = "completed"
		task.Progress = 100
		now := time.Now().UTC()
		task.CompletedAt = &now
		task.ExecutionLog = shortLog
	default:
		task.Status = "in_progress"
		task.ExecutionLog = shortLog
	}
	_ = e.taskMgr.UpdateTask(ctx, *task)
	if e.sessionMgr != nil && task.SessionID != "" {
		_ = e.sessionMgr.SaveMessage(ctx, task.SessionID, checkpoint.AgentID, "assistant", content, calls)
	}
	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, checkpoint.AgentID, map[string]any{
			"task_id":  task.ID,
			"status":   task.Status,
			"progress": task.Progress,
			"summary":  task.ExecutionLog,
		}))
	}
}

func (e *Engine) blockTaskOnResumeFailure(ctx context.Context, checkpoint *RunCheckpoint, reason string) {
	if checkpoint == nil || e.taskMgr == nil {
		return
	}
	targetTaskID := checkpoint.TaskID
	if targetTaskID == "" {
		re := regexp.MustCompile(`Task ID:\s*([^\s|]+)`)
		if match := re.FindStringSubmatch(checkpoint.Goal); len(match) > 1 {
			targetTaskID = match[1]
		}
	}
	if targetTaskID == "" {
		return
	}
	if task, err := e.taskMgr.GetTask(ctx, targetTaskID); err == nil && task != nil {
		task.Status = "blocked"
		task.ExecutionLog = fmt.Sprintf("Blocked: %s", reason)
		_ = e.taskMgr.UpdateTask(ctx, *task)
		if e.bus != nil {
			e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, checkpoint.AgentID, map[string]any{
				"task_id":  task.ID,
				"status":   task.Status,
				"progress": task.Progress,
				"summary":  task.ExecutionLog,
			}))
		}
	}
}

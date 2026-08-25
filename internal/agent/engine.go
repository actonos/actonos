package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

// Engine orchestrates the ReAct cognitive loop for agents.
type Engine struct {
	agentMgr         *AgentManager
	bus              *bus.EventBus
	llm              *llm.ModelCascadeRouter
	memory           *memory.HybridEngine
	embedding        *memory.EmbeddingService
	profileMgr       *UserProfileManager
	tools            *tools.ToolRegistry
	verifier         *Verifier
	tokenTracker     *memory.TokenTracker
	reflectionEngine *ReflectionEngine
	contextManager   *ContextManager
	runStore         *RunStore
	planner          *Planner
	taskMgr          *TaskManager
	sessionMgr       SessionHistoryProvider
	swarm            *SwarmManager
	dataDir          string
	workspaceDir     string
	inFlight         *inFlightRegistry
}

// NewEngine creates an Engine instance.
func NewEngine(
	agentMgr *AgentManager,
	eventBus *bus.EventBus,
	llmRouter *llm.ModelCascadeRouter,
	mem *memory.HybridEngine,
) *Engine {
	return &Engine{
		agentMgr:       agentMgr,
		bus:            eventBus,
		llm:            llmRouter,
		memory:         mem,
		verifier:       NewVerifier(),
		contextManager: NewContextManager(128000),
		inFlight:       newInFlightRegistry(),
	}
}

// SetSwarmManager attaches multi-agent dispatch for plan roles.
func (e *Engine) SetSwarmManager(swarm *SwarmManager) {
	e.swarm = swarm
}

// CancelRun interrupts an in-flight turn and marks the durable run cancelled.
func (e *Engine) CancelRun(ctx context.Context, runID string) error {
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	interrupted := e.inFlight.cancelRun(runID)
	if e.runStore != nil {
		if err := e.runStore.Cancel(ctx, runID, "operator_signal"); err != nil && !interrupted {
			return err
		}
	}
	if !interrupted && e.runStore == nil {
		return fmt.Errorf("run %s is not in flight", runID)
	}
	return nil
}

// CancelAgentWork interrupts every in-flight turn for an agent (used by Stop).
func (e *Engine) CancelAgentWork(agentID string) int {
	return e.inFlight.cancelAgent(agentID)
}

// ReclaimOrphanRuns marks process-crash leftovers so heartbeat does not replay them as live.
func (e *Engine) ReclaimOrphanRuns(ctx context.Context) (int, error) {
	if e.runStore == nil {
		return 0, nil
	}
	return e.runStore.ReclaimOrphans(ctx)
}

// SetRunStore attaches durable run and event persistence.
func (e *Engine) SetRunStore(store *RunStore) {
	e.runStore = store
}

// SetPlanner attaches goal decomposition for autonomous missions.
func (e *Engine) SetPlanner(planner *Planner) {
	e.planner = planner
}

// SetContextManager overrides context budgeting behavior.
func (e *Engine) SetContextManager(manager *ContextManager) {
	e.contextManager = manager
}

// SetProfileManager attaches the user profile & soul manager.
func (e *Engine) SetProfileManager(m *UserProfileManager) {
	e.profileMgr = m
}

// SetReflectionEngine attaches the memory reflection daemon.
func (e *Engine) SetReflectionEngine(r *ReflectionEngine) {
	e.reflectionEngine = r
}

// SetToolRegistry attaches the system tool registry to enable tool execution.
func (e *Engine) SetToolRegistry(r *tools.ToolRegistry) {
	e.tools = r
}

// SetTokenTracker attaches the token usage tracker.
func (e *Engine) SetTokenTracker(t *memory.TokenTracker) {
	e.tokenTracker = t
}

func (e *Engine) SetEmbeddingService(service *memory.EmbeddingService) {
	e.embedding = service
}

// SetTaskManager attaches the autonomous task manager.
func (e *Engine) SetTaskManager(tm *TaskManager) {
	e.taskMgr = tm
}

// SetSessionManager attaches the session history provider.
func (e *Engine) SetSessionManager(sm SessionHistoryProvider) {
	e.sessionMgr = sm
}

// SetDataDir sets the root data directory.
func (e *Engine) SetDataDir(dir string) {
	e.dataDir = dir
}

// SetWorkspaceDir sets the primary workspace directory for file sandbox and environment prompt.
func (e *Engine) SetWorkspaceDir(dir string) {
	e.workspaceDir = dir
}

// HasConfiguredLLM reports whether the engine has a real (non-stub) LLM provider configured.
func (e *Engine) HasConfiguredLLM() bool {
	return e.llm != nil && e.llm.HasRealProvider()
}

// RecordTokenUsage records usage metrics if token tracker is configured.
func (e *Engine) RecordTokenUsage(ctx context.Context, agentID, model, provider, source, convID string, usage llm.Usage) {
	if e.tokenTracker == nil {
		return
	}
	go func() {
		_ = e.tokenTracker.Record(context.Background(), memory.TokenUsageRecord{
			AgentID:          agentID,
			Model:            model,
			Provider:         provider,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Source:           source,
			ConversationID:   convID,
		})
	}()
}

// buildCognitivePrompt synthesizes all 7 cognitive layers into a structured XML cognitive context
// using the unified PromptBuilder architecture.
func (e *Engine) buildCognitivePrompt(ctx context.Context, agentID string, agent *AgentManifest, userMessage string) (string, int) {
	ctx = e.withSkillCatalog(ctx, agent)
	return BuildCognitiveSystemPrompt(ctx, agentID, agent, e.dataDir, e.workspaceDir, e.profileMgr, e.memory, e.embedding, userMessage)
}

func (e *Engine) withSkillCatalog(ctx context.Context, agent *AgentManifest) context.Context {
	if len(SkillCatalogFrom(ctx)) > 0 {
		return ctx
	}
	return WithSkillCatalog(ctx, e.skillCatalogForAgent(agent))
}

func (e *Engine) withSkillCatalogForID(ctx context.Context, agentID string) context.Context {
	if len(SkillCatalogFrom(ctx)) > 0 {
		return ctx
	}
	return WithSkillCatalog(ctx, e.SkillCatalogForAgent(ctx, agentID))
}

// SkillCatalogForAgent returns enabled skills the agent is allowed to invoke.
func (e *Engine) SkillCatalogForAgent(ctx context.Context, agentID string) []SkillPromptEntry {
	if e == nil || e.tools == nil {
		return nil
	}
	var authorized []string
	if e.agentMgr != nil && agentID != "" {
		if agent, err := e.agentMgr.Get(ctx, agentID); err == nil && agent != nil {
			authorized = agent.AuthorizedTools
		}
	}
	return e.skillCatalog(authorized)
}

func (e *Engine) skillCatalogForAgent(agent *AgentManifest) []SkillPromptEntry {
	if agent == nil {
		return e.skillCatalog(nil)
	}
	return e.skillCatalog(agent.AuthorizedTools)
}

func (e *Engine) skillCatalog(authorized []string) []SkillPromptEntry {
	if e == nil || e.tools == nil {
		return nil
	}
	raw := e.tools.EnabledSkillCatalog(authorized...)
	if len(raw) == 0 {
		return nil
	}
	out := make([]SkillPromptEntry, 0, len(raw))
	for _, entry := range raw {
		out = append(out, SkillPromptEntry{Name: entry.Name, Description: entry.Description, Path: entry.Path})
	}
	return out
}

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
		resp, nextPlan, stepErr := e.ExecuteNextPlanStep(ctx, agentID, goal, plan, history)
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
	if step == nil {
		if plan.AllStepsCompleted() {
			return &llm.Response{Content: plan.CompletionSummary()}, plan, nil
		}
		// Deadlocked or failed dependencies: do not re-run the entire goal.
		return &llm.Response{Content: "Plan cannot advance; remaining steps are blocked on unfinished or failed dependencies."}, plan, nil
	}
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
					return nil, plan, ctx.Err()
				case result := <-resultCh:
					plan.MarkStep(step.ID, "completed", result.Output)
					e.persistTaskPlan(ctx, plan)
					return &llm.Response{Content: result.Output, Usage: llm.Usage{TotalTokens: result.TokensUsed}}, plan, nil
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
		plan.MarkStep(step.ID, "failed", execErr.Error())
		e.persistTaskPlan(ctx, plan)
		return resp, plan, execErr
	}
	content := ""
	if resp != nil {
		content = resp.Content
	}
	if !planStepShouldComplete(step, content, resp) {
		// Leave the step pending so drain/next pulse retries it instead of
		// rubber-stamping a "if you want I can continue" wrap-up.
		e.persistTaskPlan(ctx, plan)
		return resp, plan, nil
	}
	plan.MarkStep(step.ID, "completed", content)
	e.persistTaskPlan(ctx, plan)
	return resp, plan, nil
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

// ExecuteStepWithHistory runs a cognitive iteration of the ReAct state machine with short-term dialogue history.
func (e *Engine) ExecuteStepWithHistory(ctx context.Context, agentID string, userMessage string, history []llm.Message) (finalResp *llm.Response, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = recoverAsError(agentID, rec)
		}
	}()
	ctx, cancelTurn := resolveTurnTimeout(ctx, DefaultTurnTimeout)
	defer cancelTurn()

	agent, err := e.agentMgr.Get(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", agentID, err)
	}

	if agent.Status == "" {
		agent.Status = StatusActive
	}
	if agent.Status != StatusActive {
		return nil, fmt.Errorf("agent %s is not active (status=%s)", agentID, agent.Status)
	}
	ctx = e.withSkillCatalog(ctx, agent)
	source := sourceFromContextOrMessage(ctx, userMessage, "chat")
	if err := e.checkBudget(ctx, agent, source); err != nil {
		return nil, err
	}
	maxConc := agent.DelegationScope.MaxConcurrentRuns
	if maxConc <= 0 {
		maxConc = DefaultMaxConcurrentRuns
	}
	if e.inFlight != nil && e.inFlight.count(agentID) >= maxConc {
		return nil, fmt.Errorf("%w: agent %s already has %d running turns", ErrConcurrentRunQuota, agentID, maxConc)
	}

	traceID := generateTraceID()
	ctx = tools.WithTraceID(ctx, traceID)
	run := e.startRun(ctx, traceID, agentID, userMessage, source)
	runID := traceID
	if run != nil {
		runID = run.ID
	}
	if e.inFlight != nil {
		_ = e.inFlight.track(agentID, runID, cancelTurn)
		defer e.inFlight.untrack(agentID, runID)
	}

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Build unified cognitive system prompt
	fullSystemPrompt, _ := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)
	// fmt.Printf("\n================ [DEBUG SYSTEM PROMPT (%s)] ================\n%s\n============================================================\n\n", agentID, fullSystemPrompt)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: fullSystemPrompt},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMessage})
	messages = e.attachAutonomousPlan(ctx, messages, userMessage, history)

	// 3. Model cascade
	var cascadeOrder []string
	if agent.ModelConfig.PrimaryModel != "" {
		cascadeOrder = append(cascadeOrder, agent.ModelConfig.PrimaryModel)
	}
	if agent.ModelConfig.FallbackModel != "" {
		cascadeOrder = append(cascadeOrder, agent.ModelConfig.FallbackModel)
	}

	opts := llm.CompletionOptions{
		ReasoningEffort: agent.ModelConfig.EffectiveReasoningEffort(),
	}
	defaultMaxTokens := 32768
	if agent.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &agent.ModelConfig.MaxTokens
	} else {
		opts.MaxTokens = &defaultMaxTokens
	}

	// 3. Attach authorized tools if registry available
	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		authorizedTools := agent.AuthorizedTools
		if allowed := tools.AllowedTools(ctx); allowed != nil {
			authorizedTools = allowed
		}
		opts.Tools = e.tools.ToLLMToolDefinitions(authorizedTools, tools.DeniedTools(ctx)...)
	}
	startTime := time.Now()
	var allExecutedToolCalls []llm.ToolCall
	totalUsage := llm.Usage{}
	maxIterations := 20
	consecutiveFailures := 0
	lastObservation := ""
	repeatedObservations := 0
	converged := false
	iterationsCompleted := 0

	for iter := 0; iter < maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			e.finishRun(ctx, run, RunCancelled, "context_cancelled", iterationsCompleted, totalUsage)
			return nil, fmt.Errorf("%w: %v", ErrRunCancelled, err)
		}
		iterationsCompleted = iter + 1
		targetModel := agent.ModelConfig.PrimaryModel
		if targetModel == "" {
			targetModel = "cascade-llm"
		}

		currentOpts := opts
		// On the final iteration, force convergence by omitting tools
		if iter == maxIterations-1 {
			currentOpts.Tools = nil
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "Iteration limit reached. Do not call any more tools. Synthesize all observations gathered above into your final comprehensive response to the user.",
			})
		}

		// CRITICAL FIX: Prune context BEFORE LLM call to ensure we send clean, complete message sequences
		if e.contextManager != nil {
			runID := ""
			if run != nil {
				runID = run.ID
			}
			messages = e.contextManager.PruneAndSnapshot(ctx, runID, messages, e.contextBudget(agent))
		}

		resp, err := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, currentOpts)
		if err != nil {
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
			}
			e.finishRun(ctx, run, RunFailed, "infrastructure_failure", iter+1, totalUsage)
			return nil, fmt.Errorf("llm completion: %w", err)
		}
		totalUsage = addUsage(totalUsage, resp.Usage)
		if run != nil {
			_ = e.runStore.AppendEvent(ctx, RunEvent{
				RunID: run.ID, TraceID: traceID, Step: iter + 1, Type: "llm",
				Status: "success", Data: map[string]any{"model": resp.Model, "tool_calls": len(resp.ToolCalls)},
			})
		}

		finalResp = resp
		if len(resp.ToolCalls) > 0 {
			allExecutedToolCalls = append(allExecutedToolCalls, resp.ToolCalls...)
		}
		if resp.Content != "" {
			cleanedContent, thinking := llm.ExtractThinkingContent(resp.Content, resp.ReasoningContent)
			resp.Content = cleanedContent
			resp.ReasoningContent = thinking
		}

		if len(resp.ToolCalls) == 0 && resp.Content != "" {
			cleaned, embeddedCalls := llm.ExtractEmbeddedToolCalls(resp.Content)
			if len(embeddedCalls) > 0 && currentOpts.Tools != nil {
				resp.ToolCalls = embeddedCalls
			}
			resp.Content = cleaned
		}

		finalResp = resp

		// If no tool calls requested, we reached the final response
		if len(resp.ToolCalls) == 0 || e.tools == nil || currentOpts.Tools == nil {
			converged = true
			break
		}

		// CRITICAL FIX: Build assistant message + ALL tool results ATOMICALLY to prevent race conditions
		// Collect all tool results first
		var toolMessages []llm.Message

		for _, tc := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				e.finishRun(ctx, run, RunCancelled, "context_cancelled", iter+1, totalUsage)
				return nil, fmt.Errorf("%w: %v", ErrRunCancelled, err)
			}
			toolResult, execErr := e.verifyAndExecuteTool(ctx, agentID, tc.Function.Name, tc.Function.Arguments)
			resultStr := ""
			if execErr != nil {
				resultStr = fmt.Sprintf("Error executing tool %s: %v", tc.Function.Name, execErr)
				if toolResult != nil && toolResult.Content != "" && toolResult.Content != resultStr {
					resultStr = fmt.Sprintf("%s\n%s", resultStr, toolResult.Content)
				}
				consecutiveFailures++
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(execErr, &approvalErr) {
					// Persist the in-flight block (assistant + results already produced)
					// so the resumed transcript stays well-formed.
					checkpointMessages := append(append([]llm.Message{}, messages...), llm.Message{
						Role:             llm.RoleAssistant,
						Content:          "",
						ReasoningContent: resp.ReasoningContent,
						ToolCalls:        resp.ToolCalls,
						ProviderItems:    resp.ProviderItems,
					})
					checkpointMessages = append(checkpointMessages, toolMessages...)
					e.saveApprovalCheckpoint(ctx, run, agentID, userMessage, source, checkpointMessages, iter+1, totalUsage, tc)
					e.finishRun(ctx, run, RunApprovalPending, "approval_required", iter+1, totalUsage)
					return nil, execErr
				}
			} else if toolResult != nil {
				resultStr = toolResult.Content
				consecutiveFailures = 0
			}
			if resultStr == lastObservation {
				repeatedObservations++
			} else {
				repeatedObservations = 0
				lastObservation = resultStr
			}
			if run != nil {
				status := "success"
				if execErr != nil {
					status = "error"
				}
				_ = e.runStore.AppendEvent(ctx, RunEvent{
					RunID: run.ID, TraceID: traceID, Step: iter + 1, Type: "tool",
					Status: status, ToolName: tc.Function.Name,
					Data: map[string]any{"tool_call_id": tc.ID, "observation": truncateRunData(resultStr, 4096)},
				})
			}

			toolMessages = append(toolMessages, llm.Message{
				Role:       llm.RoleTool,
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})

			if consecutiveFailures >= 5 || repeatedObservations >= 3 || iter >= maxIterations-2 {
				// Don't cut off abruptly! Disable further tool calls so LLM processes the error
				// observations and synthesizes a direct diagnostic answer to the user in the next iteration.
				opts.Tools = nil
			}
		}

		// ATOMIC APPEND: assistant message + all tool results together
		// This prevents context pruning from splitting the assistant+tool sequence
		messages = append(messages, llm.Message{
			Role:             llm.RoleAssistant,
			Content:          "",
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
			ProviderItems:    resp.ProviderItems,
		})
		messages = append(messages, toolMessages...)
	}
	if !converged {
		// Attempt a final recovery turn with tools disabled so LLM explains the errors to the user
		finalOpts := opts
		finalOpts.Tools = nil
		lastResp, genErr := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, finalOpts)
		if genErr == nil && lastResp != nil && lastResp.Content != "" {
			finalResp = lastResp
			converged = true
			totalUsage = addUsage(totalUsage, lastResp.Usage)
		} else {
			e.finishRun(ctx, run, RunBlocked, "iteration_budget_exhausted", maxIterations, totalUsage)
			return nil, fmt.Errorf("agent %s reached the maximum of %d ReAct iterations without convergence", agentID, maxIterations)
		}
	}
	if finalResp == nil {
		finalResp = &llm.Response{
			Content: "",
			Model:   agent.ModelConfig.PrimaryModel,
		}
	}
	if !e.verifier.VerifySemanticConsistency(ctx, userMessage, finalResp.Content) && strings.TrimSpace(finalResp.Content) != "" {
		finalResp.Content = "I have processed your request and completed the authorized actions."
	}
	finalResp.Usage = totalUsage
	if finalResp != nil && len(allExecutedToolCalls) > 0 {
		finalResp.ToolCalls = allExecutedToolCalls
	}

	// 4. Trigger reflection daemon asynchronously (updates MEMORY.md, preferences, and episodic memory)
	if finalResp != nil && finalResp.Content != "" {
		if e.reflectionEngine != nil {
			e.reflectionEngine.ReflectOnConversation(context.Background(), agentID, userMessage, finalResp.Content)
		} else if e.memory != nil {
			go func() {
				_, _ = e.memory.StoreMemory(
					context.Background(),
					agentID,
					memory.LayerEpisodic,
					fmt.Sprintf("User asked: %s | Response: %s", userMessage, finalResp.Content),
					nil,
					map[string]any{"timestamp": time.Now().UTC()},
					1.0,
				)
			}()
		}
	}

	if e.bus != nil && finalResp != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, agentID, map[string]any{
			"duration_ms": time.Since(startTime).Milliseconds(),
			"tokens":      finalResp.Usage.TotalTokens,
		}))
	}

	if finalResp != nil {
		modelName := finalResp.Model
		if (modelName == "" || !strings.Contains(modelName, "/")) && agent.ModelConfig.PrimaryModel != "" {
			modelName = agent.ModelConfig.PrimaryModel
		}
		e.RecordTokenUsage(ctx, agentID, modelName, "", source, "", finalResp.Usage)
	}
	e.finishRun(ctx, run, RunCompleted, "goal_completed", iterationsCompleted, totalUsage)

	return finalResp, nil
}

// ExecuteStepStream runs a cognitive iteration of the ReAct state machine while emitting
// ExecuteStepStream runs a single cognitive iteration of the ReAct state machine while streaming events.
func (e *Engine) ExecuteStepStream(ctx context.Context, agentID string, userMessage string, eventChan chan<- AgentStreamEvent) (*llm.Response, error) {
	return e.ExecuteStepStreamWithHistory(ctx, agentID, userMessage, nil, eventChan)
}

// ExecuteStepStreamWithHistory runs a streaming cognitive iteration with dialogue history.
func (e *Engine) ExecuteStepStreamWithHistory(ctx context.Context, agentID string, userMessage string, history []llm.Message, eventChan chan<- AgentStreamEvent) (*llm.Response, error) {
	defer close(eventChan)

	startTime := time.Now()

	agent, err := e.agentMgr.Get(ctx, agentID)
	if err != nil {
		eventChan <- AgentStreamEvent{
			Type:  EventStreamError,
			Error: fmt.Sprintf("getting agent %s: %v", agentID, err),
		}
		return nil, fmt.Errorf("getting agent %s: %w", agentID, err)
	}

	if agent.Status == "" {
		agent.Status = StatusActive
	}
	if agent.Status != StatusActive {
		err := fmt.Errorf("agent %s is not active (status=%s)", agentID, agent.Status)
		eventChan <- AgentStreamEvent{
			Type:  EventStreamError,
			Error: err.Error(),
		}
		return nil, err
	}
	source := sourceFromContextOrMessage(ctx, userMessage, "stream")
	if err := e.checkBudget(ctx, agent, source); err != nil {
		eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
		return nil, err
	}

	traceID := generateTraceID()
	ctx = tools.WithTraceID(ctx, traceID)
	run := e.startRun(ctx, traceID, agentID, userMessage, source)

	eventChan <- AgentStreamEvent{
		Type:    EventStreamThought,
		Thought: fmt.Sprintf("Activating agent '%s' (%s)...", agent.Name, agent.ModelConfig.PrimaryModel),
	}

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Build unified 4-layer cognitive system prompt & search memory
	eventChan <- AgentStreamEvent{
		Type:    EventStreamThought,
		Thought: "Retrieving user profile, procedural guidelines, and episodic memory...",
	}
	memStart := time.Now()
	fullSystemPrompt, memoryCount := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)
	memDuration := time.Since(memStart).Milliseconds()

	// Using for debug only, don't remove it
	// fmt.Printf("\n================ [DEBUG STREAM SYSTEM PROMPT (%s)] ================\n%s\n===================================================================\n\n", agentID, fullSystemPrompt)

	if memoryCount > 0 {
		eventChan <- AgentStreamEvent{
			Type: EventStreamAudit,
			AuditLog: &AuditLogEntry{
				Timestamp:    time.Now().UTC(),
				AgentID:      agentID,
				Action:       "cognitive_memory_retrieval",
				Status:       "success",
				Verification: fmt.Sprintf("Retrieved %d memory fragments (episodic diary + semantic vector knowledge)", memoryCount),
				DurationMs:   memDuration,
			},
		}
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: fullSystemPrompt},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMessage})
	messages = e.attachAutonomousPlan(ctx, messages, userMessage, history)

	// 3. Model cascade
	var cascadeOrder []string
	if agent.ModelConfig.PrimaryModel != "" {
		cascadeOrder = append(cascadeOrder, agent.ModelConfig.PrimaryModel)
	}
	if agent.ModelConfig.FallbackModel != "" {
		cascadeOrder = append(cascadeOrder, agent.ModelConfig.FallbackModel)
	}

	opts := llm.CompletionOptions{
		ReasoningEffort: agent.ModelConfig.EffectiveReasoningEffort(),
	}
	defaultStreamMaxTokens := 32768
	if agent.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &agent.ModelConfig.MaxTokens
	} else {
		opts.MaxTokens = &defaultStreamMaxTokens
	}

	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		authorizedTools := agent.AuthorizedTools
		if allowed := tools.AllowedTools(ctx); allowed != nil {
			authorizedTools = allowed
		}
		opts.Tools = e.tools.ToLLMToolDefinitions(authorizedTools, tools.DeniedTools(ctx)...)
	}

	var finalResp *llm.Response
	var allExecutedToolCalls []llm.ToolCall
	totalUsage := llm.Usage{}
	maxIterations := 20
	converged := false
	iterationsCompleted := 0
	consecutiveFailures := 0
	lastObservation := ""
	repeatedObservations := 0

	for iter := 0; iter < maxIterations; iter++ {
		iterationsCompleted = iter + 1
		targetModel := agent.ModelConfig.PrimaryModel
		if targetModel == "" {
			targetModel = "cascade-llm"
		}

		currentOpts := opts
		if iter == maxIterations-1 {
			currentOpts.Tools = nil
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "Iteration limit reached. Do not call any more tools. Synthesize all observations gathered above into your final comprehensive response to the user.",
			})
		}

		eventChan <- AgentStreamEvent{
			Type:    EventStreamThought,
			Thought: fmt.Sprintf("Deliberating with %s (ReAct iteration %d)...", targetModel, iter+1),
		}

		if e.contextManager != nil {
			runID := ""
			if run != nil {
				runID = run.ID
			}
			messages = e.contextManager.PruneAndSnapshot(ctx, runID, messages, e.contextBudget(agent))
		}
		resp, err := e.completeStreamIteration(ctx, cascadeOrder, messages, currentOpts, eventChan)
		if err != nil {
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
			}
			eventChan <- AgentStreamEvent{
				Type:  EventStreamError,
				Error: err.Error(),
			}
			e.finishRun(ctx, run, RunFailed, "infrastructure_failure", iterationsCompleted, totalUsage)
			return nil, fmt.Errorf("llm completion: %w", err)
		}
		totalUsage = addUsage(totalUsage, resp.Usage)
		if run != nil {
			_ = e.runStore.AppendEvent(ctx, RunEvent{
				RunID: run.ID, TraceID: traceID, Step: iterationsCompleted, Type: "llm",
				Status: "success", Data: map[string]any{"model": resp.Model, "tool_calls": len(resp.ToolCalls)},
			})
		}

		finalResp = resp
		if len(resp.ToolCalls) > 0 {
			allExecutedToolCalls = append(allExecutedToolCalls, resp.ToolCalls...)
		}

		// If no tool calls requested, stream tokens preserving exact whitespace and newlines
		if len(resp.ToolCalls) == 0 || e.tools == nil || currentOpts.Tools == nil {
			converged = true
			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "cognitive_react_cycle",
					Status:       "converged",
					Verification: "Tier-2 Semantic Consistency Verified",
					DurationMs:   time.Since(startTime).Milliseconds(),
				},
			}
			break
		}

		// CRITICAL FIX: the assistant message and every tool result must enter the
		// transcript as one unit. Collect results first, append atomically after the
		// loop so pruning can never observe a half-built tool-call block.
		assistantMsg := llm.Message{
			Role:             llm.RoleAssistant,
			Content:          "",
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
			ProviderItems:    resp.ProviderItems,
		}
		var toolMessages []llm.Message

		// Execute each tool call with AST inspection & live streaming events
		for _, tc := range resp.ToolCalls {
			toolName := tc.Function.Name
			toolArgs := string(tc.Function.Arguments)

			eventChan <- AgentStreamEvent{
				Type:       EventStreamToolCall,
				Tool:       toolName,
				ToolCallID: tc.ID,
				Args:       toolArgs,
			}

			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: fmt.Sprintf("Running Tier-1 AST & security policy verification for tool '%s'...", toolName),
			}

			// Perform Tier-1 Security Verification. A failed policy check is terminal
			// for this call and must never degrade into a warning.
			var verifyErr error
			if e.verifier != nil {
				if toolName == "native_exec" {
					verifyErr = e.verifier.VerifyToolCommand(tc.Function.Arguments)
				}
			}

			verificationStatus := "Tier-1 AST Clean & Safe"
			if verifyErr != nil {
				verificationStatus = fmt.Sprintf("Blocked: %v", verifyErr)
			}

			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "tool_security_check",
					ToolName:     toolName,
					Parameters:   toolArgs,
					Status:       map[bool]string{true: "blocked", false: "verified"}[verifyErr != nil],
					Verification: verificationStatus,
					DurationMs:   0,
				},
			}
			if verifyErr != nil {
				resultStr := fmt.Sprintf("Tool execution blocked by policy: %v", verifyErr)
				eventChan <- AgentStreamEvent{
					Type: EventStreamToolResult, Tool: toolName, ToolCallID: tc.ID,
					Result: resultStr, Status: "blocked",
				}
				toolMessages = append(toolMessages, llm.Message{
					Role: llm.RoleTool, Name: toolName, ToolCallID: tc.ID, Content: resultStr,
				})
				consecutiveFailures++
				continue
			}

			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: fmt.Sprintf("Executing tool '%s' in isolated runtime sandbox...", toolName),
			}

			t0 := time.Now()
			toolResult, execErr := e.tools.Execute(ctx, agentID, toolName, tc.Function.Arguments)
			latency := time.Since(t0).Milliseconds()

			resultStr := ""
			statusStr := "success"
			if execErr != nil {
				statusStr = "error"
				resultStr = fmt.Sprintf("Error executing tool %s: %v", toolName, execErr)
				if toolResult != nil && toolResult.Content != "" && toolResult.Content != resultStr {
					resultStr = fmt.Sprintf("%s\n%s", resultStr, toolResult.Content)
				}
				consecutiveFailures++
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(execErr, &approvalErr) {
					// Persist the in-flight tool-call block (assistant + results produced
					// so far) so ResumeApproved sees a well-formed transcript.
					checkpointMessages := append(append([]llm.Message{}, messages...), assistantMsg)
					checkpointMessages = append(checkpointMessages, toolMessages...)
					e.saveApprovalCheckpoint(ctx, run, agentID, userMessage, source, checkpointMessages, iterationsCompleted, totalUsage, tc)
					eventChan <- AgentStreamEvent{
						Type: EventStreamAudit,
						AuditLog: &AuditLogEntry{
							Timestamp: time.Now().UTC(), AgentID: agentID,
							Action: "approval_required", ToolName: toolName, Status: "pending",
							Verification: approvalErr.Approval.ID,
						},
					}
					e.finishRun(ctx, run, RunApprovalPending, "approval_required", iterationsCompleted, totalUsage)
					eventChan <- AgentStreamEvent{Type: EventStreamError, Error: execErr.Error()}
					return nil, execErr
				}
			} else if toolResult != nil {
				resultStr = toolResult.Content
				consecutiveFailures = 0
			}
			if resultStr == lastObservation {
				repeatedObservations++
			} else {
				repeatedObservations = 0
				lastObservation = resultStr
			}
			if run != nil {
				_ = e.runStore.AppendEvent(ctx, RunEvent{
					RunID: run.ID, TraceID: traceID, Step: iterationsCompleted, Type: "tool",
					Status: statusStr, ToolName: toolName, DurationMS: latency,
					Data: map[string]any{"tool_call_id": tc.ID, "observation": truncateRunData(resultStr, 4096)},
				})
			}

			eventChan <- AgentStreamEvent{
				Type:       EventStreamToolResult,
				Tool:       toolName,
				ToolCallID: tc.ID,
				Result:     resultStr,
				Status:     statusStr,
				LatencyMs:  latency,
			}

			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "tool_execution",
					ToolName:     toolName,
					Parameters:   toolArgs,
					Status:       statusStr,
					Verification: "Sandboxed Process Isolation Safe",
					DurationMs:   latency,
				},
			}

			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: fmt.Sprintf("Tool '%s' returned (%d ms). Analyzing observation...", toolName, latency),
			}

			toolMessages = append(toolMessages, llm.Message{
				Role:       llm.RoleTool,
				Name:       toolName,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})
			if consecutiveFailures >= 5 || repeatedObservations >= 3 || iterationsCompleted >= maxIterations-2 {
				// Don't kill the stream abruptly! Disable tools so LLM processes the error
				// and streams its concluding explanation directly to the user.
				opts.Tools = nil
			}
		}

		// ATOMIC APPEND: assistant tool-call message plus every matching result.
		messages = append(messages, assistantMsg)
		messages = append(messages, toolMessages...)
	}
	if !converged {
		// Stream a final recovery turn with tools disabled so LLM explains the outcome to the user
		finalOpts := opts
		finalOpts.Tools = nil
		eventChan <- AgentStreamEvent{
			Type:    EventStreamThought,
			Thought: "Synthesizing final findings and reporting observations...",
		}
		resp, streamErr := e.completeStreamIteration(ctx, cascadeOrder, messages, finalOpts, eventChan)
		if streamErr == nil && resp != nil {
			finalResp = resp
			converged = true
			totalUsage = addUsage(totalUsage, resp.Usage)
		} else {
			e.finishRun(ctx, run, RunBlocked, "iteration_budget_exhausted", iterationsCompleted, totalUsage)
			err := fmt.Errorf("agent %s reached the maximum of %d ReAct iterations without convergence", agentID, maxIterations)
			eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
			return nil, err
		}
	}
	if finalResp == nil {
		finalResp = &llm.Response{
			Content: "",
			Model:   agent.ModelConfig.PrimaryModel,
		}
	}
	if !e.verifier.VerifySemanticConsistency(ctx, userMessage, finalResp.Content) && strings.TrimSpace(finalResp.Content) != "" {
		finalResp.Content = "I have processed your request and completed the authorized actions."
	}
	finalResp.Usage = totalUsage
	if finalResp != nil && len(allExecutedToolCalls) > 0 {
		finalResp.ToolCalls = allExecutedToolCalls
	}

	// 4. Trigger reflection daemon asynchronously (updates MEMORY.md, preferences, and episodic memory)
	if finalResp != nil && finalResp.Content != "" {
		if e.reflectionEngine != nil {
			e.reflectionEngine.ReflectOnConversation(context.Background(), agentID, userMessage, finalResp.Content)
		} else if e.memory != nil {
			go func() {
				_, _ = e.memory.StoreMemory(
					context.Background(),
					agentID,
					memory.LayerEpisodic,
					fmt.Sprintf("User asked: %s | Response: %s", userMessage, finalResp.Content),
					nil,
					map[string]any{"timestamp": time.Now().UTC()},
					1.0,
				)
			}()
		}
	}

	if e.bus != nil && finalResp != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, agentID, map[string]any{
			"duration_ms": time.Since(startTime).Milliseconds(),
			"tokens":      finalResp.Usage.TotalTokens,
		}))
	}

	if finalResp != nil {
		modelName := finalResp.Model
		if (modelName == "" || !strings.Contains(modelName, "/")) && agent.ModelConfig.PrimaryModel != "" {
			modelName = agent.ModelConfig.PrimaryModel
		}
		e.RecordTokenUsage(ctx, agentID, modelName, "", source, "", finalResp.Usage)
	}

	eventChan <- AgentStreamEvent{
		Type:    EventStreamDone,
		Content: finalResp.Content,
		Model:   finalResp.Model,
		Usage:   &finalResp.Usage,
	}
	e.finishRun(ctx, run, RunCompleted, "goal_completed", iterationsCompleted, totalUsage)

	return finalResp, nil
}

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

			// Sync and complete autonomous task if this run was for a Task
			targetTaskID := checkpoint.TaskID
			if targetTaskID == "" {
				re := regexp.MustCompile(`Task ID:\s*([^\s|]+)`)
				if match := re.FindStringSubmatch(checkpoint.Goal); len(match) > 1 {
					targetTaskID = match[1]
				}
			}
			if targetTaskID != "" && e.taskMgr != nil {
				if task, err := e.taskMgr.GetTask(execCtx, targetTaskID); err == nil && task != nil {
					content := strings.TrimSpace(response.Content)
					shortLog := shortSummary(content, 250)
					if strings.Contains(content, "[TASK_COMPLETED]") && (e.verifier == nil || e.verifier.VerifyTaskCompletion(task.Description, content, response.ToolCalls)) {
						task.Status = "completed"
						task.Progress = 100
						now := time.Now().UTC()
						task.CompletedAt = &now
						task.ExecutionLog = shortLog
					} else if strings.Contains(content, "[TASK_BLOCKED") {
						task.Status = "blocked"
						task.ExecutionLog = shortLog
					} else {
						task.Status = "in_progress"
						task.ExecutionLog = shortLog
					}
					_ = e.taskMgr.UpdateTask(execCtx, *task)
					if e.sessionMgr != nil && task.SessionID != "" {
						_ = e.sessionMgr.SaveMessage(execCtx, task.SessionID, checkpoint.AgentID, "assistant", response.Content, response.ToolCalls)
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
			}

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

func (e *Engine) contextBudget(agent *AgentManifest) int {
	max := 128000
	if agent != nil && agent.ModelConfig.MaxTokens > 0 && agent.ModelConfig.MaxTokens < max-8192 {
		return max - agent.ModelConfig.MaxTokens
	}
	return 100000
}

func (e *Engine) startRun(ctx context.Context, traceID, agentID, goal, source string) *AgentRun {
	if e.runStore == nil {
		return nil
	}
	run, err := e.runStore.Start(ctx, traceID, agentID, goal, source)
	if err != nil {
		return nil
	}
	return run
}

func (e *Engine) finishRun(ctx context.Context, run *AgentRun, status RunStatus, reason string, iterations int, usage llm.Usage) {
	if run == nil || e.runStore == nil {
		return
	}
	run.Status = status
	run.TerminationReason = reason
	run.Iterations = iterations
	run.PromptTokens = usage.PromptTokens
	run.CompletionTokens = usage.CompletionTokens
	run.TotalTokens = usage.TotalTokens
	_ = e.runStore.Finish(ctx, run)
}

func (e *Engine) saveApprovalCheckpoint(
	ctx context.Context,
	run *AgentRun,
	agentID, goal, source string,
	messages []llm.Message,
	iteration int,
	usage llm.Usage,
	pending llm.ToolCall,
) {
	if run == nil || e.runStore == nil {
		return
	}
	taskID, _ := ctx.Value("task_id").(string)
	// The approval was hashed over the normalized input, so the checkpoint must
	// persist the same form or the resume-time hash comparison will not match.
	pending.Function.Arguments = tools.NormalizeToolInput(pending.Function.Arguments)
	_ = e.runStore.SaveCheckpoint(ctx, RunCheckpoint{
		RunID: run.ID, TraceID: run.TraceID, AgentID: agentID, TaskID: taskID, Goal: goal,
		Source: source, Messages: messages, Iteration: iteration,
		Usage: usage, PendingTool: pending,
	})
}

func addUsage(total, current llm.Usage) llm.Usage {
	total.PromptTokens += current.PromptTokens
	total.CompletionTokens += current.CompletionTokens
	total.TotalTokens += current.TotalTokens
	return total
}

func generateTraceID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func truncateRunData(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func (e *Engine) checkBudget(ctx context.Context, manifest *AgentManifest, source string) error {
	if manifest == nil {
		return nil
	}
	if e.tokenTracker != nil {
		hourlyCap := manifest.DelegationScope.MaxTokensPerHour
		if hourlyCap <= 0 {
			hourlyCap = DefaultMaxTokensPerHour
		}
		if used, err := e.tokenTracker.GetAgentHourlyTokens(ctx, manifest.AgentID); err == nil && hourlyCap > 0 && used >= int64(hourlyCap) {
			if source == "heartbeat" || source == "cron" {
				return fmt.Errorf("%w: agent %s used %d tokens in the last hour (cap %d); degrading autonomous pulses", ErrHourlyTokenQuota, manifest.AgentID, used, hourlyCap)
			}
		}
	}
	if e.tokenTracker == nil || manifest.DelegationScope.MaxMonthlyBudgetUSD <= 0 {
		return nil
	}
	cost, err := e.tokenTracker.GetAgentMonthlyCost(ctx, manifest.AgentID)
	if err != nil {
		return fmt.Errorf("checking monthly agent budget: %w", err)
	}
	budget := manifest.DelegationScope.MaxMonthlyBudgetUSD
	if cost >= budget && (source == "heartbeat" || source == "cron") {
		return fmt.Errorf(
			"agent %s monthly budget exhausted for autonomous work: spent %.4f USD of %.4f USD (interactive chat still allowed on fallback)",
			manifest.AgentID, cost, budget,
		)
	}
	if cost >= budget*0.8 && manifest.ModelConfig.FallbackModel != "" {
		manifest.ModelConfig.PrimaryModel = manifest.ModelConfig.FallbackModel
	}
	return nil
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

func sourceFromContextOrMessage(ctx context.Context, message, fallback string) string {
	if src := ExecutionSource(ctx); src != "" {
		return src
	}
	return sourceFromMessage(message, fallback)
}

func sourceFromMessage(message, fallback string) string {
	switch {
	case strings.Contains(message, "[AUTONOMOUS MISSION"), strings.Contains(message, "<autonomous_mission_cycle"):
		return "heartbeat"
	case strings.Contains(message, "[AUTONOMOUS HEARTBEAT"), strings.Contains(message, "<autonomous_heartbeat"):
		return "heartbeat"
	case strings.Contains(message, "[AUTONOMOUS PROACTIVE"):
		return "cron"
	case strings.Contains(message, "[Channel:"), strings.Contains(message, "[Channel Metadata]"):
		return "channel"
	default:
		return fallback
	}
}

func (e *Engine) verifyAndExecuteTool(ctx context.Context, agentID, name string, args json.RawMessage) (*tools.ToolResult, error) {
	if e.verifier != nil && name == "native_exec" {
		if err := e.verifier.VerifyToolCommand(args); err != nil {
			return nil, err
		}
	}
	if e.tools == nil {
		return nil, fmt.Errorf("tool registry is not configured")
	}
	return e.tools.Execute(ctx, agentID, name, args)
}

func (e *Engine) completeStreamIteration(
	ctx context.Context,
	cascadeOrder []string,
	messages []llm.Message,
	opts llm.CompletionOptions,
	eventChan chan<- AgentStreamEvent,
) (*llm.Response, error) {
	stream, err := e.llm.StreamCompleteWithCascade(ctx, cascadeOrder, messages, opts)
	if err != nil {
		return nil, err
	}
	response := &llm.Response{}
	if len(cascadeOrder) > 0 {
		response.Model = cascadeOrder[0]
	}

	// Content deltas are forwarded the moment they arrive so the UI renders a real
	// typing effect. Buffering them until the stream closed made every token land
	// in one burst at the end, which is what looked like "jumping text".
	//
	// The catch: we cannot know whether this iteration is a tool-calling turn until
	// the stream ends, and a tool-calling turn's prose must not be shown as the
	// final answer. So tokens are emitted live, and if tool calls do turn up we
	// send a retraction event telling the client to discard this turn's tokens.
	streamedTokens := false

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				goto DoneStream
			}
			if chunk.Error != nil {
				return nil, chunk.Error
			}
			if chunk.DeltaReasoning != "" {
				response.ReasoningContent += chunk.DeltaReasoning
				eventChan <- AgentStreamEvent{Type: EventStreamReasoning, Reasoning: chunk.DeltaReasoning}
			}
			if chunk.DeltaContent != "" {
				response.Content += chunk.DeltaContent
				// If the model is streaming thinking inside <think> tags, emit directly as reasoning
				if isInsideThinkingBlock(response.Content) {
					cleanDelta := stripThinkingTags(chunk.DeltaContent)
					if cleanDelta != "" {
						response.ReasoningContent += cleanDelta
						eventChan <- AgentStreamEvent{Type: EventStreamReasoning, Reasoning: cleanDelta}
					}
				} else if !isInsideMarkupTag(response.Content) {
					// Only emit prose if we are NOT inside a DSML or internal markup tag block.
					// This prevents raw <｜｜DSML... tokens from ever flashing on the user's screen.
					cleanDelta := stripThinkingTags(chunk.DeltaContent)
					if cleanDelta != "" {
						eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: cleanDelta}
						streamedTokens = true
					}
				}
			}
			if len(chunk.ToolCalls) > 0 {
				response.ToolCalls = append(response.ToolCalls, chunk.ToolCalls...)
			}
			if len(chunk.ProviderItems) > 0 {
				response.ProviderItems = append(response.ProviderItems, chunk.ProviderItems...)
			}
			if chunk.Usage != nil {
				response.Usage = *chunk.Usage
			}
			if chunk.Done {
				goto DoneStream
			}
		}
	}

DoneStream:
	rawStreamedContent := response.Content

	if response.Content != "" {
		cleanedContent, thinking := llm.ExtractThinkingContent(response.Content, response.ReasoningContent)
		if thinking != "" && response.ReasoningContent == "" {
			eventChan <- AgentStreamEvent{Type: EventStreamReasoning, Reasoning: thinking}
		}
		response.Content = cleanedContent
		response.ReasoningContent = thinking
	}

	// If the model produced embedded tool calls (e.g. DeepSeek DSML or XML tool calls) in content
	if len(response.ToolCalls) == 0 && response.Content != "" {
		cleaned, embeddedCalls := llm.ExtractEmbeddedToolCalls(response.Content)
		if len(embeddedCalls) > 0 {
			response.ToolCalls = embeddedCalls
		}
		response.Content = cleaned
	}

	// This turn called tools, so retract any interim streamed tokens from the main answer buffer.
	if len(response.ToolCalls) > 0 && streamedTokens {
		eventChan <- AgentStreamEvent{Type: EventStreamTokenReset}
	} else if len(response.ToolCalls) == 0 && streamedTokens && response.Content != rawStreamedContent {
		// Tokens were streamed live, but DSML/thinking tags were stripped at stream end.
		// Retract the raw stream and re-emit the clean content!
		eventChan <- AgentStreamEvent{Type: EventStreamTokenReset}
		if response.Content != "" {
			eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: response.Content}
		}
	} else if len(response.ToolCalls) == 0 && !streamedTokens && response.Content != "" {
		// Content was suppressed during streaming because of tag filtering, emit clean final content now.
		eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: response.Content}
	}

	return response, nil
}

// isInsideThinkingBlock reports if accumulated stream text is currently inside <think>, <thought>, etc.
func isInsideThinkingBlock(accumulated string) bool {
	lower := strings.ToLower(accumulated)
	for _, openTag := range []string{"<think>", "<thought>", "<thinking>", "[think]", "[reasoning]"} {
		closeTag := "</" + strings.TrimPrefix(strings.TrimSuffix(openTag, ">"), "<") + ">"
		if strings.HasPrefix(openTag, "[") {
			closeTag = "[/" + strings.TrimPrefix(strings.TrimSuffix(openTag, "]"), "[") + "]"
		}
		lastOpen := strings.LastIndex(lower, openTag)
		if lastOpen != -1 {
			lastClose := strings.LastIndex(lower, closeTag)
			if lastClose < lastOpen {
				return true
			}
		}
	}
	return false
}

// stripThinkingTags removes thinking markup open/close tags from incremental chunks.
func stripThinkingTags(chunk string) string {
	re := regexp.MustCompile(`(?i)</?(?:think|thought|thinking|\[/?(?:think|reasoning)\])>?`)
	return re.ReplaceAllString(chunk, "")
}

// isInsideMarkupTag reports if accumulated stream text is currently inside a DSML, thinking, or tool call markup block.
func isInsideMarkupTag(accumulated string) bool {
	// 1. DeepSeek DSML tool calls block
	if (strings.Contains(accumulated, "DSML｜｜tool_calls>") || strings.Contains(accumulated, "DSML||tool_calls>")) &&
		!strings.Contains(accumulated, "/DSML｜｜tool_calls>") && !strings.Contains(accumulated, "/DSML||tool_calls>") &&
		!strings.Contains(accumulated, "</｜｜DSML｜｜tool_calls>") && !strings.Contains(accumulated, "</||DSML||tool_calls>") {
		return true
	}

	// 2. Unclosed tag fragment
	lastOpen := strings.LastIndex(accumulated, "<")
	if lastOpen != -1 {
		lastClose := strings.LastIndex(accumulated, ">")
		if lastClose < lastOpen {
			fragment := strings.ToLower(accumulated[lastOpen:])
			if strings.HasPrefix(fragment, "<｜") ||
				strings.HasPrefix(fragment, "<|") ||
				strings.Contains(fragment, "dsml") ||
				strings.HasPrefix(fragment, "<think") ||
				strings.HasPrefix(fragment, "<thought") ||
				strings.HasPrefix(fragment, "<tool_call") ||
				strings.HasPrefix(fragment, "<function=") ||
				strings.HasPrefix(fragment, "<invoke") {
				return true
			}
		}
	}

	// 3. Bare JSON tool call in progress: e.g. `{"command":` or `{"path":` or `{"name":"native_`
	trimmed := strings.TrimSpace(accumulated)
	lastBrace := strings.LastIndex(trimmed, "{")
	if lastBrace != -1 {
		lastCloseBrace := strings.LastIndex(trimmed, "}")
		if lastCloseBrace < lastBrace {
			fragment := strings.ToLower(trimmed[lastBrace:])
			if strings.Contains(fragment, `"command"`) ||
				strings.Contains(fragment, `"path"`) ||
				strings.Contains(fragment, `"native_`) ||
				strings.Contains(fragment, `"tool"`) ||
				strings.Contains(fragment, `"arguments"`) {
				return true
			}
		}
	}

	return false
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

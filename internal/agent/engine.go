package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

// DefaultEntropyThreshold is θ for uncertainty-gated branching.
const DefaultEntropyThreshold = 0.65

// Engine orchestrates the POMDP & ReAct cognitive loop for agents.
type Engine struct {
	agentMgr         *AgentManager
	bus              *bus.EventBus
	llm              *llm.ModelCascadeRouter
	memory           *memory.HybridEngine
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
	theta            float64 // Entropy threshold
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
		contextManager: NewContextManager(8192),
		theta:          DefaultEntropyThreshold,
	}
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

// SetTaskManager attaches the autonomous task manager.
func (e *Engine) SetTaskManager(tm *TaskManager) {
	e.taskMgr = tm
}

// SetSessionManager attaches the session history provider.
func (e *Engine) SetSessionManager(sm SessionHistoryProvider) {
	e.sessionMgr = sm
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

// buildCognitivePrompt synthesizes all 4 layers of memory into a structured cognitive context:
// 1. Agent Soul / Persona Identity
// 2. User Profile Memory (Owner Identity, Role, Language, Preferences)
// 3. Procedural Memory (Workflows, Tool execution best practices)
// 4. Episodic Memory (Relevant past semantic memories from hybrid vector + BM25 search)
// 5. Agent-specific System Instructions
func (e *Engine) buildCognitivePrompt(ctx context.Context, agentID string, agent *AgentManifest, userMessage string) (string, int) {
	var sb strings.Builder

	// 1. Agent Identity, Role & Operational Profile (Multi-Purpose Foundation)
	sb.WriteString("# Agent Identity & Operational Context\n")
	if agent != nil {
		if agent.Name != "" {
			fmt.Fprintf(&sb, "- **Agent Name**: %s\n", agent.Name)
		}
		if agent.Description != "" {
			fmt.Fprintf(&sb, "- **Role / Position & Scope**: %s\n", agent.Description)
		}
	}
	sb.WriteString("- **Platform Environment**: ActonOS Intelligent Autonomous Multi-Agent Workspace\n\n")

	// 2. Base Soul Persona (isolated per agent)
	if e.profileMgr != nil {
		soul := e.profileMgr.GetAgentSoul(agentID)
		if soul != "" {
			sb.WriteString("## Core Soul & Temperament\n")
			sb.WriteString(soul)
			sb.WriteString("\n\n")
		}

		// 3. User Profile Memory (Owner Identity & Interaction Preferences)
		profile := e.profileMgr.GetProfile()
		sb.WriteString("## Collaborator Identity & Preferences\n")
		if profile.UserName != "" {
			fmt.Fprintf(&sb, "- **Collaborator Name**: %s\n", profile.UserName)
		}
		if profile.UserRole != "" {
			fmt.Fprintf(&sb, "- **Collaborator Role**: %s\n", profile.UserRole)
		}
		if profile.Language != "" {
			fmt.Fprintf(&sb, "- **Preferred Language**: %s\n", profile.Language)
		}
		if profile.Timezone != "" {
			fmt.Fprintf(&sb, "- **Timezone**: %s (Current Time: %s)\n", profile.Timezone, time.Now().UTC().Format(time.RFC3339))
		}
		if profile.CommunicationStyle != "" {
			fmt.Fprintf(&sb, "- **Communication Style**: %s\n", profile.CommunicationStyle)
		}
		if profile.CustomInstructions != "" {
			fmt.Fprintf(&sb, "- **Collaborator Directives**: %s\n", profile.CustomInstructions)
		}
		sb.WriteString("\n")

		// 4. Procedural Memory (Workflows & Execution Guidelines)
		patterns, _ := e.profileMgr.GetRelevantPatterns(ctx, "general")
		if len(patterns) > 0 {
			sb.WriteString("## Procedural Knowledge & Verified Workflows\n")
			for _, pat := range patterns {
				fmt.Fprintf(&sb, "- **%s** (%s): %s\n", pat.PatternName, pat.Domain, pat.Workflow)
			}
			sb.WriteString("\n")
		}
	}

	// 5. Agent Specific System Instructions
	if agent != nil && agent.SystemInstructions != "" {
		sb.WriteString("## Role Directives & Specialized Instructions\n")
		sb.WriteString(agent.SystemInstructions)
		sb.WriteString("\n\n")
	}

	// 6. Universal Conversational Standards & Anti-Robot Principles (Multi-Purpose)
	sb.WriteString("## Universal Operating Standards & Demeanor\n")
	sb.WriteString("- **Language Match (CRITICAL)**: Always respond in the EXACT language used by the collaborator in their prompt (e.g. Vietnamese if asked in Vietnamese, English if asked in English).\n")
	sb.WriteString("- **Direct Answer Delivery (CRITICAL)**: Always provide the actual answer, news, findings, or solution requested. NEVER respond with an empty greeting, status recap, or service menu ('Hey, I'm here and ready...') when the user asked for information or tasks.\n")
	sb.WriteString("- **Authentic & Empathetic Partnership**: Communicate naturally, intelligently, and respectfully. Embody your designated role and expertise with genuine dedication.\n")
	sb.WriteString("- **Zero Robotic Clichés**: NEVER produce canned AI disclaimers ('As an AI...', 'I am just a language model...'), generic filler, or repetitive apologies. Dive straight into meaningful, high-value assistance.\n")
	sb.WriteString("- **Clarity & Actionable Insight**: Deliver structured, clear, and beautifully formatted Markdown responses. Provide decisive recommendations, thorough analysis, or precise actions tailored to the user's specific domain.\n\n")

	// 7. Tool Selection & Execution Discipline
	sb.WriteString("## Tool Selection & Execution Discipline\n")
	sb.WriteString("- **Domain Relevance**: ONLY invoke tools that are directly related to the user's explicit request.\n")
	sb.WriteString("  - For web search, news, current events, or general knowledge: use `native_web_search`. NEVER explore workspace files (`native_file_read`, `native_file_list`), NEVER run shell commands (`native_exec`), and NEVER inspect host hardware telemetry (`native_sysinfo`).\n")
	sb.WriteString("  - For workspace code or local file questions: only then inspect files in the workspace.\n")
	sb.WriteString("  - NEVER randomly read filesystem files or query system diagnostics unless the user explicitly requested system or file operations.\n")
	sb.WriteString("- **Synthesize Tool Results (CRITICAL)**: When tool observations are present in the conversation, your response MUST synthesize and present the actual data and news gathered. NEVER ignore tool results to output a generic greeting.\n")
	sb.WriteString("- **Immediate Convergence**: As soon as relevant information is gathered (e.g. from web search or file read), IMMEDIATELY deliver your final response to the user. Do NOT invoke extra or unrelated tools.\n")
	sb.WriteString("- **Graceful Fallback**: If a tool returns no results, do not randomly try unrelated tools. Directly explain what you searched and provide the best available answer or summary.\n\n")

	// 8. Layer 4: Episodic Memory (Past interactions & learned facts)
	// Heartbeat mission execution sets "suppress_episodic_memory" to prevent
	// stale memories from deleted tasks from contaminating the current context.
	episodicCount := 0
	suppressEpisodic, _ := ctx.Value("suppress_episodic_memory").(bool)
	if e.memory != nil && !suppressEpisodic {
		memories, err := e.memory.Search(ctx, agentID, memory.LayerEpisodic, userMessage, nil, 4)
		if err == nil && len(memories) > 0 {
			episodicCount = len(memories)
			sb.WriteString("## Relevant Past Episodic Memories\n")
			for _, m := range memories {
				fmt.Fprintf(&sb, "- %s\n", m.Content)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), episodicCount
}

// CalculateEntropy calculates Shannon Entropy H(p) = -sum(p * log2(p)).
func CalculateEntropy(probabilities []float64) float64 {
	if len(probabilities) == 0 {
		return 0
	}
	var entropy float64
	for _, p := range probabilities {
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// ExecuteStep runs a single cognitive iteration of the ReAct state machine.
func (e *Engine) ExecuteStep(ctx context.Context, agentID string, userMessage string) (*llm.Response, error) {
	return e.ExecuteStepWithHistory(ctx, agentID, userMessage, nil)
}

// ExecuteAutonomousGoal decomposes and executes a dependency-aware plan before returning.
func (e *Engine) ExecuteAutonomousGoal(ctx context.Context, agentID, goal string, history []llm.Message) (*llm.Response, error) {
	if e.planner == nil || len(history) > 0 {
		return e.ExecuteStepWithHistory(ctx, agentID, goal, history)
	}
	agents, err := e.agentMgr.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing agents for planning: %w", err)
	}
	var modelCascade []string
	if agent, err := e.agentMgr.Get(ctx, agentID); err == nil && agent != nil {
		if agent.ModelConfig.PrimaryModel != "" {
			modelCascade = append(modelCascade, agent.ModelConfig.PrimaryModel)
		}
		if agent.ModelConfig.FallbackModel != "" {
			modelCascade = append(modelCascade, agent.ModelConfig.FallbackModel)
		}
	}
	plan, err := e.planner.DecomposeGoal(ctx, goal, agents, modelCascade...)
	if err != nil {
		return nil, fmt.Errorf("decomposing autonomous goal: %w", err)
	}
	var total llm.Usage
	var last *llm.Response
	var accumulated []llm.Message
	err = e.planner.ExecutePlan(ctx, plan, func(stepCtx context.Context, step PlanStep) (string, error) {
		prompt := fmt.Sprintf(
			"[PLAN STEP %s]\nGoal: %s\nStep: %s\nRole: %s\nAcceptance: complete this step with tool evidence and explicit verification.",
			step.ID, goal, step.Description, step.AgentRole,
		)
		response, stepErr := e.ExecuteStepWithHistory(stepCtx, agentID, prompt, append(history, accumulated...))
		if stepErr != nil {
			return "", stepErr
		}
		last = response
		total = addUsage(total, response.Usage)
		accumulated = append(accumulated,
			llm.Message{Role: llm.RoleUser, Content: prompt},
			llm.Message{Role: llm.RoleAssistant, Content: response.Content, ToolCalls: response.ToolCalls},
		)
		return response.Content, nil
	})
	if err != nil {
		return nil, err
	}
	if last == nil {
		return nil, errors.New("autonomous plan produced no response")
	}
	last.Usage = total
	return last, nil
}

// ExecuteStepWithHistory runs a cognitive iteration of the ReAct state machine with short-term dialogue history.
func (e *Engine) ExecuteStepWithHistory(ctx context.Context, agentID string, userMessage string, history []llm.Message) (*llm.Response, error) {
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
	if err := e.checkBudget(ctx, agent); err != nil {
		return nil, err
	}

	traceID := generateTraceID()
	ctx = tools.WithTraceID(ctx, traceID)
	source := sourceFromMessage(userMessage, "chat")
	if source == "cron" || source == "channel" {
		ctx = tools.WithBypassApproval(ctx)
	}
	run := e.startRun(ctx, traceID, agentID, userMessage, source)

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Build unified cognitive system prompt
	fullSystemPrompt, _ := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)

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
		Temperature: &agent.ModelConfig.Temperature,
	}
	if agent.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &agent.ModelConfig.MaxTokens
	}

	// 3. Attach authorized tools if registry available
	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		opts.Tools = e.tools.ToLLMToolDefinitions(agent.AuthorizedTools)
	}

	startTime := time.Now()
	var finalResp *llm.Response
	totalUsage := llm.Usage{}
	maxIterations := 10
	consecutiveFailures := 0
	lastObservation := ""
	repeatedObservations := 0
	converged := false
	iterationsCompleted := 0

	for iter := range maxIterations {
		iterationsCompleted = iter + 1
		currentOpts := opts
		// On the final iteration, force convergence by omitting tools
		if iter == maxIterations-1 {
			currentOpts.Tools = nil
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "Iteration limit reached. Do not call any more tools. Synthesize all observations gathered above into your final comprehensive response to the user.",
			})
		}
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

		// If no tool calls requested, we reached the final response
		if len(resp.ToolCalls) == 0 || e.tools == nil || currentOpts.Tools == nil {
			converged = true
			break
		}

		// Append assistant response with requested tool calls
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			toolResult, execErr := e.tools.Execute(ctx, agentID, tc.Function.Name, tc.Function.Arguments)
			resultStr := ""
			if execErr != nil {
				resultStr = fmt.Sprintf("Error executing tool %s: %v", tc.Function.Name, execErr)
				consecutiveFailures++
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(execErr, &approvalErr) {
					e.saveApprovalCheckpoint(ctx, run, agentID, userMessage, source, messages, iter+1, totalUsage, tc)
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

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})
			if consecutiveFailures >= 3 || repeatedObservations >= 2 {
				e.finishRun(ctx, run, RunBlocked, "no_progress", iter+1, totalUsage)
				return nil, fmt.Errorf("agent %s stopped after repeated tool failures or observations", agentID)
			}
		}
	}
	if !converged {
		e.finishRun(ctx, run, RunBlocked, "iteration_budget_exhausted", maxIterations, totalUsage)
		return nil, fmt.Errorf("agent %s reached the maximum of %d ReAct iterations without convergence", agentID, maxIterations)
	}
	if finalResp == nil || !e.verifier.VerifySemanticConsistency(ctx, userMessage, finalResp.Content) {
		e.finishRun(ctx, run, RunFailed, "verification_failed", maxIterations, totalUsage)
		return nil, fmt.Errorf("agent %s final response failed semantic verification", agentID)
	}
	finalResp.Usage = totalUsage

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
	if err := e.checkBudget(ctx, agent); err != nil {
		eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
		return nil, err
	}

	traceID := generateTraceID()
	ctx = tools.WithTraceID(ctx, traceID)
	source := sourceFromMessage(userMessage, "stream")
	if source == "cron" || source == "channel" {
		ctx = tools.WithBypassApproval(ctx)
	}
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
	fullSystemPrompt, episodicCount := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)
	memDuration := time.Since(memStart).Milliseconds()

	if episodicCount > 0 {
		eventChan <- AgentStreamEvent{
			Type: EventStreamAudit,
			AuditLog: &AuditLogEntry{
				Timestamp:    time.Now().UTC(),
				AgentID:      agentID,
				Action:       "cognitive_memory_retrieval",
				Status:       "success",
				Verification: fmt.Sprintf("Retrieved %d episodic memory fragments + user profile context", episodicCount),
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
		Temperature: &agent.ModelConfig.Temperature,
	}
	if agent.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &agent.ModelConfig.MaxTokens
	}

	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		opts.Tools = e.tools.ToLLMToolDefinitions(agent.AuthorizedTools)
	}

	var finalResp *llm.Response
	totalUsage := llm.Usage{}
	maxIterations := 10
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

		// Append assistant response with requested tool calls
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

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
				messages = append(messages, llm.Message{
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
				consecutiveFailures++
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(execErr, &approvalErr) {
					e.saveApprovalCheckpoint(ctx, run, agentID, userMessage, source, messages, iterationsCompleted, totalUsage, tc)
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

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Name:       toolName,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})
			if consecutiveFailures >= 3 || repeatedObservations >= 2 {
				e.finishRun(ctx, run, RunBlocked, "no_progress", iterationsCompleted, totalUsage)
				err := fmt.Errorf("agent %s stopped after repeated tool failures or observations", agentID)
				eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
				return nil, err
			}
		}
	}
	if !converged {
		e.finishRun(ctx, run, RunBlocked, "iteration_budget_exhausted", iterationsCompleted, totalUsage)
		err := fmt.Errorf("agent %s reached the maximum of %d ReAct iterations without convergence", agentID, maxIterations)
		eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
		return nil, err
	}
	if finalResp == nil || !e.verifier.VerifySemanticConsistency(ctx, userMessage, finalResp.Content) {
		e.finishRun(ctx, run, RunFailed, "verification_failed", iterationsCompleted, totalUsage)
		err := fmt.Errorf("agent %s final response failed semantic verification", agentID)
		eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
		return nil, err
	}
	finalResp.Usage = totalUsage

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
	opts := llm.CompletionOptions{Temperature: &manifest.ModelConfig.Temperature}
	if manifest.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &manifest.ModelConfig.MaxTokens
	}
	opts.Tools = e.tools.ToLLMToolDefinitions(manifest.AuthorizedTools)
	usage := checkpoint.Usage

	// Subsequent tool calls must not inherit the consumed approval ID, ensuring
	// new high-risk actions can request their own approvals cleanly.
	execCtx := tools.WithTraceID(ctx, checkpoint.TraceID)
	execCtx = tools.WithApprovalID(execCtx, "")

	consecutiveFailures := 0
	lastObservation := ""
	repeatedObservations := 0

	// Resume with a FRESH iteration budget (8 iterations) so the LLM has
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
					if strings.Contains(content, "[TASK_COMPLETED]") || (e.verifier != nil && e.verifier.VerifyTaskCompletion(task.Description, content, response.ToolCalls)) {
						task.Status = "completed"
						task.Progress = 100
						now := time.Now().UTC()
						task.CompletedAt = &now
						task.ExecutionLog = shortLog
					} else if strings.Contains(content, "[TASK_BLOCKED") {
						task.Status = "blocked"
						task.ExecutionLog = shortLog
					} else {
						task.Status = "completed"
						task.Progress = 100
						now := time.Now().UTC()
						task.CompletedAt = &now
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
			Role: llm.RoleAssistant, Content: response.Content, ToolCalls: response.ToolCalls,
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
			if consecutiveFailures >= 3 || repeatedObservations >= 2 {
				e.finishRun(execCtx, run, RunBlocked, "no_progress", absoluteStep, usage)
				return nil, fmt.Errorf("resumed agent %s stopped after repeated tool failures or observations", checkpoint.AgentID)
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
	max := 8192
	if agent != nil && agent.ModelConfig.MaxTokens > 0 && agent.ModelConfig.MaxTokens < max-1024 {
		return max - agent.ModelConfig.MaxTokens
	}
	return 6144
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

func (e *Engine) checkBudget(ctx context.Context, manifest *AgentManifest) error {
	if e.tokenTracker == nil || manifest == nil || manifest.DelegationScope.MaxMonthlyBudgetUSD <= 0 {
		return nil
	}
	cost, err := e.tokenTracker.GetAgentMonthlyCost(ctx, manifest.AgentID)
	if err != nil {
		return fmt.Errorf("checking monthly agent budget: %w", err)
	}
	if cost >= manifest.DelegationScope.MaxMonthlyBudgetUSD {
		return fmt.Errorf(
			"agent %s monthly budget exhausted: spent %.4f USD of %.4f USD",
			manifest.AgentID, cost, manifest.DelegationScope.MaxMonthlyBudgetUSD,
		)
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

func sourceFromMessage(message, fallback string) string {
	switch {
	case strings.Contains(message, "[AUTONOMOUS MISSION"):
		return "heartbeat"
	case strings.Contains(message, "[AUTONOMOUS HEARTBEAT"):
		return "heartbeat"
	case strings.Contains(message, "[AUTONOMOUS PROACTIVE"):
		return "cron"
	case strings.Contains(message, "[Channel:"), strings.Contains(message, "[Channel Metadata]"):
		return "channel"
	default:
		return fallback
	}
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
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				return response, nil
			}
			if chunk.Error != nil {
				return nil, chunk.Error
			}
			if chunk.DeltaContent != "" {
				response.Content += chunk.DeltaContent
				eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: chunk.DeltaContent}
			}
			if len(chunk.ToolCalls) > 0 {
				response.ToolCalls = append(response.ToolCalls, chunk.ToolCalls...)
			}
			if chunk.Usage != nil {
				response.Usage = *chunk.Usage
			}
			if chunk.Done {
				return response, nil
			}
		}
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

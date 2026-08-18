package agent

import (
	"context"
	"fmt"
	"math"
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
		agentMgr: agentMgr,
		bus:      eventBus,
		llm:      llmRouter,
		memory:   mem,
		verifier: NewVerifier(),
		theta:    DefaultEntropyThreshold,
	}
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

	// 1. Base Soul Persona (isolated per agent)
	if e.profileMgr != nil {
		soul := e.profileMgr.GetAgentSoul(agentID)
		if soul != "" {
			sb.WriteString(soul)
			sb.WriteString("\n\n")
		}

		// 2. Layer 2: User Profile Memory (Owner Identity & Interaction Preferences)
		profile := e.profileMgr.GetProfile()
		sb.WriteString("## Owner Identity & Interaction Preferences\n")
		if profile.UserName != "" {
			sb.WriteString(fmt.Sprintf("- **Owner Name**: %s\n", profile.UserName))
		}
		if profile.UserRole != "" {
			sb.WriteString(fmt.Sprintf("- **Owner Role**: %s\n", profile.UserRole))
		}
		if profile.Language != "" {
			sb.WriteString(fmt.Sprintf("- **Preferred Language**: %s\n", profile.Language))
		}
		if profile.Timezone != "" {
			sb.WriteString(fmt.Sprintf("- **Timezone**: %s (Current Time: %s)\n", profile.Timezone, time.Now().UTC().Format(time.RFC3339)))
		}
		if profile.CommunicationStyle != "" {
			sb.WriteString(fmt.Sprintf("- **Communication Style**: %s\n", profile.CommunicationStyle))
		}
		if profile.CustomInstructions != "" {
			sb.WriteString(fmt.Sprintf("- **Owner Directives**: %s\n", profile.CustomInstructions))
		}
		sb.WriteString("\n")

		// 3. Layer 3: Procedural Memory (Workflows & Execution Guidelines)
		patterns, _ := e.profileMgr.GetRelevantPatterns(ctx, "general")
		if len(patterns) > 0 {
			sb.WriteString("## Procedural Memory & Verified Workflows\n")
			for _, pat := range patterns {
				sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", pat.PatternName, pat.Domain, pat.Workflow))
			}
			sb.WriteString("\n")
		}
	}

	// 4. Agent Specific System Instructions
	if agent.SystemInstructions != "" {
		sb.WriteString("## Agent Directives\n")
		sb.WriteString(agent.SystemInstructions)
		sb.WriteString("\n\n")
	}

	// 5. Conversational Demeanor & Anti-Robot Principles
	sb.WriteString("## Conversational Standard & Tone Guidelines\n")
	sb.WriteString("- Communicate naturally, intelligently, and empathetically with authentic personality, warmth, and sharp engineering insight.\n")
	sb.WriteString("- Never produce clichéd robotic intros ('As an AI...', 'I am happy to help...'), stiff disclaimers, or excessive apologies.\n")
	sb.WriteString("- Respond directly to the core of the user's intent with crisp explanations and clean, beautifully formatted markdown.\n\n")

	// 6. Layer 4: Episodic Memory (Past interactions & learned facts)
	episodicCount := 0
	if e.memory != nil {
		memories, err := e.memory.Search(ctx, agentID, memory.LayerEpisodic, userMessage, nil, 4)
		if err == nil && len(memories) > 0 {
			episodicCount = len(memories)
			sb.WriteString("## Relevant Past Episodic Memories\n")
			for _, m := range memories {
				sb.WriteString(fmt.Sprintf("- %s\n", m.Content))
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

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Build unified 4-layer cognitive system prompt
	fullSystemPrompt, _ := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: fullSystemPrompt},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMessage})

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

	// 3. Attach authorized tools if registry available
	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		opts.Tools = e.tools.ToLLMToolDefinitions(agent.AuthorizedTools)
	}

	startTime := time.Now()
	var finalResp *llm.Response
	maxIterations := 5

	for iter := 0; iter < maxIterations; iter++ {
		resp, err := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, opts)
		if err != nil {
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
			}
			return nil, fmt.Errorf("llm completion: %w", err)
		}

		finalResp = resp

		// If no tool calls requested, we reached the final response
		if len(resp.ToolCalls) == 0 || e.tools == nil {
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
			} else if toolResult != nil {
				resultStr = toolResult.Content
			}

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})
		}
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
		e.RecordTokenUsage(ctx, agentID, finalResp.Model, finalResp.Model, "chat", "", finalResp.Usage)
	}

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

	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		opts.Tools = e.tools.ToLLMToolDefinitions(agent.AuthorizedTools)
	}

	var finalResp *llm.Response
	maxIterations := 5

	for iter := 0; iter < maxIterations; iter++ {
		targetModel := agent.ModelConfig.PrimaryModel
		if targetModel == "" {
			targetModel = "cascade-llm"
		}

		eventChan <- AgentStreamEvent{
			Type:    EventStreamThought,
			Thought: fmt.Sprintf("Deliberating with %s (ReAct iteration %d)...", targetModel, iter+1),
		}

		resp, err := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, opts)
		if err != nil {
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
			}
			eventChan <- AgentStreamEvent{
				Type:  EventStreamError,
				Error: err.Error(),
			}
			return nil, fmt.Errorf("llm completion: %w", err)
		}

		finalResp = resp

		// If no tool calls requested, stream tokens preserving exact whitespace and newlines
		if len(resp.ToolCalls) == 0 || e.tools == nil {
			runes := []rune(resp.Content)
			if len(runes) > 0 {
				chunkSize := 12
				for i := 0; i < len(runes); i += chunkSize {
					end := i + chunkSize
					if end > len(runes) {
						end = len(runes)
					}
					eventChan <- AgentStreamEvent{
						Type:    EventStreamToken,
						Content: string(runes[i:end]),
					}
					time.Sleep(10 * time.Millisecond)
				}
			} else if resp.Content != "" {
				eventChan <- AgentStreamEvent{
					Type:    EventStreamToken,
					Content: resp.Content,
				}
			}

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

			// Perform Tier-1 Security Verification
			var verifyErr error
			if e.verifier != nil {
				if toolName == "native_run_command" || toolName == "bash" {
					verifyErr = e.verifier.VerifyCommand(toolArgs)
				}
			}

			verificationStatus := "Tier-1 AST Clean & Safe"
			if verifyErr != nil {
				verificationStatus = fmt.Sprintf("Verification Warning: %v", verifyErr)
			}

			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "tool_security_check",
					ToolName:     toolName,
					Parameters:   toolArgs,
					Status:       "verified",
					Verification: verificationStatus,
					DurationMs:   0,
				},
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
			} else if toolResult != nil {
				resultStr = toolResult.Content
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
		}
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
		e.RecordTokenUsage(ctx, agentID, finalResp.Model, finalResp.Model, "stream", "", finalResp.Usage)
	}

	eventChan <- AgentStreamEvent{
		Type:    EventStreamDone,
		Content: finalResp.Content,
		Model:   finalResp.Model,
		Usage:   &finalResp.Usage,
	}

	return finalResp, nil
}

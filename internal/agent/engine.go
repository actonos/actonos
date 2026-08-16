package agent

import (
	"context"
	"fmt"
	"math"
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
	agentMgr *AgentManager
	bus      *bus.EventBus
	llm      *llm.ModelCascadeRouter
	memory   *memory.HybridEngine
	tools    *tools.ToolRegistry
	theta    float64 // Entropy threshold
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
		theta:    DefaultEntropyThreshold,
	}
}

// SetToolRegistry attaches the system tool registry to enable tool execution.
func (e *Engine) SetToolRegistry(r *tools.ToolRegistry) {
	e.tools = r
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
	agent, err := e.agentMgr.Get(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", agentID, err)
	}

	if agent.Status != StatusActive {
		return nil, fmt.Errorf("agent %s is not active (status=%s)", agentID, agent.Status)
	}

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Context retrieval from Hybrid Memory (if available)
	var memoryContext string
	if e.memory != nil {
		memories, err := e.memory.Search(ctx, agentID, memory.LayerEpisodic, userMessage, nil, 3)
		if err == nil && len(memories) > 0 {
			memoryContext = "\nRelevant past memories:\n"
			for _, m := range memories {
				memoryContext += fmt.Sprintf("- %s\n", m.Content)
			}
		}
	}

	// 2. Build system instructions with context
	fullSystemPrompt := agent.SystemInstructions
	if memoryContext != "" {
		fullSystemPrompt += "\n" + memoryContext
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: fullSystemPrompt},
		{Role: llm.RoleUser, Content: userMessage},
	}

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

	// 4. Store memory fragment asynchronously
	if e.memory != nil && finalResp != nil && finalResp.Content != "" {
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

	if e.bus != nil && finalResp != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, agentID, map[string]any{
			"duration_ms": time.Since(startTime).Milliseconds(),
			"tokens":      finalResp.Usage.TotalTokens,
		}))
	}

	return finalResp, nil
}

package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

// DefaultEntropyThreshold is θ for uncertainty-gated branching.
const DefaultEntropyThreshold = 0.65

// Engine orchestrates the POMDP & ReAct cognitive loop for agents.
type Engine struct {
	agentMgr *AgentManager
	bus      *bus.EventBus
	llm      *llm.ModelCascadeRouter
	memory   *memory.HybridEngine
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

	startTime := time.Now()
	resp, err := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, opts)
	if err != nil {
		if e.bus != nil {
			e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
		}
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	// 4. Store memory fragment asynchronously
	if e.memory != nil && resp.Content != "" {
		go func() {
			_, _ = e.memory.StoreMemory(
				context.Background(),
				agentID,
				memory.LayerEpisodic,
				fmt.Sprintf("User asked: %s | Response: %s", userMessage, resp.Content),
				nil,
				map[string]any{"timestamp": time.Now().UTC()},
				1.0,
			)
		}()
	}

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, agentID, map[string]any{
			"duration_ms": time.Since(startTime).Milliseconds(),
			"tokens":      resp.Usage.TotalTokens,
		}))
	}

	return resp, nil
}

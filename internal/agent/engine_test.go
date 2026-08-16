package agent

import (
	"context"
	"math"
	"testing"

	"github.com/actonos/actonos/internal/llm"
)

func TestEngine_ExecuteStep(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, _ := NewAgentManager(db, eventBus)

	llmRouter := llm.NewModelCascadeRouter()
	mockLLM := llm.NewMockProvider("claude-3-7-sonnet", "Hello, I am ready to help you.")
	llmRouter.RegisterProvider("claude-3-7-sonnet", mockLLM)

	engine := NewEngine(agentMgr, eventBus, llmRouter, nil)
	ctx := context.Background()

	created, err := agentMgr.Create(ctx, AgentManifest{
		Name:                "Assistant",
		SystemInstructions: "You are a helpful assistant.",
		ModelConfig: llm.ModelConfig{
			PrimaryModel: "claude-3-7-sonnet",
		},
	})
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	resp, err := engine.ExecuteStep(ctx, created.AgentID, "Hello!")
	if err != nil {
		t.Fatalf("ExecuteStep failed: %v", err)
	}

	if resp.Content != "Hello, I am ready to help you." {
		t.Fatalf("unexpected response content: %s", resp.Content)
	}
}

func TestCalculateEntropy(t *testing.T) {
	// Deterministic probability distribution [1.0] -> Entropy = 0
	h1 := CalculateEntropy([]float64{1.0})
	if math.Abs(h1-0.0) > 0.001 {
		t.Fatalf("expected entropy 0 for deterministic outcome, got %f", h1)
	}

	// Uniform distribution over 2 outcomes [0.5, 0.5] -> Entropy = 1.0 bit
	h2 := CalculateEntropy([]float64{0.5, 0.5})
	if math.Abs(h2-1.0) > 0.001 {
		t.Fatalf("expected entropy 1.0 for uniform binary outcome, got %f", h2)
	}
}

package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTokenTracker_RecordAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	db, err := Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	tracker := NewTokenTracker(db.SQLDB())
	ctx := context.Background()

	// Record 1: GPT-4o
	err = tracker.Record(ctx, TokenUsageRecord{
		AgentID:          "agent_system_core",
		Model:            "gpt-4o",
		Provider:         "openai",
		PromptTokens:     1000,
		CompletionTokens: 500,
		Source:           "chat",
	})
	if err != nil {
		t.Fatalf("recording token 1: %v", err)
	}

	// Record 2: Claude 3.5 Sonnet
	err = tracker.Record(ctx, TokenUsageRecord{
		AgentID:          "agent_dev",
		Model:            "claude-3-5-sonnet",
		Provider:         "anthropic",
		PromptTokens:     2000,
		CompletionTokens: 1000,
		Source:           "cron",
	})
	if err != nil {
		t.Fatalf("recording token 2: %v", err)
	}

	// Get Summary
	summary, err := tracker.GetSummary(ctx)
	if err != nil {
		t.Fatalf("getting summary: %v", err)
	}

	if summary.TotalPromptTokens != 3000 {
		t.Errorf("expected 3000 total prompt tokens, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCompletionTokens != 1500 {
		t.Errorf("expected 1500 total completion tokens, got %d", summary.TotalCompletionTokens)
	}
	if summary.TotalTokens != 4500 {
		t.Errorf("expected 4500 total tokens, got %d", summary.TotalTokens)
	}
	if summary.TotalCostUSD <= 0 {
		t.Errorf("expected positive cost, got %f", summary.TotalCostUSD)
	}
	if len(summary.ByModel) != 2 {
		t.Errorf("expected 2 models, got %d", len(summary.ByModel))
	}
	if len(summary.ByAgent) != 2 {
		t.Errorf("expected 2 agents, got %d", len(summary.ByAgent))
	}

	// Test Agent Monthly Cost
	cost, err := tracker.GetAgentMonthlyCost(ctx, "agent_system_core")
	if err != nil {
		t.Fatalf("getting agent monthly cost: %v", err)
	}
	if cost <= 0 {
		t.Errorf("expected positive agent cost, got %f", cost)
	}

	history, err := tracker.GetHistory(ctx, 10, "agent_system_core", "chat")
	if err != nil {
		t.Fatalf("getting filtered history: %v", err)
	}
	if len(history) != 1 || history[0].AgentID != "agent_system_core" || history[0].Source != "chat" {
		t.Fatalf("unexpected filtered history: %+v", history)
	}
	all, err := tracker.GetHistory(ctx, 0, "all", "all")
	if err != nil || len(all) != 2 {
		t.Fatalf("unexpected full history: %+v err=%v", all, err)
	}

	nilTracker := NewTokenTracker(nil)
	empty, err := nilTracker.GetHistory(ctx, 10, "", "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("nil tracker history should be empty: %+v err=%v", empty, err)
	}
}

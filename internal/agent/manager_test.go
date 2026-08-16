package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

func setupTestDB(t *testing.T) (*memory.DB, *bus.EventBus) {
	t.Helper()
	dir := t.TempDir()
	db, err := memory.Open(filepath.Join(dir, "agent_test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	eventBus := bus.NewEventBus()
	t.Cleanup(func() { eventBus.Close() })

	return db, eventBus
}

func TestAgentManager_CRUD(t *testing.T) {
	db, eventBus := setupTestDB(t)

	mgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatalf("failed to create agent manager: %v", err)
	}

	ctx := context.Background()

	// 1. Create Agent
	manifest := AgentManifest{
		Name:        "Senior Architect",
		Description: "Designs distributed systems",
		AvatarIcon:  "code-bracket",
		ModelConfig: llm.ModelConfig{
			PrimaryModel: "claude-3-7-sonnet",
			Temperature:  0.2,
		},
		SystemInstructions: "You are a software architect.",
		AuthorizedTools:    []string{"bash", "web_search"},
		DelegationScope: DelegationScope{
			MaxMonthlyBudgetUSD:   100.0,
			AllowedWorkspacePaths: []string{"/data/workspace/app"},
			RequireHumanApproval:  ApprovalHigh,
		},
	}

	created, err := mgr.Create(ctx, manifest)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	if created.AgentID == "" {
		t.Fatalf("expected generated AgentID, got empty")
	}
	if created.Status != StatusActive {
		t.Fatalf("expected StatusActive, got %s", created.Status)
	}

	// 2. Get Agent
	retrieved, err := mgr.Get(ctx, created.AgentID)
	if err != nil {
		t.Fatalf("failed to get agent: %v", err)
	}
	if retrieved.Name != "Senior Architect" {
		t.Fatalf("expected name 'Senior Architect', got '%s'", retrieved.Name)
	}

	// 3. Update Agent
	retrieved.Description = "Updated description"
	updated, err := mgr.Update(ctx, *retrieved)
	if err != nil {
		t.Fatalf("failed to update agent: %v", err)
	}
	if updated.Description != "Updated description" {
		t.Fatalf("expected updated description, got '%s'", updated.Description)
	}

	// 4. Start & Stop
	if err := mgr.Stop(ctx, created.AgentID); err != nil {
		t.Fatalf("failed to stop agent: %v", err)
	}
	stopped, _ := mgr.Get(ctx, created.AgentID)
	if stopped.Status != StatusStopped {
		t.Fatalf("expected StatusStopped, got %s", stopped.Status)
	}

	if err := mgr.Start(ctx, created.AgentID); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	started, _ := mgr.Get(ctx, created.AgentID)
	if started.Status != StatusActive {
		t.Fatalf("expected StatusActive, got %s", started.Status)
	}

	// 5. List Agents
	list, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("failed to list agents: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 agent in list, got %d", len(list))
	}

	// 6. Delete Agent
	if err := mgr.Delete(ctx, created.AgentID); err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	_, err = mgr.Get(ctx, created.AgentID)
	if err == nil {
		t.Fatalf("expected error getting deleted agent, got nil")
	}
}

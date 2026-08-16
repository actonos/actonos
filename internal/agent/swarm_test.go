package agent

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/llm"
)

func TestSwarmManager_SpawnSubAgent(t *testing.T) {
	db, eventBus := setupTestDB(t)

	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatalf("failed to create agent manager: %v", err)
	}

	llmRouter := llm.NewModelCascadeRouter()
	mockLLM := llm.NewMockProvider("claude-3-7-sonnet", "Subtask analysis result: all checks passed.")
	llmRouter.RegisterProvider("claude-3-7-sonnet", mockLLM)

	swarm := NewSwarmManager(agentMgr, eventBus, llmRouter, nil, 4)

	ctx := context.Background()

	// Create Parent Agent
	parent, err := agentMgr.Create(ctx, AgentManifest{
		Name:            "Parent Agent",
		AuthorizedTools: []string{"bash", "git", "file_ops"},
		ModelConfig: llm.ModelConfig{
			PrimaryModel: "claude-3-7-sonnet",
		},
	})
	if err != nil {
		t.Fatalf("failed to create parent agent: %v", err)
	}

	// 1. Valid sub-task within authorized tool scope
	subTask := SubTask{
		Title:           "Run linter",
		Prompt:          "Check Go codebase syntax",
		AuthorizedTools: []string{"bash"},
		Timeout:         5 * time.Second,
	}

	resultChan, err := swarm.SpawnSubAgent(ctx, parent.AgentID, subTask)
	if err != nil {
		t.Fatalf("expected spawn success, got: %v", err)
	}

	select {
	case result, ok := <-resultChan:
		if !ok {
			t.Fatal("result channel closed without result")
		}
		if result.Status != "success" {
			t.Fatalf("expected status 'success', got '%s' (err: %s)", result.Status, result.Error)
		}
		if result.Output != "Subtask analysis result: all checks passed." {
			t.Fatalf("unexpected output: %s", result.Output)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sub-agent execution")
	}

	// 2. Zero-Trust scope violation: sub-agent requests unauthorized tool
	forbiddenTask := SubTask{
		Title:           "Unauthorized DB wipe",
		Prompt:          "Drop prod table",
		AuthorizedTools: []string{"db_admin_drop"},
	}

	_, err = swarm.SpawnSubAgent(ctx, parent.AgentID, forbiddenTask)
	if err == nil {
		t.Fatal("expected error due to Zero-Trust scope violation, got nil")
	}
}

func TestSwarmManager_DispatchSwarm(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, _ := NewAgentManager(db, eventBus)

	llmRouter := llm.NewModelCascadeRouter()
	mockLLM := llm.NewMockProvider("claude-3-7-sonnet", "Chunk processed")
	llmRouter.RegisterProvider("claude-3-7-sonnet", mockLLM)

	swarm := NewSwarmManager(agentMgr, eventBus, llmRouter, nil, 4)
	ctx := context.Background()

	parent, _ := agentMgr.Create(ctx, AgentManifest{
		Name:            "Orchestrator",
		AuthorizedTools: []string{"*"},
		ModelConfig:     llm.ModelConfig{PrimaryModel: "claude-3-7-sonnet"},
	})

	tasks := []SubTask{
		{Title: "Task 1", Prompt: "Process 1"},
		{Title: "Task 2", Prompt: "Process 2"},
		{Title: "Task 3", Prompt: "Process 3"},
	}

	resultsChan, err := swarm.DispatchSwarm(ctx, parent.AgentID, tasks)
	if err != nil {
		t.Fatalf("DispatchSwarm failed: %v", err)
	}

	var collected []SubTaskResult
	for res := range resultsChan {
		collected = append(collected, res)
	}

	if len(collected) != 3 {
		t.Fatalf("expected 3 completed sub-tasks, got %d", len(collected))
	}
}

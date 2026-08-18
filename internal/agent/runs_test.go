package agent

import (
	"context"
	"database/sql"
	"testing"

	"github.com/actonos/actonos/internal/llm"
	_ "modernc.org/sqlite"
)

func newRunStoreForTest(t *testing.T) *RunStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE agent_runs (
			id TEXT PRIMARY KEY, trace_id TEXT, agent_id TEXT, goal TEXT, source TEXT,
			status TEXT, termination_reason TEXT, iterations INTEGER,
			prompt_tokens INTEGER, completion_tokens INTEGER, total_tokens INTEGER,
			started_at DATETIME, updated_at DATETIME, completed_at DATETIME,
			checkpoint_json TEXT
		);
		CREATE TABLE run_events (
			id TEXT PRIMARY KEY, run_id TEXT, trace_id TEXT, step INTEGER,
			type TEXT, status TEXT, tool_name TEXT, data_json TEXT,
			duration_ms INTEGER, created_at DATETIME
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	return NewRunStore(db)
}

func TestRunStoreLifecycleCheckpointAndEvents(t *testing.T) {
	ctx := context.Background()
	store := newRunStoreForTest(t)
	run, err := store.Start(ctx, "trace-1", "agent-1", "finish mission", "test")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunRunning || run.ID == "" {
		t.Fatalf("unexpected run: %+v", run)
	}

	if err := store.AppendEvent(ctx, RunEvent{
		RunID: run.ID, TraceID: run.TraceID, Step: 1, Type: "tool",
		Status: "pending", ToolName: "write_file", Data: map[string]any{"path": "a.txt"},
	}); err != nil {
		t.Fatal(err)
	}
	run.Status = RunApprovalPending
	run.TerminationReason = "approval_required"
	run.Iterations = 2
	run.PromptTokens, run.CompletionTokens, run.TotalTokens = 5, 3, 8
	if err := store.Finish(ctx, run); err != nil {
		t.Fatal(err)
	}
	checkpoint := RunCheckpoint{
		RunID: run.ID, TraceID: run.TraceID, AgentID: run.AgentID, Goal: run.Goal,
		Source: run.Source, Iteration: 2, Usage: llm.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "finish mission"}},
		PendingTool: llm.ToolCall{ID: "call-1", Type: "function", Function: llm.FunctionCall{
			Name: "write_file", Arguments: []byte(`{"path":"a.txt","content":"ok"}`),
		}},
	}
	if err := store.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	loaded, loadedRun, err := store.LoadCheckpointByTrace(ctx, run.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != run.ID || loaded.PendingTool.Function.Name != "write_file" ||
		loaded.Usage.TotalTokens != 8 || loadedRun.Status != RunApprovalPending {
		t.Fatalf("checkpoint mismatch: checkpoint=%+v run=%+v", loaded, loadedRun)
	}

	events, err := store.Events(ctx, run.ID)
	if err != nil || len(events) != 1 || events[0].Data["path"] != "a.txt" {
		t.Fatalf("unexpected events: %+v err=%v", events, err)
	}
	runs, err := store.List(ctx, 0)
	if err != nil || len(runs) != 1 || runs[0].TotalTokens != 8 {
		t.Fatalf("unexpected runs: %+v err=%v", runs, err)
	}

	run.Status = RunCompleted
	run.TerminationReason = "goal_completed"
	if err := store.Finish(ctx, run); err != nil {
		t.Fatal(err)
	}
	runs, err = store.List(ctx, 10)
	if err != nil || runs[0].CompletedAt == nil {
		t.Fatalf("terminal run was not completed: %+v err=%v", runs, err)
	}
}

func TestRunStoreMissingCheckpoint(t *testing.T) {
	ctx := context.Background()
	store := newRunStoreForTest(t)
	run, err := store.Start(ctx, "trace-2", "agent-1", "goal", "test")
	if err != nil {
		t.Fatal(err)
	}
	run.Status = RunApprovalPending
	if err := store.Finish(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadCheckpointByTrace(ctx, run.TraceID); err == nil {
		t.Fatal("expected missing checkpoint error")
	}
}

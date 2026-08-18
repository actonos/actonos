package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTaskManagerForTest(t *testing.T) *TaskManager {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	manager, err := NewTaskManager(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestTaskManagerCRUDFilteringAndMarkdown(t *testing.T) {
	manager := newTaskManagerForTest(t)
	ctx := context.Background()
	existing, err := manager.ListTasks(ctx, "all", "all")
	if err != nil || len(existing) != 2 {
		t.Fatalf("default task seeding failed: count=%d err=%v", len(existing), err)
	}

	task, err := manager.CreateTask(ctx, AutonomousTask{
		Title: "Coverage mission", Description: "Exercise task lifecycle",
		Status: "in_progress", Priority: "p0_critical", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.SessionID == "" || task.AssignedAgentID != "auto" {
		t.Fatalf("defaults were not applied: %+v", task)
	}

	got, err := manager.GetTask(ctx, task.ID)
	if err != nil || got.Title != task.Title {
		t.Fatalf("GetTask failed: task=%+v err=%v", got, err)
	}
	got.Status = "completed"
	got.ExecutionLog = "verified"
	if err := manager.UpdateTask(ctx, *got); err != nil {
		t.Fatal(err)
	}
	got, err = manager.GetTask(ctx, task.ID)
	if err != nil || got.Progress != 100 || got.CompletedAt == nil {
		t.Fatalf("completion fields not applied: task=%+v err=%v", got, err)
	}

	completed, err := manager.ListTasks(ctx, "completed", "p0_critical")
	if err != nil || len(completed) != 1 {
		t.Fatalf("filtering failed: tasks=%+v err=%v", completed, err)
	}
	markdown, err := os.ReadFile(filepath.Join(manager.workspaceDir, "TASKS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "Coverage mission") || !strings.Contains(string(markdown), "Status: completed") {
		t.Fatalf("markdown was not synchronized:\n%s", markdown)
	}

	if err := manager.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.GetTask(ctx, task.ID); err != sql.ErrNoRows {
		t.Fatalf("expected deleted task to be absent, got %v", err)
	}
}

func TestTaskManagerHeartbeatAndNilDatabase(t *testing.T) {
	workspace := t.TempDir()
	manager := &TaskManager{workspaceDir: workspace}
	ctx := context.Background()
	if tasks, err := manager.ListTasks(ctx, "", ""); err != nil || len(tasks) != 0 {
		t.Fatalf("nil database list failed: tasks=%v err=%v", tasks, err)
	}
	if _, err := manager.GetTask(ctx, "missing"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
	if err := manager.SaveHeartbeatConfig(ctx, HeartbeatConfig{Directives: "stay healthy", IntervalMinutes: 3}); err != nil {
		t.Fatal(err)
	}
	cfg, err := manager.GetHeartbeatConfig(ctx)
	if err != nil || cfg.Directives != "stay healthy" || !cfg.Enabled || !cfg.AutoDelegate {
		t.Fatalf("unexpected heartbeat config: cfg=%+v err=%v", cfg, err)
	}
}

func TestTaskManagerDeleteCleansUpSessionHistory(t *testing.T) {
	manager := newTaskManagerForTest(t)
	ctx := context.Background()

	// Create a task
	task, err := manager.CreateTask(ctx, AutonomousTask{
		Title:       "Greek Mythology Task",
		Description: "Write stories about Greek gods into a file",
		Priority:    "p2_normal",
		CreatedBy:   "user",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Insert messages for this task's session into the database
	_, err = manager.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			tool_calls_json TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			title TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	convID := task.SessionID
	_, _ = manager.db.ExecContext(ctx, `INSERT INTO conversations (id, agent_id, title) VALUES (?, 'agent_system_core', 'Task Chat')`, convID)
	_, _ = manager.db.ExecContext(ctx, `INSERT INTO messages (id, conversation_id, agent_id, role, content) VALUES ('msg_1', ?, 'agent_system_core', 'assistant', 'Here is a story of Zeus')`, convID)

	// Verify messages exist before deletion
	var count int
	_ = manager.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages WHERE conversation_id = ?", convID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 message before delete, got %d", count)
	}

	// Delete task
	if err := manager.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}

	// Verify messages and conversation were purged
	_ = manager.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages WHERE conversation_id = ?", convID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 messages after task deletion, got %d", count)
	}

	var convCount int
	_ = manager.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM conversations WHERE id = ?", convID).Scan(&convCount)
	if convCount != 0 {
		t.Fatalf("expected 0 conversations after task deletion, got %d", convCount)
	}
}

package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

func TestHeartbeatRoutineAndRunLedger(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK"))
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Hour)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "ok" || run.ID == "" || run.TokensUsed != 30 {
		t.Fatalf("unexpected heartbeat run: %+v err=%v", run, err)
	}
	runs, err := daemon.GetRecentRuns(context.Background(), 0)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("heartbeat ledger mismatch: %+v err=%v", runs, err)
	}
	daemon.Start(context.Background())
	daemon.Start(context.Background())
	daemon.Stop()
	daemon.Stop()

	nilDaemon := NewHeartbeatDaemon(manager, engine, nil, nil, "", 0)
	if runs, err := nilDaemon.GetRecentRuns(context.Background(), 10); err != nil || len(runs) != 0 {
		t.Fatalf("nil heartbeat ledger mismatch: %+v err=%v", runs, err)
	}
}

func TestHeartbeatAdvancesAndCompletesAutonomousTask(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	taskManager, err := NewTaskManager(db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := taskManager.ListTasks(context.Background(), "pending", "")
	if err != nil || len(tasks) == 0 {
		t.Fatalf("default task missing: %+v err=%v", tasks, err)
	}
	target := tasks[0]
	target.TargetChannel = "none"
	if err := taskManager.UpdateTask(context.Background(), target); err != nil {
		t.Fatal(err)
	}

	provider := llm.NewMockProvider("openai/gpt-4o", "")
	provider.CompleteFunc = func(context.Context, []llm.Message, llm.CompletionOptions) (*llm.Response, error) {
		content := "[PROGRESS: 60%] Inspected system state."
		if provider.CompleteCalls > 1 {
			content = "[TASK_COMPLETED] System health verified."
		}
		return &llm.Response{Model: "openai/gpt-4o", Content: content, Usage: llm.Usage{TotalTokens: 4}}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Minute)
	daemon.SetTaskManager(taskManager)
	daemon.SetSessionManager(nil)

	first, _ := daemon.TriggerManualPulse(context.Background())
	if first.Status != "action_taken" {
		t.Fatalf("task was not advanced: %+v", first)
	}
	updated, err := taskManager.GetTask(context.Background(), target.ID)
	if err != nil || updated.Status != "in_progress" || updated.Progress != 60 {
		t.Fatalf("progress was not persisted: %+v err=%v", updated, err)
	}
	second, _ := daemon.TriggerManualPulse(context.Background())
	updated, err = taskManager.GetTask(context.Background(), target.ID)
	if err != nil || second.Status != "action_taken" || updated.Status != "completed" || updated.Progress != 100 {
		t.Fatalf("completion was not persisted: run=%+v task=%+v err=%v", second, updated, err)
	}
	if cleanFullContent("[TASK_COMPLETED] done") != "done" ||
		cleanFullContent("[PROGRESS: 20%] next") != "next" ||
		cleanFullContent("[TASK_BLOCKED: wait] blocked") != "blocked" {
		t.Fatal("heartbeat marker cleanup failed")
	}
	if got := shortSummary(strings.Repeat("x", 20), 10); got != "xxxxxxx..." {
		t.Fatalf("summary truncation mismatch: %q", got)
	}
}

func TestReflectionPersistsRedactedPreferenceAndMemory(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "reflection.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, err := NewUserProfileManager(db, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	vectorStore, err := memory.NewVectorStore(filepath.Join(tempDir, "vectors"))
	if err != nil {
		t.Fatal(err)
	}
	hybrid := memory.NewHybridEngine(db, vectorStore, nil)
	router := llm.NewModelCascadeRouter()
	done := make(chan struct{}, 1)
	mock := llm.NewMockProvider("reflection", "")
	mock.CompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		if strings.Contains(messages[0].Content, "sk-super-secret-token") {
			t.Error("secret was not redacted before reflection")
		}
		done <- struct{}{}
		return &llm.Response{Content: `{"preference_key":"style","preference_value":"concise","episodic_memory":"Use concise reports."}`}, nil
	}
	router.RegisterProvider("reflection", mock)
	reflection := NewReflectionEngine(profile, hybrid, router, nil)
	reflection.RunReflectionCycle(context.Background())
	reflection.ReflectOnConversation(context.Background(), "agent-a", "api_key=sk-super-secret-token", "Completed safely.")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reflection LLM was not called")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if profile.GetProfile().Preferences["style"] == "concise" &&
			strings.Contains(profile.GetAgentMemoryMD("agent-a"), "Use concise reports") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if profile.GetProfile().Preferences["style"] != "concise" {
		t.Fatal("reflected preference was not persisted")
	}
	if !strings.Contains(profile.GetAgentMemoryMD("agent-a"), "Use concise reports") {
		t.Fatal("episodic reflection was not persisted")
	}
	reflection.ReflectOnConversation(context.Background(), "", "", "ignored")
	reflection.ReflectOnConversation(context.Background(), "", "hello", "HEARTBEAT_OK")
	ctx, cancel := context.WithCancel(context.Background())
	reflection.Start(ctx)
	cancel()
	reflection.Stop()
}

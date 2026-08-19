package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

func TestHeartbeatRoutineAndRunLedger(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Hour)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "ok" || run.ID == "" || run.TokensUsed != 0 || provider.CompleteCalls != 0 {
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

func TestHasActionableHeartbeatDirectives(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "empty", content: "", want: false},
		{name: "legacy generic supervisor", content: legacyDefaultHeartbeatDirective, want: false},
		{name: "template comments and headings", content: "<!-- scheduler note -->\n# Heartbeat\n\n- [ ]\n```markdown\n```", want: false},
		{name: "multiline comment", content: "<!--\nnot a directive\n-->\n## Still idle", want: false},
		{name: "explicit directive", content: "# Ops\nCheck the deployment health endpoint.", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasActionableHeartbeatDirectives(tt.content); got != tt.want {
				t.Fatalf("hasActionableHeartbeatDirectives(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestHeartbeatSkipsLegacyDefaultDirective(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "HEARTBEAT.md"), []byte(legacyDefaultHeartbeatDirective), 0644); err != nil {
		t.Fatal(err)
	}

	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "ok" || provider.CompleteCalls != 0 {
		t.Fatalf("legacy directive should not trigger a heartbeat routine: run=%+v err=%v calls=%d", run, err, provider.CompleteCalls)
	}
}

func TestHeartbeatSkipsLegacySystemTasks(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	taskManager, err := NewTaskManager(db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taskManager.CreateTask(context.Background(), AutonomousTask{
		Title:       "Legacy system health check",
		Description: "This task was previously seeded automatically.",
		CreatedBy:   "system",
	}); err != nil {
		t.Fatal(err)
	}

	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Hour)
	daemon.SetTaskManager(taskManager)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "ok" || provider.CompleteCalls != 0 {
		t.Fatalf("system-created task should not trigger a heartbeat mission: run=%+v err=%v calls=%d", run, err, provider.CompleteCalls)
	}
}

func TestHeartbeatExcludesCronFromRoutineTools(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "HEARTBEAT.md"), []byte("Check the deployment health endpoint."), 0644); err != nil {
		t.Fatal(err)
	}

	provider := llm.NewMockProvider("openai/gpt-4o", "")
	provider.CompleteFunc = func(_ context.Context, _ []llm.Message, opts llm.CompletionOptions) (*llm.Response, error) {
		for _, tool := range opts.Tools {
			if tool.Function.Name == "native_cron_schedule" {
				t.Fatal("routine heartbeat exposed cron scheduling to the model")
			}
		}
		return &llm.Response{Model: "openai/gpt-4o", Content: "HEARTBEAT_OK"}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	registry := tools.NewToolRegistry(nil)
	tools.RegisterNativeTools(registry, workspace)
	engine.SetToolRegistry(registry)

	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "ok" || provider.CompleteCalls != 1 {
		t.Fatalf("unexpected heartbeat routine run: %+v err=%v calls=%d", run, err, provider.CompleteCalls)
	}
}

func TestHeartbeatHonorsConfiguredSilentTarget(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "HEARTBEAT.md"), []byte("Check the deployment health endpoint."), 0644); err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", llm.NewMockProvider("openai/gpt-4o", "Deployment needs attention."))
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	daemon.SyncConfig(HeartbeatConfig{Enabled: true, TargetChannel: "none", TargetAccountID: "all"})

	notifications := eventBus.Subscribe(bus.EventAgentActionDone)
	defer eventBus.Unsubscribe(bus.EventAgentActionDone, notifications)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "action_taken" {
		t.Fatalf("unexpected heartbeat routine run: %+v err=%v", run, err)
	}
	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()
	for {
		select {
		case event := <-notifications:
			payload, _ := event.Payload.(map[string]any)
			if payload["type"] == "proactive_cron_notification" {
				t.Fatalf("silent heartbeat unexpectedly emitted a notification: %+v", event)
			}
		case <-timeout.C:
			return
		}
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
	target, err := taskManager.CreateTask(context.Background(), AutonomousTask{
		Title:         "Verify system health",
		Description:   "Inspect system state.",
		CreatedBy:     "user",
		TargetChannel: "none",
	})
	if err != nil {
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

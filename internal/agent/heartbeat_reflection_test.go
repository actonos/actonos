package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

func configureHeartbeatDirective(t *testing.T, db *memory.DB, daemon *HeartbeatDaemon, directive string) {
	t.Helper()
	taskManager, err := NewTaskManager(db.SQLDB(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskManager.SaveHeartbeatConfig(context.Background(), HeartbeatConfig{
		Enabled: true, IntervalMinutes: 60, Directives: directive,
		TargetChannel: "all", TargetAccountID: "all", AutoDelegate: true, ZeroNoise: true,
	}); err != nil {
		t.Fatal(err)
	}
	daemon.SetTaskManager(taskManager)
}

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
	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	configureHeartbeatDirective(t, db, daemon, legacyDefaultHeartbeatDirective)

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
	provider := llm.NewMockProvider("openai/gpt-4o", "")
	provider.CompleteFunc = func(_ context.Context, _ []llm.Message, opts llm.CompletionOptions) (*llm.Response, error) {
		for _, tool := range opts.Tools {
			if tool.Function.Name == "native_cron_schedule" {
				t.Fatal("routine heartbeat exposed cron scheduling to the model")
			}
			if tool.Function.Name == "native_channel_notify" {
				t.Fatal("routine heartbeat exposed channel notify to the model")
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
	configureHeartbeatDirective(t, db, daemon, "Check the deployment health endpoint.")
	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || run.Status != "ok" || provider.CompleteCalls != 1 {
		t.Fatalf("unexpected heartbeat routine run: %+v err=%v calls=%d", run, err, provider.CompleteCalls)
	}
}

func TestClassifyHeartbeatResponse(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		ackMaxChars int
		wantAck     bool
		wantAlert   string
	}{
		{name: "empty reply is ack", content: "", wantAck: true},
		{name: "bare token is ack", content: "HEARTBEAT_OK", wantAck: true},
		{name: "token with short trailing footnote is ack", content: "HEARTBEAT_OK (checked inbox, nothing urgent)", ackMaxChars: 300, wantAck: true},
		{name: "token at end with short preamble is ack", content: "Everything nominal. HEARTBEAT_OK", ackMaxChars: 300, wantAck: true},
		{name: "token with oversized remainder is alert", content: "HEARTBEAT_OK " + strings.Repeat("x", 400), ackMaxChars: 300, wantAck: false, wantAlert: "HEARTBEAT_OK " + strings.Repeat("x", 400)},
		{name: "token in the middle is not special-cased", content: "Note: HEARTBEAT_OK is our usual signal, but today the deploy failed.", wantAck: false, wantAlert: "Note: HEARTBEAT_OK is our usual signal, but today the deploy failed."},
		{name: "lookalike token does not match", content: "HEARTBEAT_OKAY, everything is fine and dandy today.", wantAck: false, wantAlert: "HEARTBEAT_OKAY, everything is fine and dandy today."},
		{name: "unrelated hallucinated content is an alert", content: "Đã hoàn tất: điều hướng đến trang chủ github.com và tìm kiếm từ khóa \"ActonOS\".", wantAck: false, wantAlert: "Đã hoàn tất: điều hướng đến trang chủ github.com và tìm kiếm từ khóa \"ActonOS\"."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAck, gotAlert := classifyHeartbeatResponse(tt.content, tt.ackMaxChars)
			if gotAck != tt.wantAck {
				t.Fatalf("classifyHeartbeatResponse(%q) isAck = %v, want %v", tt.content, gotAck, tt.wantAck)
			}
			if !gotAck && gotAlert != tt.wantAlert {
				t.Fatalf("classifyHeartbeatResponse(%q) alert = %q, want %q", tt.content, gotAlert, tt.wantAlert)
			}
		})
	}
}

// TestLooksLikeIdleChatter guards against a model free-associating a
// conversational greeting/self-introduction instead of executing the
// standing directive or emitting HEARTBEAT_OK — such replies must be
// recognized as noise so they are never delivered as a user-facing alert.
func TestLooksLikeIdleChatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "vietnamese greeting is idle chatter", content: "Chào Bieber! Tôi đang sẵn sàng tại đây. Bạn muốn tôi hỗ trợ gì hôm nay?", want: true},
		{name: "vietnamese self-intro is idle chatter", content: "Chào anh Bieber, em đây — Nova đã sẵn sàng. Anh cần em hỗ trợ gì hôm nay?", want: true},
		{name: "english capability menu is idle chatter", content: "Hi there! How can I help you today? I can research, write code, or analyze data.", want: true},
		{name: "real weather report is not idle chatter", content: "🌤 Thời tiết TP. Hồ Chí Minh — 19:45 ngày 19/08/2026: Nhiệt độ 27.5°C, cảm giác thực tế 31.7°C.", want: false},
		{name: "task status report is not idle chatter", content: "Đã kiểm tra TASKS.md — backlog hiện không còn tác vụ nào đang chờ.", want: false},
		{name: "empty content is not idle chatter", content: "", want: false},
		{name: "long factual report opening with a data point is not flagged", content: "Nhiệt độ hiện tại tại TP. Hồ Chí Minh là 28°C, trời quang mây, độ ẩm 70%. Đã gửi bản tin này tới tất cả các kênh chat đã cấu hình theo đúng chỉ thị trong HEARTBEAT.md.", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeIdleChatter(tt.content); got != tt.want {
				t.Fatalf("looksLikeIdleChatter(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// TestHeartbeatCooldownSuppressesRapidNonManualPulses guards against a task
// mutation and an approval decision firing TriggerWakeup within moments of
// each other, which previously ran a full model turn per event.
func TestHeartbeatCooldownSuppressesRapidNonManualPulses(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	configureHeartbeatDirective(t, db, daemon, "Check the deployment health endpoint.")

	first := daemon.checkCycle(context.Background(), false)
	if first == nil || first.Status != "ok" || provider.CompleteCalls != 1 {
		t.Fatalf("expected first non-manual cycle to run: %+v calls=%d", first, provider.CompleteCalls)
	}
	second := daemon.checkCycle(context.Background(), false)
	if second != nil {
		t.Fatalf("expected rapid follow-up non-manual cycle to be suppressed by cooldown, got %+v", second)
	}
	if provider.CompleteCalls != 1 {
		t.Fatalf("cooldown-suppressed cycle should not invoke the model, calls=%d", provider.CompleteCalls)
	}

	// A manual pulse always bypasses the cooldown.
	manual, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || manual == nil || manual.Status != "ok" || provider.CompleteCalls != 2 {
		t.Fatalf("expected manual pulse to bypass cooldown: %+v err=%v calls=%d", manual, err, provider.CompleteCalls)
	}
}

// TestHeartbeatActiveHoursSkipsOutsideWindow guards OpenClaw's documented
// activeHours behavior: routine cycles outside the configured window must be
// skipped entirely (no model call), while manual pulses always run.
func TestHeartbeatActiveHoursSkipsOutsideWindow(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	configureHeartbeatDirective(t, db, daemon, "Check the deployment health endpoint.")

	// A zero-width window ("start == end") means the window is always outside,
	// per OpenClaw's documented semantics.
	daemon.SyncConfig(HeartbeatConfig{Enabled: true, ActiveHoursStart: "08:00", ActiveHoursEnd: "08:00", ActiveHoursTimezone: "UTC"})

	run := daemon.checkCycle(context.Background(), false)
	if run != nil || provider.CompleteCalls != 0 {
		t.Fatalf("expected non-manual cycle outside active hours to be skipped: %+v calls=%d", run, provider.CompleteCalls)
	}

	manual, err := daemon.TriggerManualPulse(context.Background())
	if err != nil || manual == nil || manual.Status != "ok" || provider.CompleteCalls != 1 {
		t.Fatalf("expected manual pulse to bypass active-hours window: %+v err=%v calls=%d", manual, err, provider.CompleteCalls)
	}
}

func TestHeartbeatHonorsConfiguredSilentTarget(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", llm.NewMockProvider("openai/gpt-4o", "Deployment needs attention."))
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	configureHeartbeatDirective(t, db, daemon, "Check the deployment health endpoint.")
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

// TestHeartbeatSuppressesOffTopicGreetingResponse reproduces a real observed
// production bug: a weaker model ignores the directive-or-HEARTBEAT_OK
// contract and free-associates a conversational greeting instead of
// executing the standing directive. The run must be classified as nominal
// ("ok") and must NOT be delivered as a user-facing alert notification.
func TestHeartbeatSuppressesOffTopicGreetingResponse(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	directive := "Autonomous standing supervisor. Routinely review pending tasks and monitor system stability.\n\n" +
		"- Check current Ho Chi Minh, Vietnam weather and send to all chat channels"
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", llm.NewMockProvider("openai/gpt-4o",
		"Chào Bieber! Tôi đang sẵn sàng tại đây. Bạn muốn tôi hỗ trợ gì hôm nay?"))
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), workspace, time.Hour)
	configureHeartbeatDirective(t, db, daemon, directive)
	daemon.SyncConfig(HeartbeatConfig{Enabled: true, TargetChannel: "all", TargetAccountID: "all"})

	notifications := eventBus.Subscribe(bus.EventAgentActionDone)
	defer eventBus.Unsubscribe(bus.EventAgentActionDone, notifications)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "ok" {
		t.Fatalf("expected off-topic greeting to be suppressed as nominal, got status=%q summary=%q", run.Status, run.Summary)
	}
	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()
	for {
		select {
		case event := <-notifications:
			payload, _ := event.Payload.(map[string]any)
			if payload["type"] == "proactive_cron_notification" {
				t.Fatalf("off-topic greeting response should never be delivered as an alert: %+v", event)
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

// TestHeartbeatEscalatesStalledTaskAfterRepeatedNoProgress guards against a
// mission task looping forever at unchanged progress, silently consuming a
// full model turn every heartbeat cycle with no operator visibility.
func TestHeartbeatEscalatesStalledTaskAfterRepeatedNoProgress(t *testing.T) {
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
		Title:         "Investigate flaky test",
		Description:   "Find root cause of intermittent CI failure.",
		CreatedBy:     "user",
		TargetChannel: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The model reports the exact same 40% progress every single cycle,
	// never actually advancing — a realistic "stuck" scenario.
	provider := llm.NewMockProvider("openai/gpt-4o", "[PROGRESS: 40%] Still investigating, no new findings yet.")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Minute)
	daemon.SetTaskManager(taskManager)
	daemon.SetSessionManager(nil)

	var last *HeartbeatRun
	for i := 0; i < maxStalledCyclesBeforeEscalation+1; i++ {
		last, err = daemon.TriggerManualPulse(context.Background())
		if err != nil {
			t.Fatalf("cycle %d failed: %v", i, err)
		}
	}
	if !strings.Contains(last.Summary, "STALL WARNING") {
		t.Fatalf("expected stall escalation after %d unchanged-progress cycles, got summary=%q", maxStalledCyclesBeforeEscalation+1, last.Summary)
	}
	updated, err := taskManager.GetTask(context.Background(), target.ID)
	if err != nil || !strings.Contains(updated.ExecutionLog, "STALL WARNING") {
		t.Fatalf("expected stall warning persisted in execution log: %+v err=%v", updated, err)
	}
}

// TestHeartbeatApprovalPendingOnlyBlocksItsOwnAgent guards against an
// unrelated pending approval (for a different agent) blocking the entire
// backlog from launching any new task.
func TestHeartbeatApprovalPendingOnlyBlocksItsOwnAgent(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	taskManager, err := NewTaskManager(db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// This task belongs to the default primary agent and has no pending
	// approval of its own — it must still be launched even though some
	// unrelated agent elsewhere has a pending approval.
	target, err := taskManager.CreateTask(context.Background(), AutonomousTask{
		Title:           "Unrelated task",
		Description:     "Should still launch.",
		CreatedBy:       "user",
		TargetChannel:   "none",
		AssignedAgentID: "agent_system_core",
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := llm.NewMockProvider("openai/gpt-4o", "[PROGRESS: 30%] Working on it.")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Minute)
	daemon.SetTaskManager(taskManager)
	daemon.SetSessionManager(nil)
	daemon.SetApprovalManager(fakeApprovalLister{
		pending: []tools.ApprovalRequest{{AgentID: "some_other_unrelated_agent"}},
	})

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "action_taken" {
		t.Fatalf("expected unrelated task to launch despite an unrelated agent's pending approval, got: %+v", run)
	}
	updated, err := taskManager.GetTask(context.Background(), target.ID)
	if err != nil || updated.Status != "in_progress" {
		t.Fatalf("expected task to be launched: %+v err=%v", updated, err)
	}
}

// fakeApprovalLister is a minimal ApprovalListProvider stub for tests.
type fakeApprovalLister struct {
	pending []tools.ApprovalRequest
}

func (f fakeApprovalLister) List(_ context.Context, _ string, _ int) ([]tools.ApprovalRequest, error) {
	return f.pending, nil
}

// TestHeartbeatWarnsWhenDirectiveRequestsNotifyButTargetIsNone reproduces the
// silent-loss scenario: a task's text asks to send/notify somewhere, but its
// structured TargetChannel is "none" — the daemon must not fail the task, but
// it must log a diagnosable warning instead of quietly dropping the result.
func TestHeartbeatWarnsWhenDirectiveRequestsNotifyButTargetIsNone(t *testing.T) {
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
		Title:         "Weather check",
		Description:   "Check HCMC weather and gửi thông báo cho tất cả các kênh chat.",
		CreatedBy:     "user",
		TargetChannel: "none",
	}); err != nil {
		t.Fatal(err)
	}

	provider := llm.NewMockProvider("openai/gpt-4o", "[TASK_COMPLETED] 28C, sunny.")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Minute)
	daemon.SetTaskManager(taskManager)
	daemon.SetSessionManager(nil)

	if !mentionsNotificationIntent("Check HCMC weather and gửi thông báo cho tất cả các kênh chat.") {
		t.Fatal("expected notification intent to be detected in the task description")
	}
	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "action_taken" {
		t.Fatalf("task should still execute normally despite the misconfiguration: %+v", run)
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

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

func TestHungTurnIsCutByTimeout(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	provider := llm.NewMockProvider("timeout-model", "")
	provider.CompleteFunc = func(ctx context.Context, _ []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	router.RegisterProvider("timeout-model", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	created, err := manager.Create(context.Background(), AgentManifest{
		Name: "Timeout", Status: StatusActive, ModelConfig: llm.ModelConfig{PrimaryModel: "timeout-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err = engine.ExecuteStep(ctx, created.AgentID, "hello")
	if err == nil {
		t.Fatal("expected timeout to cut the hung turn")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestHeartbeatPanicReleasesMutex(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	provider := llm.NewMockProvider("panic-model", "")
	provider.CompleteFunc = func(context.Context, []llm.Message, llm.CompletionOptions) (*llm.Response, error) {
		panic("boom")
	}
	router.RegisterProvider("panic-model", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Hour)
	configureHeartbeatDirective(t, db, daemon, "Check the deployment health endpoint.")

	first := daemon.checkCycle(context.Background(), true)
	if first == nil || first.Status != "error" || !strings.Contains(first.Summary, "panic") {
		t.Fatalf("expected recovered panic run, got %+v", first)
	}

	done := make(chan struct{})
	go func() {
		_ = daemon.checkCycle(context.Background(), true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat mutex stayed held after panic")
	}
}

func TestEmptyOutputDoesNotCompleteMission(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	taskManager, err := NewTaskManager(db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := taskManager.CreateTask(context.Background(), AutonomousTask{
		Title: "Write a report", Description: "Produce a written report file", CreatedBy: "user", TargetChannel: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	provider := llm.NewMockProvider("empty-model", cannedSuccessPhrase)
	router.RegisterProvider("empty-model", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Minute)
	daemon.SetTaskManager(taskManager)
	daemon.SetSessionManager(nil)

	run, err := daemon.TriggerManualPulse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := taskManager.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status == "completed" {
		t.Fatalf("canned success completed the mission: run=%+v task=%+v", run, updated)
	}
}

func TestResumeApprovedWithoutCompletionMarkerLeavesInProgress(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	approvalMgr := tools.NewApprovalManager(db.SQLDB())
	registry := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(registry, workspace)
	registry.SetApprovalManager(approvalMgr)
	registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{AuthorizedTools: []string{"native_file_write"}, ApprovalThreshold: "High", AllowedPaths: []string{"*"}}, nil
	})
	provider := llm.NewMockProvider("resume-model", "")
	provider.CompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		if provider.CompleteCalls == 1 {
			return &llm.Response{
				Model: "resume-model",
				ToolCalls: []llm.ToolCall{{
					ID: "call-write", Type: "function",
					Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"result.txt","content":"x"}`)},
				}},
			}, nil
		}
		return &llm.Response{Model: "resume-model", Content: "still working, no completion marker"}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("resume-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	runStore := NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)
	taskMgr, err := NewTaskManager(db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine.SetTaskManager(taskMgr)
	task, err := taskMgr.CreateTask(context.Background(), AutonomousTask{
		Title: "Write file", Description: "write result.txt", CreatedBy: "user",
	})
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Resume", Status: StatusActive,
		ModelConfig: llm.ModelConfig{PrimaryModel: "resume-model"},
		AuthorizedTools: []string{"native_file_write"},
		DelegationScope: DelegationScope{AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), "task_id", task.ID)
	_, err = engine.ExecuteStepWithHistory(ctx, manifest.AgentID, "Write result.txt", nil)
	var approvalRequired *tools.ApprovalRequiredError
	if !errors.As(err, &approvalRequired) {
		t.Fatalf("expected approval required, got %v", err)
	}
	if _, err := approvalMgr.Decide(context.Background(), approvalRequired.Approval.ID, "approved", "test", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ResumeApproved(context.Background(), approvalRequired.Approval); err != nil {
		t.Fatal(err)
	}
	updated, err := taskMgr.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status == "completed" {
		t.Fatalf("resume without completion marker must not complete the task: %+v", updated)
	}
}

func TestHeartbeatSourceIsHeartbeatForApprovalPolicy(t *testing.T) {
	got := sourceFromContextOrMessage(WithExecutionSource(context.Background(), "heartbeat"), "plain user text", "chat")
	if got != "heartbeat" {
		t.Fatalf("expected heartbeat source, got %q", got)
	}
	got = sourceFromContextOrMessage(context.Background(), "<autonomous_mission_cycle>", "chat")
	if got != "heartbeat" {
		t.Fatalf("expected XML mission tag to map to heartbeat, got %q", got)
	}
}

func TestVerifyToolCommandRunsOnNonStreamPath(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	registry := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(registry, workspace)
	registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{AuthorizedTools: []string{"native_exec"}, ApprovalThreshold: "Low", AllowedPaths: []string{"*"}}, nil
	})
	provider := llm.NewMockProvider("exec-model", "")
	var sawObservation string
	var mu sync.Mutex
	provider.CompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		if provider.CompleteCalls == 1 {
			return &llm.Response{
				Model: "exec-model",
				ToolCalls: []llm.ToolCall{{
					ID: "c1", Type: "function",
					Function: llm.FunctionCall{Name: "native_exec", Arguments: json.RawMessage(`{"command":"rm -rf /"}`)},
				}},
			}, nil
		}
		mu.Lock()
		defer mu.Unlock()
		if len(messages) > 0 {
			sawObservation = messages[len(messages)-1].Content
		}
		return &llm.Response{Model: "exec-model", Content: "blocked as expected"}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("exec-model", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	created, err := manager.Create(context.Background(), AgentManifest{
		Name: "Exec", Status: StatusActive,
		ModelConfig: llm.ModelConfig{PrimaryModel: "exec-model"},
		AuthorizedTools: []string{"native_exec"},
		DelegationScope: DelegationScope{RequireHumanApproval: ApprovalLow, AllowedWorkspacePaths: []string{"*"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := engine.ExecuteStep(context.Background(), created.AgentID, "wipe disk")
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Content == "" {
		t.Fatal("expected a synthesized response after blocked exec")
	}
	mu.Lock()
	obs := sawObservation
	mu.Unlock()
	if !strings.Contains(strings.ToLower(obs), "forbidden") && !strings.Contains(strings.ToLower(obs), "error executing") {
		t.Fatalf("expected VerifyToolCommand observation on non-stream path, got %q", obs)
	}
}

func TestStoredPlanAdvancesOneReadyStep(t *testing.T) {
	p := NewPlanner(nil)
	plan := &TaskPlan{Goal: "g", Steps: []PlanStep{
		{ID: "a", Description: "first", Status: "pending"},
		{ID: "b", Description: "second", Status: "pending", Dependencies: []string{"a"}},
	}}
	step, err := p.NextReadyStep(plan)
	if err != nil || step == nil || step.ID != "a" {
		t.Fatalf("expected step a, got %+v err=%v", step, err)
	}
	plan.MarkStep("a", "completed", "ok")
	step, err = p.NextReadyStep(plan)
	if err != nil || step == nil || step.ID != "b" {
		t.Fatalf("expected step b after a completed, got %+v err=%v", step, err)
	}
}

func TestCustomAgentHeartbeatIntervalHonored(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Create(context.Background(), AgentManifest{
		Name: "Custom Pulse", Status: StatusActive,
		ModelConfig: llm.ModelConfig{PrimaryModel: "openai/gpt-4o"},
		HeartbeatConfig: &AgentHeartbeatConfig{Enabled: true, IntervalMinutes: 60, Directives: "Ping the health endpoint now."},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := llm.NewMockProvider("openai/gpt-4o", "HEARTBEAT_OK")
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("openai/gpt-4o", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	daemon := NewHeartbeatDaemon(manager, engine, eventBus, db.SQLDB(), t.TempDir(), time.Hour)
	now := time.Now().UTC()
	daemon.checkCustomAgentPulses(context.Background(), now, false)
	firstCalls := provider.CompleteCalls
	if firstCalls == 0 {
		t.Fatal("expected first custom pulse to invoke the model")
	}
	daemon.checkCustomAgentPulses(context.Background(), now.Add(10*time.Minute), false)
	if provider.CompleteCalls != firstCalls {
		t.Fatalf("interval was not honored: calls %d -> %d", firstCalls, provider.CompleteCalls)
	}
	_ = created
}

func TestRunCancelStopsInFlightTurn(t *testing.T) {
	db, eventBus := setupTestDB(t)
	manager, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewModelCascadeRouter()
	provider := llm.NewMockProvider("cancel-model", "")
	started := make(chan string, 1)
	provider.CompleteFunc = func(ctx context.Context, _ []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		select {
		case started <- "go":
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	router.RegisterProvider("cancel-model", provider)
	engine := NewEngine(manager, eventBus, router, nil)
	runStore := NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)
	created, err := manager.Create(context.Background(), AgentManifest{
		Name: "Cancel", Status: StatusActive, ModelConfig: llm.ModelConfig{PrimaryModel: "cancel-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, e := engine.ExecuteStep(context.Background(), created.AgentID, "long work")
		errCh <- e
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not start")
	}
	runs, err := runStore.ListFiltered(context.Background(), RunListFilter{Status: RunRunning, Limit: 10})
	if err != nil || len(runs) == 0 {
		t.Fatalf("expected a running row: %v %+v", err, runs)
	}
	if err := engine.CancelRun(context.Background(), runs[0].ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected cancelled error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled turn did not return")
	}
}

func TestReclaimOrphansBlocksRunningRows(t *testing.T) {
	db, _ := setupTestDB(t)
	store := NewRunStore(db.SQLDB())
	run, err := store.Start(context.Background(), "trace", "agent_x", "goal", "heartbeat")
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.ReclaimOrphans(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("expected 1 reclaimed run, got n=%d err=%v", n, err)
	}
	got, err := store.Get(context.Background(), run.ID)
	if err != nil || got.Status != RunBlocked {
		t.Fatalf("expected blocked orphan, got %+v err=%v", got, err)
	}
}

func TestIsCannedOrEmptyCompletion(t *testing.T) {
	if !IsCannedOrEmptyCompletion("") || !IsCannedOrEmptyCompletion(cannedSuccessPhrase) {
		t.Fatal("empty and canned must be rejected")
	}
	if IsCannedOrEmptyCompletion("real progress [PROGRESS: 10%]") {
		t.Fatal("real content must not be treated as canned")
	}
}

package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

func TestPlanner_DecomposeGoal(t *testing.T) {
	router := llm.NewModelCascadeRouter()
	mock := llm.NewMockProvider("anthropic/claude-sonnet-4.5", `[{"id":"task_1","description":"Analyze code","agent_role":"code","dependencies":[]}]`)
	router.RegisterProvider("anthropic/claude-sonnet-4.5", mock)

	planner := NewPlanner(router)
	plan, err := planner.DecomposeGoal(context.Background(), "Build a microservice", nil)
	if err != nil {
		t.Fatalf("DecomposeGoal failed: %v", err)
	}

	if len(plan.Steps) != 2 {
		t.Fatalf("non-atomic 1-step JSON must expand to the default DAG, got %d: %+v", len(plan.Steps), plan.Steps)
	}
}

func TestPlannerKeepsAtomicSingleStep(t *testing.T) {
	router := llm.NewModelCascadeRouter()
	mock := llm.NewMockProvider("anthropic/claude-sonnet-4.5", `[{"id":"task_1","description":"Ping health","agent_role":"general","kind":"research","atomic":true,"dependencies":[]}]`)
	router.RegisterProvider("anthropic/claude-sonnet-4.5", mock)

	planner := NewPlanner(router)
	plan, err := planner.DecomposeGoal(context.Background(), "Ping the health endpoint", nil)
	if err != nil {
		t.Fatalf("DecomposeGoal failed: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].ID != "task_1" || !plan.Steps[0].Atomic || plan.Steps[0].Kind != StepKindResearch {
		t.Fatalf("atomic 1-step plan was rewritten: %+v", plan.Steps)
	}
}

func TestPlannerExpandsNonAtomicSingleStepRegardlessOfGoalLanguage(t *testing.T) {
	router := llm.NewModelCascadeRouter()
	mock := llm.NewMockProvider("anthropic/claude-sonnet-4.5", `[{"id":"task_1","description":"Create the outline","agent_role":"general","dependencies":[]}]`)
	router.RegisterProvider("anthropic/claude-sonnet-4.5", mock)

	planner := NewPlanner(router)
	plan, err := planner.DecomposeGoal(context.Background(), "Erstelle eine Lektionsserie über ABC, jeder Teil eine PDF-Datei", nil)
	if err != nil {
		t.Fatalf("DecomposeGoal failed: %v", err)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("non-atomic 1-step plan must expand even when the goal is not English/Vietnamese: %+v", plan.Steps)
	}
}

func TestReopenApprovalFailedSteps(t *testing.T) {
	plan := &TaskPlan{Steps: []PlanStep{
		{ID: "task_1", Status: StepStatusFailed, Result: approvalPausedMarker + " human approval required: approval_id=a1"},
		{ID: "task_2", Status: StepStatusPending, Dependencies: []string{"task_1"}},
		{ID: "task_3", Status: StepStatusFailed, Result: "tool execution blocked: disk full"},
	}}
	if !plan.reopenApprovalFailedSteps() {
		t.Fatal("expected approval-failed step to reopen")
	}
	if plan.StepStatus("task_1") != StepStatusPending {
		t.Fatalf("task_1 should be pending, got %q", plan.StepStatus("task_1"))
	}
	if plan.StepStatus("task_3") != StepStatusFailed {
		t.Fatal("real failures must stay failed")
	}
	if PlanStepIDFromPrompt(BuildPlanStepPrompt("task_2", "goal", "do it", "general", "")) != "task_2" {
		t.Fatal("step id not extracted from plan-step prompt")
	}
}

func TestPlanner_ExecutePlan(t *testing.T) {
	planner := NewPlanner(nil)
	plan := &TaskPlan{Steps: []PlanStep{
		{ID: "one", Status: "pending"},
		{ID: "two", Dependencies: []string{"one"}, Status: "pending"},
	}}
	var order []string
	err := planner.ExecutePlan(context.Background(), plan, func(_ context.Context, step PlanStep) (string, error) {
		order = append(order, step.ID)
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("executing plan: %v", err)
	}
	if strings.Join(order, ",") != "one,two" {
		t.Fatalf("unexpected execution order: %v", order)
	}

	cycle := &TaskPlan{Steps: []PlanStep{
		{ID: "a", Dependencies: []string{"b"}},
		{ID: "b", Dependencies: []string{"a"}},
	}}
	if err := planner.ExecutePlan(context.Background(), cycle, func(context.Context, PlanStep) (string, error) {
		return "", nil
	}); err == nil {
		t.Fatal("expected dependency cycle error")
	}
}

func TestVerifier_StaticAnalysis(t *testing.T) {
	verifier := NewVerifier()

	// 1. Verify Forbidden Commands
	if err := verifier.VerifyCommand("rm -rf / --no-preserve-root"); err == nil {
		t.Fatal("expected error on forbidden command, got nil")
	}

	if err := verifier.VerifyCommand("ls -la /workspace"); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	// 2. Verify Path Escape
	tempDir := t.TempDir()
	workspace := filepath.Join(tempDir, "workspace")

	if err := verifier.VerifyPath("../etc/passwd", workspace); err == nil {
		t.Fatal("expected path escape error, got nil")
	}

	if err := verifier.VerifyPath("src/main.go", workspace); err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
}

func TestUserProfile_And_ContextManager(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	mgr, err := NewUserProfileManager(db, tempDir)
	if err != nil {
		t.Fatalf("creating user profile manager: %v", err)
	}

	ctx := context.Background()
	profile := UserProfile{
		UserName:           "Bieber",
		Language:           "vi",
		CommunicationStyle: "concise",
		Preferences:        map[string]string{"theme": "dark"},
	}
	if err := mgr.UpdateProfile(ctx, profile); err != nil {
		t.Fatalf("updating profile: %v", err)
	}

	p := mgr.GetProfile()
	if p.UserName != "Bieber" || p.Language != "vi" {
		t.Fatalf("unexpected profile values: %+v", p)
	}

	// Context Manager test
	ctxMgr := NewContextManager(4096)
	augmented := ctxMgr.BuildAugmentedSystemPrompt("Base Instructions", p, nil)
	if !strings.Contains(augmented, "User: Bieber") || !strings.Contains(augmented, "Preferred Language: vi") {
		t.Fatalf("augmented prompt missing profile info: %s", augmented)
	}

	messages := []llm.Message{{Role: llm.RoleSystem, Content: "system"}}
	for i := 0; i < 20; i++ {
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: strings.Repeat("context ", 80)})
	}
	pruned := ctxMgr.PruneMessages(messages, 300)
	if len(pruned) >= len(messages) {
		t.Fatal("expected token-budget compaction")
	}
	if pruned[0].Role != llm.RoleSystem || pruned[len(pruned)-1].Content != messages[len(messages)-1].Content {
		t.Fatal("context compaction did not preserve system and newest messages")
	}
}

func TestUserProfileProceduralSoulAndMemoryLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mgr, err := NewUserProfileManager(db, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, pattern := range []ProceduralPattern{
		{ID: "general", Domain: "general", PatternName: "Verify", Workflow: "test then build", SuccessRate: 0.9},
		{ID: "coding", Domain: "coding", PatternName: "Patch", Workflow: "edit then test", SuccessRate: 1},
	} {
		if err := mgr.StoreProceduralPattern(ctx, pattern); err != nil {
			t.Fatal(err)
		}
	}
	patterns, err := mgr.GetRelevantPatterns(ctx, "coding")
	if err != nil || len(patterns) != 2 || patterns[0].PatternName != "Patch" {
		t.Fatalf("unexpected procedural patterns: %+v err=%v", patterns, err)
	}

	if got := mgr.GetAgentSoul("new-agent"); !strings.Contains(got, "ActonOS") && !strings.Contains(got, "Agent Soul") {
		t.Fatalf("expected default soul, got %q", got)
	}
	if err := mgr.SaveAgentSoul(ctx, "agent-a", "Agent A soul"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetAgentSoul("agent-a"); got != "Agent A soul" {
		t.Fatalf("isolated soul mismatch: %q", got)
	}
	if err := mgr.SaveSoul(ctx, "System soul"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetSoul(); got != "System soul" {
		t.Fatalf("system soul mismatch: %q", got)
	}

	if err := mgr.AppendAgentMemoryMD(ctx, "agent-a", "Agent-specific lesson"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetAgentMemoryMD("agent-a"); !strings.Contains(got, "Agent-specific lesson") {
		t.Fatalf("isolated memory mismatch: %q", got)
	}
	if err := mgr.AppendMemoryMD(ctx, "System lesson"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetMemoryMD(); !strings.Contains(got, "System lesson") {
		t.Fatalf("system memory mismatch: %q", got)
	}

	if err := mgr.ClearAgentMemoryMD(ctx, "agent-a"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetAgentMemoryMD("agent-a"); got != "" {
		t.Fatalf("expected empty memory after clear, got %q", got)
	}
	if err := mgr.ClearMemoryMD(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetMemoryMD(); got != "" {
		t.Fatalf("expected empty system memory after clear, got %q", got)
	}

	reloaded, err := NewUserProfileManager(db, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.GetProfile().UserName == "" {
		t.Fatal("disk profile did not reload")
	}
}

func TestVerifierJSONSemanticAndCompletion(t *testing.T) {
	v := NewVerifier()
	if err := v.VerifyJSONSyntax(`{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifyJSONSyntax(`{"broken"`); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if v.VerifySemanticConsistency(context.Background(), "goal", "") {
		t.Fatal("empty output accepted")
	}
	if v.VerifySemanticConsistency(context.Background(), "goal", "I cannot fulfill this request") {
		t.Fatal("refusal accepted as successful output")
	}
	if v.VerifyTaskCompletion("fix the file", "completed", nil) {
		t.Fatal("action goal accepted without tool evidence")
	}
	if !v.VerifyTaskCompletion("fix the file", "completed", []llm.ToolCall{{ID: "one"}}) {
		t.Fatal("verified action completion rejected")
	}
	if v.VerifyTaskCompletion("explain", "tests failed: broken", nil) {
		t.Fatal("failed observation accepted")
	}
	if err := v.VerifyToolCommand(json.RawMessage(`{"command":"echo safe"}`)); err != nil {
		t.Fatal(err)
	}
	if !HasDeliverableWrite([]llm.ToolCall{{
		Function: llm.FunctionCall{Name: "native_workspace_write"},
	}}) {
		t.Fatal("workspace write should count as a deliverable")
	}
	if HasDeliverableWrite(nil) {
		t.Fatal("no tool calls must not count as a deliverable")
	}
	if HasDeliverableWrite([]llm.ToolCall{{
		Function: llm.FunctionCall{Name: "native_web_search"},
	}}) {
		t.Fatal("non-write tools must not count as a deliverable")
	}
	if err := v.VerifyToolCommand(json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty command accepted")
	}
}

func TestContextManagerPersistsCompactionSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "context.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := NewContextManager(128)
	manager.SetDB(db.SQLDB())
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "system"}}
	for index := 0; index < 12; index++ {
		messages = append(messages, llm.Message{
			Role: llm.RoleUser, Content: strings.Repeat("important context ", 40),
		})
	}
	pruned := manager.PruneAndSnapshot(context.Background(), "run-context", messages, 128)
	if len(pruned) >= len(messages) {
		t.Fatal("expected context compaction")
	}
	var summary string
	var sourceCount, retainedCount int
	err = db.SQLDB().QueryRow(`
		SELECT summary, source_message_count, retained_message_count
		FROM context_snapshots WHERE run_id = ?
	`, "run-context").Scan(&summary, &sourceCount, &retainedCount)
	if err != nil {
		t.Fatal(err)
	}
	if summary == "" || sourceCount != len(messages) || retainedCount != len(pruned) {
		t.Fatalf("invalid snapshot provenance: source=%d retained=%d summary=%q", sourceCount, retainedCount, summary)
	}
}

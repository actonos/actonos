package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/memory"
)

type recordingFileMutationSink struct {
	path    string
	agentID string
	deleted bool
	calls   int
}

func (s *recordingFileMutationSink) NotifyFileMutation(_ context.Context, path, agentID string, deleted bool) error {
	s.path = path
	s.agentID = agentID
	s.deleted = deleted
	s.calls++
	return nil
}

func TestToolRegistry_NotifiesSuccessfulFileMutations(t *testing.T) {
	workspace := t.TempDir()
	registry := NewToolRegistry(nil)
	RegisterNativeTools(registry, workspace)
	sink := &recordingFileMutationSink{}
	registry.SetFileMutationSink(sink)

	if _, err := registry.Execute(context.Background(), "agent-files", "native_file_write",
		json.RawMessage(`{"path":"notes.txt","content":"semantic content"}`)); err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(workspace, "agents", "agent-files", "workspace", "notes.txt")
	wantInfo, wantErr := os.Stat(wantPath)
	gotInfo, gotErr := os.Stat(sink.path)
	if sink.calls != 1 || wantErr != nil || gotErr != nil || !os.SameFile(wantInfo, gotInfo) || sink.agentID != "agent-files" || sink.deleted {
		t.Fatalf("unexpected write notification: %+v", sink)
	}
	notifiedPath := sink.path
	if _, err := registry.Execute(context.Background(), "agent-files", "native_file_delete",
		json.RawMessage(`{"path":"notes.txt"}`)); err != nil {
		t.Fatal(err)
	}
	if sink.calls != 2 || sink.path != notifiedPath || sink.agentID != "agent-files" || !sink.deleted {
		t.Fatalf("unexpected delete notification: %+v", sink)
	}
}

func TestToolRegistry_RegisterAndExecute(t *testing.T) {
	t.Setenv("ACTONOS_ALLOW_INSECURE_EXEC", "1")
	eventBus := bus.NewEventBus()
	defer eventBus.Close()

	registry := NewToolRegistry(eventBus)

	// Register Native Tools
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	_ = os.MkdirAll(workspaceDir, 0755)

	RegisterNativeTools(registry, workspaceDir)

	tools := registry.List()
	if len(tools) < 4 {
		t.Fatalf("expected at least 4 native tools, got %d", len(tools))
	}

	// Test native_file_write
	ctx := context.Background()
	writeInput := json.RawMessage(`{"path": "test.txt", "content": "Hello ActonOS Tools"}`)
	res, err := registry.Execute(ctx, "test_agent", "native_file_write", writeInput)
	if err != nil {
		t.Fatalf("native_file_write failed: %v", err)
	}

	if res.Content == "" {
		t.Fatalf("expected write success content")
	}

	// Test native_file_read
	readInput := json.RawMessage(`{"path": "test.txt"}`)
	readRes, err := registry.Execute(ctx, "test_agent", "native_file_read", readInput)
	if err != nil {
		t.Fatalf("native_file_read failed: %v", err)
	}
	if readRes.Content != "Hello ActonOS Tools" {
		t.Fatalf("expected 'Hello ActonOS Tools', got '%s'", readRes.Content)
	}

	// Test Path Escape protection (2 levels up escape)
	escapeInput := json.RawMessage(`{"path": "../../etc/passwd"}`)
	_, err = registry.Execute(ctx, "test_agent", "native_file_read", escapeInput)
	if err == nil {
		t.Fatal("expected path escape error for 2 levels up, got nil")
	}

	// Private agent file tools must not access sibling data directories.
	oneLevelWrite := json.RawMessage(`{"path": "../skills/skill.md", "content": "Skill definition content"}`)
	if _, err := registry.Execute(ctx, "test_agent", "native_file_write", oneLevelWrite); err == nil {
		t.Fatal("expected one-level-up private workspace escape to fail")
	}

	// Test native_file_list
	listRes, err := registry.Execute(ctx, "test_agent", "native_file_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("native_file_list failed: %v", err)
	}
	if listRes.Content == "" {
		t.Fatal("expected file list content")
	}

	// Test native_file_search
	searchRes, err := registry.Execute(ctx, "test_agent", "native_file_search", json.RawMessage(`{"query": "ActonOS"}`))
	if err != nil {
		t.Fatalf("native_file_search failed: %v", err)
	}
	if searchRes.Content == "" {
		t.Fatal("expected search result content")
	}

	// Test native_exec
	execRes, err := registry.Execute(ctx, "test_agent", "native_exec", json.RawMessage(`{"command": "echo 'sandbox test'"}`))
	if err != nil {
		t.Fatalf("native_exec failed: %v", err)
	}
	if execRes.Content == "" {
		t.Fatal("expected exec result content")
	}

	// Test native_channel_notify
	notifyRes, err := registry.Execute(ctx, "test_agent", "native_channel_notify", json.RawMessage(`{"channel": "telegram", "message": "Test notification"}`))
	if err != nil {
		t.Fatalf("native_channel_notify failed: %v", err)
	}
	if notifyRes.Content == "" {
		t.Fatal("expected notify result content")
	}

	// Test native_file_delete
	delRes, err := registry.Execute(ctx, "test_agent", "native_file_delete", json.RawMessage(`{"path": "test.txt"}`))
	if err != nil {
		t.Fatalf("native_file_delete failed: %v", err)
	}
	if delRes.Content == "" {
		t.Fatal("expected delete result content")
	}
}

func TestToolRegistryEnforcesAuthorizationAndApproval(t *testing.T) {
	db, err := memory.Open(filepath.Join(t.TempDir(), "policy.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	defer db.Close()

	registry := NewToolRegistry(nil)
	RegisterNativeTools(registry, t.TempDir())
	registry.SetApprovalManager(NewApprovalManager(db.SQLDB()))
	registry.SetPolicyResolver(func(context.Context, string) (AgentToolPolicy, error) {
		return AgentToolPolicy{
			AuthorizedTools:   []string{"native_file_read", "native_file_write"},
			ApprovalThreshold: "High",
			AllowedPaths:      []string{"*"},
		}, nil
	})

	if _, err := registry.Execute(context.Background(), "agent", "native_exec", json.RawMessage(`{"command":"echo denied"}`)); !errors.Is(err, ErrToolUnauthorized) {
		t.Fatalf("expected unauthorized tool error, got %v", err)
	}

	input := json.RawMessage(`{"path":"approved.txt","content":"safe"}`)
	_, err = registry.Execute(context.Background(), "agent", "native_file_write", input)
	var approvalErr *ApprovalRequiredError
	if !errors.As(err, &approvalErr) {
		t.Fatalf("expected approval requirement, got %v", err)
	}
	approved, err := registry.approvals.Decide(context.Background(), approvalErr.Approval.ID, "approved", "tester", "")
	if err != nil {
		t.Fatalf("approving action: %v", err)
	}
	ctx := WithApprovalID(context.Background(), approved.ID)
	if _, err := registry.Execute(ctx, "agent", "native_file_write", input); err != nil {
		t.Fatalf("executing approved action: %v", err)
	}
	tampered := json.RawMessage(`{"path":"approved.txt","content":"tampered"}`)
	if _, err := registry.Execute(ctx, "agent", "native_file_write", tampered); !errors.Is(err, ErrApprovalInvalid) {
		t.Fatalf("expected exact-action hash rejection, got %v", err)
	}
}

// TestToolRegistryDoesNotRepublishReusedApproval guards the bug where a
// pending approval that already surfaced to the operator produced a second,
// duplicate "approval:required" event (and therefore a duplicate web
// notification) the next time the same exact action was attempted.
func TestToolRegistryDoesNotRepublishReusedApproval(t *testing.T) {
	db, err := memory.Open(filepath.Join(t.TempDir(), "policy-dedup.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	defer db.Close()

	eventBus := bus.NewEventBus()
	registry := NewToolRegistry(eventBus)
	RegisterNativeTools(registry, t.TempDir())
	registry.SetApprovalManager(NewApprovalManager(db.SQLDB()))
	registry.SetPolicyResolver(func(context.Context, string) (AgentToolPolicy, error) {
		return AgentToolPolicy{
			AuthorizedTools:   []string{"native_file_write"},
			ApprovalThreshold: "High",
			AllowedPaths:      []string{"*"},
		}, nil
	})

	events := eventBus.Subscribe("approval:required")
	defer eventBus.Unsubscribe("approval:required", events)

	input := json.RawMessage(`{"path":"approved.txt","content":"safe"}`)
	if _, err := registry.Execute(context.Background(), "agent", "native_file_write", input); err == nil {
		t.Fatal("expected approval requirement on first attempt")
	}
	if _, err := registry.Execute(context.Background(), "agent", "native_file_write", input); err == nil {
		t.Fatal("expected approval requirement on second (reused) attempt")
	}

	count := 0
	timeout := time.NewTimer(200 * time.Millisecond)
	defer timeout.Stop()
drain:
	for {
		select {
		case <-events:
			count++
		case <-timeout.C:
			break drain
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 approval:required event for a reused pending approval, got %d", count)
	}
}

func TestCommandPolicyBlocksDestructiveInput(t *testing.T) {
	registry := NewToolRegistry(nil)
	RegisterNativeTools(registry, t.TempDir())
	_, err := registry.Execute(context.Background(), "agent", "native_exec", json.RawMessage(`{"command":"rm -rf / --no-preserve-root"}`))
	if err == nil {
		t.Fatal("expected destructive command to be rejected")
	}
}

func TestToolRegistry_ToLLMToolDefinitions(t *testing.T) {
	registry := NewToolRegistry(nil)
	RegisterNativeTools(registry, t.TempDir())

	// Authorized subset
	defs := registry.ToLLMToolDefinitions([]string{"native_file_read", "native_sysinfo"})
	if len(defs) != 2 {
		t.Fatalf("expected 2 tool definitions, got %d", len(defs))
	}

	// Wildcard
	allDefs := registry.ToLLMToolDefinitions([]string{"*"})
	if len(allDefs) < 4 {
		t.Fatalf("expected all tools with wildcard, got %d", len(allDefs))
	}

	for _, def := range registry.ToLLMToolDefinitions([]string{"*"}, "native_cron_schedule") {
		if def.Function.Name == "native_cron_schedule" {
			t.Fatal("excluded cron tool was still offered to the LLM")
		}
	}

	_, err := registry.Execute(
		WithDeniedTools(context.Background(), "native_cron_schedule"),
		"agent",
		"native_cron_schedule",
		json.RawMessage(`{"action":"list"}`),
	)
	if !errors.Is(err, ErrToolDeniedInContext) {
		t.Fatalf("expected context-denied tool error, got %v", err)
	}

	// Allowlist: only native_sysinfo may execute, even though the tool is
	// registered and not otherwise denied.
	allowedCtx := WithAllowedTools(context.Background(), "native_sysinfo")
	if _, err := registry.Execute(allowedCtx, "agent", "native_file_read", json.RawMessage(`{"path":"x"}`)); !errors.Is(err, ErrToolDeniedInContext) {
		t.Fatalf("expected tool outside allowlist to be denied, got %v", err)
	}
	if _, err := registry.Execute(allowedCtx, "agent", "native_sysinfo", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected allowlisted tool to execute, got %v", err)
	}

	// Nested WithAllowedTools calls intersect rather than broaden.
	narrowed := WithAllowedTools(WithAllowedTools(context.Background(), "native_sysinfo", "native_file_read"), "native_file_read")
	if got := AllowedTools(narrowed); len(got) != 1 || got[0] != "native_file_read" {
		t.Fatalf("expected nested WithAllowedTools to intersect to [native_file_read], got %v", got)
	}
}

func TestSkillWatcher_LoadSkill(t *testing.T) {
	tempDir := t.TempDir()
	skillsDir := filepath.Join(tempDir, "skills")
	testSkillDir := filepath.Join(skillsDir, "echo_skill")
	_ = os.MkdirAll(testSkillDir, 0755)

	manifest := SkillManifest{
		Name:        "echo_test",
		Description: "Echoes input back",
		Entrypoint:  "run.sh",
	}
	manifestBytes, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(testSkillDir, "skill.json"), manifestBytes, 0644)
	_ = os.WriteFile(filepath.Join(testSkillDir, "run.sh"), []byte("#!/bin/sh\ncat\n"), 0755)

	registry := NewToolRegistry(nil)
	watcher := NewSkillWatcher(registry, skillsDir)
	watcher.ScanAll()

	tool, err := registry.Get("skill_echo_test")
	if err != nil {
		t.Fatalf("expected skill_echo_test to be registered: %v", err)
	}

	if tool.Description() != "Echoes input back" {
		t.Fatalf("unexpected description: %s", tool.Description())
	}

	// Delete skill directory and rescan
	_ = os.RemoveAll(testSkillDir)
	watcher.ScanAll()

	_, errAfterDelete := registry.Get("skill_echo_test")
	if errAfterDelete == nil {
		t.Fatal("expected skill_echo_test to be unregistered after directory removal")
	}
}

// TestActionHashSurvivesJSONRoundTrip guards the approval bug where a pending
// action was hashed from raw bytes. Persisting the run checkpoint marshals the
// arguments, which compacts whitespace and HTML-escapes <, > and &, so the
// resume-time hash no longer matched and every approve/reject failed.
func TestActionHashSurvivesJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		a, b json.RawMessage
	}{
		{
			name: "whitespace is insignificant",
			a:    json.RawMessage(`{"command": "echo hello"}`),
			b:    json.RawMessage(`{"command":"echo hello"}`),
		},
		{
			name: "key order is insignificant",
			a:    json.RawMessage(`{"command":"ls","cwd":"/tmp"}`),
			b:    json.RawMessage(`{"cwd":"/tmp","command":"ls"}`),
		},
		{
			name: "html escaping is insignificant",
			a:    json.RawMessage(`{"command":"echo a > b && cat <c"}`),
			b:    json.RawMessage(`{"command":"echo a > b && cat <c"}`),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if ActionHash("agent", "native_exec", test.a) != ActionHash("agent", "native_exec", test.b) {
				t.Fatalf("hash changed for equivalent input:\n a=%s\n b=%s", test.a, test.b)
			}
		})
	}

	// A genuinely different action must still produce a different hash.
	if ActionHash("agent", "native_exec", json.RawMessage(`{"command":"ls"}`)) ==
		ActionHash("agent", "native_exec", json.RawMessage(`{"command":"rm -rf x"}`)) {
		t.Fatal("distinct commands collided")
	}
	if ActionHash("agent-a", "native_exec", json.RawMessage(`{"command":"ls"}`)) ==
		ActionHash("agent-b", "native_exec", json.RawMessage(`{"command":"ls"}`)) {
		t.Fatal("distinct agents collided")
	}
	if ActionHash("agent", "native_exec", json.RawMessage(`{"command":"ls"}`)) ==
		ActionHash("agent", "native_file_write", json.RawMessage(`{"command":"ls"}`)) {
		t.Fatal("distinct tools collided")
	}
}

// TestApprovalValidatesAfterMarshalRoundTrip reproduces the exact failing path:
// request an approval, round-trip the pending arguments the way the run
// checkpoint does, then validate the approved action.
func TestApprovalValidatesAfterMarshalRoundTrip(t *testing.T) {
	db, err := memory.Open(filepath.Join(t.TempDir(), "approvals-roundtrip.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := NewApprovalManager(db.SQLDB())
	ctx := context.Background()

	// Formatted the way an LLM emits tool arguments, including a shell redirect.
	original := json.RawMessage(`{"command": "df -h > /tmp/disk.txt && echo <done>"}`)
	item, err := manager.Request(ctx, "trace-rt", "agent_system_core", "native_exec", "High", original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Decide(ctx, item.ID, "approved", "operator", "ok"); err != nil {
		t.Fatal(err)
	}

	// Emulate SaveCheckpoint -> LoadCheckpointByTrace.
	type checkpoint struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	encoded, err := json.Marshal(checkpoint{Arguments: original})
	if err != nil {
		t.Fatal(err)
	}
	var restored checkpoint
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if string(restored.Arguments) == string(original) {
		t.Fatal("expected the round trip to alter the bytes; test no longer covers the regression")
	}
	if err := manager.ValidateApproved(ctx, item.ID, "agent_system_core", "native_exec", restored.Arguments); err != nil {
		t.Fatalf("approved action rejected after checkpoint round trip: %v", err)
	}
}

// TestApprovalReopenAfterFailedExecution covers recovery when an approved
// action fails to execute: the record must return to pending so the operator
// can retry or reject instead of being stranded.
func TestApprovalReopenAfterFailedExecution(t *testing.T) {
	db, err := memory.Open(filepath.Join(t.TempDir(), "approvals-reopen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := NewApprovalManager(db.SQLDB())
	ctx := context.Background()

	item, err := manager.Request(ctx, "trace-reopen", "agent-a", "native_exec", "High", json.RawMessage(`{"command":"pwd"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Decide(ctx, item.ID, "approved", "operator", "ok"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reopen(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "pending" || reopened.DecidedAt != nil {
		t.Fatalf("approval was not reopened: %+v", reopened)
	}
	// A reopened approval must be decidable again, including rejection.
	rejected, err := manager.Decide(ctx, item.ID, "rejected", "operator", "changed my mind")
	if err != nil || rejected.Status != "rejected" {
		t.Fatalf("reopened approval could not be rejected: %+v err=%v", rejected, err)
	}
}

func TestApprovalManagerLifecycleAndExactHash(t *testing.T) {
	db, err := memory.Open(filepath.Join(t.TempDir(), "approvals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := NewApprovalManager(db.SQLDB())
	ctx := context.Background()
	input := json.RawMessage(`{"path":"safe.txt"}`)
	item, err := manager.Request(ctx, "trace-a", "agent-a", "native_file_write", "High", input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := manager.Request(ctx, "trace-other", "agent-a", "native_file_write", "High", input)
	if err != nil || duplicate.ID != item.ID {
		t.Fatalf("pending exact action was not deduplicated: %+v err=%v", duplicate, err)
	}
	if !item.IsNew() {
		t.Fatal("first Request() for a fresh action should report IsNew() == true")
	}
	if duplicate.IsNew() {
		t.Fatal("Request() reusing a pending approval should report IsNew() == false")
	}
	if err := manager.ValidateApproved(ctx, item.ID, item.AgentID, item.ToolName, input); !errors.Is(err, ErrApprovalInvalid) {
		t.Fatalf("pending approval unexpectedly validated: %v", err)
	}
	approved, err := manager.Decide(ctx, item.ID, "approved", "tester", "reviewed")
	if err != nil || approved.Status != "approved" || approved.DecidedAt == nil {
		t.Fatalf("approval decision failed: %+v err=%v", approved, err)
	}
	if err := manager.ValidateApproved(ctx, item.ID, item.AgentID, item.ToolName, input); err != nil {
		t.Fatalf("exact approved action rejected: %v", err)
	}
	if err := manager.ValidateApproved(ctx, item.ID, item.AgentID, item.ToolName, json.RawMessage(`{"path":"other.txt"}`)); !errors.Is(err, ErrApprovalInvalid) {
		t.Fatalf("tampered action was accepted: %v", err)
	}
	if _, err := manager.Decide(ctx, item.ID, "approved", "tester", "again"); !errors.Is(err, ErrApprovalNotPending) {
		t.Fatalf("second decision should fail: %v", err)
	}
	if _, err := manager.Decide(ctx, item.ID, "invalid", "tester", ""); err == nil {
		t.Fatal("invalid decision should fail")
	}
	items, err := manager.List(ctx, "approved", 0)
	if err != nil || len(items) != 1 || items[0].Reason != "reviewed" {
		t.Fatalf("approval list mismatch: %+v err=%v", items, err)
	}

	expiring, err := manager.Request(ctx, "", "agent-a", "native_exec", "High", json.RawMessage(`{"command":"pwd"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQLDB().Exec("UPDATE approvals SET expires_at = ? WHERE id = ?", time.Now().Add(-time.Minute), expiring.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.List(ctx, "expired", 10); err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateApproved(ctx, expiring.ID, expiring.AgentID, expiring.ToolName, expiring.Input); !errors.Is(err, ErrApprovalInvalid) {
		t.Fatalf("expired approval unexpectedly validated: %v", err)
	}
}

func TestToolRegistryMetadataNormalizationAndRemoval(t *testing.T) {
	registry := NewToolRegistry(nil)
	RegisterNativeTools(registry, t.TempDir())
	registry.SetAuditLogger(nil)
	if got := registry.ListByCategory("native"); len(got) == 0 {
		t.Fatal("expected native tools")
	}
	if _, err := registry.Get("missing"); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("expected tool not found, got %v", err)
	}
	registry.Unregister("native_sysinfo")
	if _, err := registry.Get("native_sysinfo"); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("unregister failed: %v", err)
	}
	for input, expectedFragment := range map[string]string{
		"":                       `{}`,
		`"https://example.com"`:  `"url":"https://example.com"`,
		`"{\"path\":\"a.txt\"}"`: `"path":"a.txt"`,
		`plain input`:            `"input":"plain input"`,
	} {
		if normalized := string(NormalizeToolInput(json.RawMessage(input))); !strings.Contains(normalized, expectedFragment) {
			t.Fatalf("normalization mismatch for %q: %s", input, normalized)
		}
	}
	ctx := WithTraceID(context.Background(), "trace-123")
	ctx = WithApprovalID(ctx, "approval-123")
	if TraceIDFromContext(ctx) != "trace-123" {
		t.Fatal("trace ID was not preserved")
	}
}

func TestNativeToolValidationAndSystemInfo(t *testing.T) {
	ctx := context.Background()
	for name, tool := range map[string]Tool{
		"http":       NewHTTPFetchTool(),
		"navigate":   NewBrowserNavigateTool(),
		"screenshot": NewBrowserScreenshotTool(t.TempDir()),
		"search":     NewWebSearchTool(),
	} {
		if _, err := tool.Execute(ctx, json.RawMessage(`{}`)); err == nil {
			t.Fatalf("%s accepted missing required input", name)
		}
	}
	if _, err := NewHTTPFetchTool().Execute(ctx, json.RawMessage(`{"url":"http://127.0.0.1/private"}`)); err == nil {
		t.Fatal("HTTP fetch accepted SSRF target")
	}
	if _, err := NewBrowserNavigateTool().Execute(ctx, json.RawMessage(`{"url":"localhost"}`)); err == nil {
		t.Fatal("browser accepted SSRF target")
	}
	if _, err := NewBrowserScreenshotTool(t.TempDir()).Execute(ctx, json.RawMessage(`{"url":"169.254.169.254"}`)); err == nil {
		t.Fatal("screenshot accepted metadata target")
	}
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "storage"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "storage", "acton.db"), []byte("db"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := NewSysInfoTool(dataDir).Execute(ctx, nil)
	if err != nil || !strings.Contains(info.Content, "nominal_healthy") ||
		!strings.Contains(info.Content, "online_wal_mode") {
		t.Fatalf("system info failed: result=%+v err=%v", info, err)
	}
	cron := NewCronScheduleTool(nil)
	if _, err := cron.Execute(ctx, json.RawMessage(`{"action":"list"}`)); err == nil {
		t.Fatal("cron tool accepted missing scheduler")
	}
	cron.SetScheduler(nil)
}

func TestWASMToolAndPluginManagerFailureLifecycle(t *testing.T) {
	ctx := context.Background()
	tool, err := NewWASMTool(ctx, "broken", "Broken plugin", json.RawMessage(`{"type":"object"}`), []byte("not wasm"))
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name() != "wasm_broken" || tool.Description() != "Broken plugin" ||
		tool.Category() != "wasm" || len(tool.ParametersSchema()) == 0 {
		t.Fatal("unexpected WASM tool metadata")
	}
	if _, err := tool.Execute(ctx, json.RawMessage(`{}`)); !errors.Is(err, ErrWASMExecution) {
		t.Fatalf("invalid WASM execution was not classified: %v", err)
	}
	if err := tool.Close(ctx); err != nil {
		t.Fatal(err)
	}

	registry := NewToolRegistry(nil)
	manager := NewWASMPluginManager(registry, t.TempDir())
	if err := manager.ScanAndRegisterPlugins(ctx); err != nil {
		t.Fatal(err)
	}
	if err := manager.LoadPlugin(ctx, "missing", filepath.Join(t.TempDir(), "missing.wasm")); err == nil {
		t.Fatal("missing WASM file should fail")
	}
	if err := manager.UnloadPlugin(ctx, "missing"); err == nil {
		t.Fatal("unloading missing plugin should fail")
	}
	empty := NewWASMPluginManager(registry, "")
	if err := empty.ScanAndRegisterPlugins(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFileWriteTool_RobustParsing(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	_ = os.MkdirAll(workspaceDir, 0755)

	agentID := "agent_parser"
	tool := NewFileWriteTool(workspaceDir)
	ctx := WithAgentID(context.Background(), agentID)
	privateDir := filepath.Join(workspaceDir, agentID, "workspace")

	// 1. Standard JSON
	_, err := tool.Execute(ctx, json.RawMessage(`{"path": "email1.html", "content": "<h1>Hello</h1>"}`))
	if err != nil {
		t.Fatalf("standard json failed: %v", err)
	}

	// 2. Multiline raw string JSON with unescaped newlines in HTML
	multilineInput := json.RawMessage("{\"path\": \"email2.html\", \"content\": \"<!doctype html>\n<html>\n<body>\n<h1>Title</h1>\n</body>\n</html>\"}")
	_, err = tool.Execute(ctx, multilineInput)
	if err != nil {
		t.Fatalf("multiline unescaped html failed: %v", err)
	}

	// 3. Stringified JSON literal
	stringifiedInput := json.RawMessage(`"{\"path\": \"email3.html\", \"content\": \"<h1>Stringified</h1>\"}"`)
	_, err = tool.Execute(ctx, stringifiedInput)
	if err != nil {
		t.Fatalf("stringified json failed: %v", err)
	}

	// Verify written files
	data1, _ := os.ReadFile(filepath.Join(privateDir, "email1.html"))
	if string(data1) != "<h1>Hello</h1>" {
		t.Fatalf("unexpected content in email1: %q", string(data1))
	}
	data2, _ := os.ReadFile(filepath.Join(privateDir, "email2.html"))
	if !strings.Contains(string(data2), "<h1>Title</h1>") {
		t.Fatalf("unexpected content in email2: %q", string(data2))
	}
	data3, _ := os.ReadFile(filepath.Join(privateDir, "email3.html"))
	if string(data3) != "<h1>Stringified</h1>" {
		t.Fatalf("unexpected content in email3: %q", string(data3))
	}
}

func TestAgentSpecificWorkspaceFileOperations(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	_ = os.MkdirAll(workspaceDir, 0755)

	eventBus := bus.NewEventBus()
	defer eventBus.Close()
	registry := NewToolRegistry(eventBus)
	RegisterNativeTools(registry, workspaceDir)

	ctx := context.Background()
	agentID := "agent_system_core"

	// 1. Agent writes a file with relative path "note.txt"
	writeInput := json.RawMessage(`{"path": "note.txt", "content": "agent note"}`)
	_, err := registry.Execute(ctx, agentID, "native_file_write", writeInput)
	if err != nil {
		t.Fatalf("native_file_write failed: %v", err)
	}

	// Verify it was written inside data/agents/{slug}/workspace/note.txt.
	expectedAgentFile := filepath.Join(tempDir, "agents", agentID, "workspace", "note.txt")
	data, err := os.ReadFile(expectedAgentFile)
	if err != nil {
		t.Fatalf("expected file in agent workspace %s: %v", expectedAgentFile, err)
	}
	if string(data) != "agent note" {
		t.Fatalf("expected 'agent note', got %q", string(data))
	}

	// 2. Agent reads back "note.txt"
	readInput := json.RawMessage(`{"path": "note.txt"}`)
	readRes, err := registry.Execute(ctx, agentID, "native_file_read", readInput)
	if err != nil {
		t.Fatalf("native_file_read failed: %v", err)
	}
	if readRes.Content != "agent note" {
		t.Fatalf("expected 'agent note', got %q", readRes.Content)
	}

	// 3. A user file in the legacy shared workspace must never be visible through native_file_read.
	sharedFile := filepath.Join(workspaceDir, "DELIVERIES.md")
	_ = os.WriteFile(sharedFile, []byte("# Deliveries Shared"), 0644)

	sharedReadInput := json.RawMessage(`{"path": "DELIVERIES.md"}`)
	if _, err := registry.Execute(ctx, agentID, "native_file_read", sharedReadInput); err == nil {
		t.Fatal("private agent file tool unexpectedly read the shared user workspace")
	}

	// 4. Non-existent file read returns "reading file" error, not "access denied / path escapes"
	missingInput := json.RawMessage(`{"path": "MISSING.md"}`)
	_, missingErr := registry.Execute(ctx, agentID, "native_file_read", missingInput)
	if missingErr == nil {
		t.Fatal("expected error on missing file")
	}
	if strings.Contains(missingErr.Error(), "access denied") || strings.Contains(missingErr.Error(), "escapes") {
		t.Fatalf("missing file returned security escape error instead of not-exist: %v", missingErr)
	}
}

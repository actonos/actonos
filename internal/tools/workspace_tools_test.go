package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/memory"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

type recordingWorkspaceMutationSink struct {
	fileID  string
	agentID string
	deleted bool
	calls   int
}

func (s *recordingWorkspaceMutationSink) NotifyWorkspaceMutation(_ context.Context, fileID, agentID string, deleted bool) error {
	s.fileID = fileID
	s.agentID = agentID
	s.deleted = deleted
	s.calls++
	return nil
}

func newWorkspaceToolRegistry(t *testing.T) (*ToolRegistry, *workspacepkg.Store, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := memory.Open(filepath.Join(dataDir, "storage", "test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := workspacepkg.NewStore(context.Background(), db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatalf("creating workspace store: %v", err)
	}
	registry := NewToolRegistry(nil)
	RegisterNativeToolsWithConfig(registry, NativeToolsConfig{
		DataDir:       dataDir,
		AgentsDir:     filepath.Join(dataDir, "agents"),
		UserWorkspace: store,
	})
	return registry, store, dataDir
}

func TestWorkspaceToolsRoundTripBinaryWithoutHostPaths(t *testing.T) {
	registry, _, dataDir := newWorkspaceToolRegistry(t)
	sink := &recordingWorkspaceMutationSink{}
	registry.SetWorkspaceMutationSink(sink)
	ctx := context.Background()
	agentID := "agent_alpha"
	content := []byte{0, 1, 2, 0xff, 0x80}
	encoded := base64.StdEncoding.EncodeToString(content)

	writeResult, err := registry.Execute(ctx, agentID, "native_workspace_write", json.RawMessage(`{
		"name":"báo cáo: / không-extension?", "content_base64":"`+encoded+`"
	}`))
	if err != nil {
		t.Fatalf("writing user workspace file: %v", err)
	}
	fileID, _ := writeResult.Data["file_id"].(string)
	if fileID == "" || sink.fileID != fileID || sink.agentID != agentID || sink.deleted || sink.calls != 1 {
		t.Fatalf("unexpected write result or mutation: result=%#v sink=%#v", writeResult, sink)
	}
	assertNoHostPath(t, dataDir, writeResult)

	readInput, _ := json.Marshal(map[string]any{"file_id": fileID, "encoding": "base64"})
	readResult, err := registry.Execute(ctx, "agent_beta", "native_workspace_read", readInput)
	if err != nil {
		t.Fatalf("reading shared user workspace file: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(readResult.Content)
	if err != nil || !bytes.Equal(decoded, content) {
		t.Fatalf("binary round trip mismatch: got=%x err=%v", decoded, err)
	}
	assertNoHostPath(t, dataDir, readResult)

	searchResult, err := registry.Execute(ctx, agentID, "native_workspace_search", json.RawMessage(`{"query":"báo cáo"}`))
	if err != nil || !strings.Contains(searchResult.Content, fileID) {
		t.Fatalf("search did not return opaque ID: result=%#v err=%v", searchResult, err)
	}
	assertNoHostPath(t, dataDir, searchResult)

	deleteInput, _ := json.Marshal(map[string]any{"file_id": fileID, "expected_version": writeResult.Data["version"]})
	deleteResult, err := registry.Execute(ctx, agentID, "native_workspace_delete", deleteInput)
	if err != nil {
		t.Fatalf("deleting user workspace file: %v", err)
	}
	if sink.calls != 2 || !sink.deleted || sink.fileID != fileID {
		t.Fatalf("unexpected delete mutation: %#v", sink)
	}
	assertNoHostPath(t, dataDir, deleteResult)
}

func TestWorkspaceWriteSchemaDisambiguatesTextAndBinary(t *testing.T) {
	registry, _, _ := newWorkspaceToolRegistry(t)
	tool, ok := registry.tools["native_workspace_write"]
	if !ok {
		t.Fatal("native_workspace_write was not registered")
	}
	var schema struct {
		Description string `json:"description"`
		OneOf       []struct {
			Required []string `json:"required"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(tool.ParametersSchema(), &schema); err != nil {
		t.Fatalf("invalid workspace write schema: %v", err)
	}
	if !strings.Contains(schema.Description, "exactly one") || len(schema.OneOf) != 2 {
		t.Fatalf("schema does not explain the mutually exclusive payloads: %+v", schema)
	}
	seen := map[string]bool{}
	for _, branch := range schema.OneOf {
		if len(branch.Required) != 1 {
			t.Fatalf("expected one required payload per branch, got %#v", branch.Required)
		}
		seen[branch.Required[0]] = true
	}
	if !seen["content"] || !seen["content_base64"] {
		t.Fatalf("schema branches do not cover both payloads: %#v", seen)
	}
}

func TestWorkspaceWriteRejectsAmbiguousPayload(t *testing.T) {
	registry, _, _ := newWorkspaceToolRegistry(t)
	_, err := registry.Execute(context.Background(), "agent_alpha", "native_workspace_write", json.RawMessage(`{"name":"ambiguous","content":"text","content_base64":"dGV4dA=="}`))
	if !errors.Is(err, ErrExecutionFailed) || !strings.Contains(err.Error(), "provide only one") {
		t.Fatalf("ambiguous payload error = %v", err)
	}
}

func TestPrivateAgentFileToolsAreMutuallyIsolated(t *testing.T) {
	registry, _, dataDir := newWorkspaceToolRegistry(t)
	ctx := context.Background()
	if _, err := registry.Execute(ctx, "agent_alpha", "native_file_write", json.RawMessage(`{"path":"private.txt","content":"alpha-only"}`)); err != nil {
		t.Fatalf("writing private file: %v", err)
	}
	if _, err := registry.Execute(ctx, "agent_beta", "native_file_read", json.RawMessage(`{"path":"private.txt"}`)); err == nil {
		t.Fatal("agent beta unexpectedly read agent alpha's private file")
	}
	alphaPath := filepath.Join(dataDir, "agents", "agent_alpha", "workspace", "private.txt")
	data, err := os.ReadFile(alphaPath)
	if err != nil || string(data) != "alpha-only" {
		t.Fatalf("private file not stored under the correct root: data=%q err=%v", data, err)
	}
	for _, input := range []string{`{"path":"../../../../../etc/passwd"}`, `{"path":"../../../../outside.txt"}`} {
		if _, err := registry.Execute(ctx, "agent_alpha", "native_file_read", json.RawMessage(input)); err == nil {
			t.Fatalf("private path escape unexpectedly succeeded: %s", input)
		}
	}
}

func TestWorkspaceWriteRejectsStaleVersion(t *testing.T) {
	registry, _, _ := newWorkspaceToolRegistry(t)
	ctx := context.Background()
	created, err := registry.Execute(ctx, "agent_alpha", "native_workspace_write", json.RawMessage(`{"name":"draft","content":"one"}`))
	if err != nil {
		t.Fatal(err)
	}
	fileID := created.Data["file_id"].(string)
	update, _ := json.Marshal(map[string]any{"file_id": fileID, "content": "two", "expected_version": created.Data["version"]})
	if _, err := registry.Execute(ctx, "agent_alpha", "native_workspace_write", update); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(ctx, "agent_alpha", "native_workspace_write", update); !errors.Is(err, ErrExecutionFailed) || !strings.Contains(err.Error(), workspacepkg.ErrVersion.Error()) {
		t.Fatalf("stale update error = %v, want wrapped ErrVersion", err)
	}
}

func assertNoHostPath(t *testing.T, dataDir string, result *ToolResult) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	normalizedDataDir := strings.ToLower(strings.ReplaceAll(dataDir, `\\`, "/"))
	normalizedResult := strings.ToLower(strings.ReplaceAll(string(raw), `\\`, "/"))
	if strings.Contains(normalizedResult, normalizedDataDir) || strings.Contains(normalizedResult, "absolute_path") {
		t.Fatalf("tool result leaked a host path: %s", raw)
	}
}

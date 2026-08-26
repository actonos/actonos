package plugin

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/workspace"
	"github.com/tetratelabs/wazero"
	_ "modernc.org/sqlite"
)

var testWasmBytes = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x06, 0x01, 0x60, 0x01, 0x7f, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00,
	0x05, 0x03, 0x01, 0x00, 0x01,
	0x07, 0x0f, 0x01, 0x0b, 0x61, 0x63, 0x74, 0x6f, 0x6e, 0x5f, 0x61, 0x6c, 0x6c, 0x6f, 0x63, 0x00, 0x00,
	0x0a, 0x07, 0x01, 0x05, 0x00, 0x41, 0xe4, 0x00, 0x0b,
}

func TestHostSyscallWorkspaceSaveAndRead(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	wsStore, err := workspace.NewStore(ctx, db, filepath.Join(tempDir, "workspace"))
	if err != nil {
		t.Fatal(err)
	}

	manifest := PluginManifest{
		ID: "test-plugin",
		Permissions: PluginPermissions{
			Workspace: true,
		},
	}
	gate := NewSecurityGate(manifest)

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	mod, err := r.Instantiate(ctx, testWasmBytes)
	if err != nil {
		t.Fatal(err)
	}

	hostCtx := &HostContext{
		PluginID:  "test-plugin",
		Manifest:  manifest,
		Gate:      gate,
		Workspace: wsStore,
		AllocFn:   mod.ExportedFunction("acton_alloc"),
		Memory:    mod.Memory(),
	}

	execCtx := WithHostContext(ctx, hostCtx)

	// 1. Test saving a base64 image into nested workspace directory
	testPNGData := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4")
	testBase64 := base64.StdEncoding.EncodeToString(testPNGData)

	saveReq := WorkspaceSavePayload{
		Path:          "images/generated/cat.png",
		ContentBase64: testBase64,
		MIMEType:      "image/png",
	}
	saveReqBytes, _ := json.Marshal(saveReq)

	_ = mod.Memory().Write(100, saveReqBytes)

	resLen := workspaceSaveFile(execCtx, mod, 100, uint32(len(saveReqBytes)))
	if resLen <= 0 {
		t.Fatalf("expected positive response length, got %d", resLen)
	}

	var saveResp WorkspaceFileResponse
	if err := json.Unmarshal(hostCtx.LastResponse, &saveResp); err != nil {
		t.Fatalf("unmarshaling save response: %v", err)
	}

	if saveResp.Error != "" {
		t.Fatalf("unexpected save error: %s", saveResp.Error)
	}
	if saveResp.Name != "cat.png" {
		t.Errorf("expected name 'cat.png', got %s", saveResp.Name)
	}
	if saveResp.Path != "images/generated/cat.png" {
		t.Errorf("expected path 'images/generated/cat.png', got %s", saveResp.Path)
	}
	if saveResp.URL != "/api/workspace/raw?path=images%2Fgenerated%2Fcat.png" {
		t.Errorf("unexpected URL: %s", saveResp.URL)
	}
	if saveResp.SizeBytes != int64(len(testPNGData)) {
		t.Errorf("expected size %d, got %d", len(testPNGData), saveResp.SizeBytes)
	}

	// 2. Verify file exists in workspace store
	node, file, err := wsStore.Open(ctx, saveResp.ID)
	if err != nil {
		t.Fatalf("opening file in store: %v", err)
	}
	defer file.Close()

	if node.Name != "cat.png" {
		t.Errorf("stored node name mismatch: %s", node.Name)
	}

	// 3. Test reading the file back via workspaceReadFile
	readReq := map[string]string{
		"path": "images/generated/cat.png",
	}
	readReqBytes, _ := json.Marshal(readReq)
	_ = mod.Memory().Write(2000, readReqBytes)

	readLen := workspaceReadFile(execCtx, mod, 2000, uint32(len(readReqBytes)))
	if readLen <= 0 {
		t.Fatalf("expected positive read length, got %d", readLen)
	}

	var readResp WorkspaceFileResponse
	if err := json.Unmarshal(hostCtx.LastResponse, &readResp); err != nil {
		t.Fatalf("unmarshaling read response: %v", err)
	}

	if readResp.Error != "" {
		t.Fatalf("unexpected read error: %s", readResp.Error)
	}
	decodedContent, err := base64.StdEncoding.DecodeString(readResp.ContentBase64)
	if err != nil {
		t.Fatalf("decoding base64 content: %v", err)
	}
	if string(decodedContent) != string(testPNGData) {
		t.Errorf("content mismatch: got %v, want %v", decodedContent, testPNGData)
	}

	// 4. Test legacy packed pointer functions
	packedRes := hostWorkspaceSave(execCtx, mod, 100, uint32(len(saveReqBytes)))
	if packedRes == 0 {
		t.Error("expected non-zero packed pointer result from hostWorkspaceSave")
	}
	packedRead := hostWorkspaceRead(execCtx, mod, 2000, uint32(len(readReqBytes)))
	if packedRead == 0 {
		t.Error("expected non-zero packed pointer result from hostWorkspaceRead")
	}
}

func TestHostSyscallWorkspacePermissionDenied(t *testing.T) {
	ctx := context.Background()
	manifest := PluginManifest{
		ID: "unauthorized-plugin",
		Permissions: PluginPermissions{
			Workspace: false,
			Storage:   false,
		},
	}
	gate := NewSecurityGate(manifest)

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	mod, err := r.Instantiate(ctx, testWasmBytes)
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	db, _ := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	defer db.Close()
	wsStore, _ := workspace.NewStore(ctx, db, filepath.Join(tempDir, "workspace"))

	hostCtx := &HostContext{
		PluginID:  "unauthorized-plugin",
		Manifest:  manifest,
		Gate:      gate,
		Workspace: wsStore,
	}
	execCtx := WithHostContext(ctx, hostCtx)

	saveReqBytes := []byte(`{"path":"test.txt","content":"hello"}`)
	_ = mod.Memory().Write(100, saveReqBytes)

	workspaceSaveFile(execCtx, mod, 100, uint32(len(saveReqBytes)))

	var saveResp WorkspaceFileResponse
	_ = json.Unmarshal(hostCtx.LastResponse, &saveResp)
	if saveResp.Error == "" {
		t.Error("expected error for unauthorized workspace access, got nil")
	}
}

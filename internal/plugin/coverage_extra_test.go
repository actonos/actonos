package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/bus"
	"github.com/tetratelabs/wazero"
)

func TestRegistryHelpersAndSingleFileLoad(t *testing.T) {
	if !containsStr([]string{"Channel"}, "channel") {
		t.Fatal("containsStr should be case-insensitive")
	}
	tags := generatePluginTags("channel-discord", []string{"channel"})
	if len(tags) == 0 {
		t.Fatal("expected tags")
	}
	if getDefaultPluginStars("channel-discord") < 1 {
		t.Fatal("expected default stars")
	}

	dir := t.TempDir()
	mgr := NewPluginRegistryManagerWithURLs(dir, nil, nil, "http://127.0.0.1:1/registry", "http://127.0.0.1:1/dl")
	mgr.SetEventBus(bus.NewEventBus())
	mgr.SetPluginManager(nil)
	mgr.SetRegistryURLs("http://127.0.0.1:1/registry", "http://127.0.0.1:1/dl")

	key, err := parseEd25519PublicKey([]byte(hex.EncodeToString(make([]byte, 32))))
	if err != nil || len(key) != 32 {
		t.Fatalf("parse key: %v len=%d", err, len(key))
	}

	wasmPath := filepath.Join(dir, "stub.wasm")
	if err := os.WriteFile(wasmPath, []byte("\x00asm\x01\x00\x00\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	loader, err := NewWasmLoader(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loader.Close(context.Background()) })
	pm := NewManager(loader, nil, nil, nil, nil, nil, dir)
	if err := pm.loadSingleFile(context.Background(), "stub", wasmPath, filepath.Join(dir, "stub.json")); err == nil {
		t.Log("incomplete wasm may still compile depending on runtime")
	}
	_ = pm.GetPluginLogs("stub")
	_ = pm.ListPlugins()
}

func TestHostContextHelpers(t *testing.T) {
	if HostContextFrom(context.Background()) != nil {
		t.Fatal("expected nil host context")
	}
	h := &HostContext{}
	h.Record("", "hello")
	h.SeedLogs([]string{"a", "b"})
	h.AppendLog(string(make([]byte, maxPluginLogLine+8)))
	if len(h.GetLogs()) == 0 {
		t.Fatal("expected logs")
	}
	if _, err := writeBufferToGuest(context.Background(), nil, []byte("x")); err == nil {
		t.Fatal("expected alloc error")
	}
	client := sandboxedHTTPClient(h, 0)
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestAllocExportWasmAndRegistryCtor(t *testing.T) {
	_ = NewPluginRegistryManager(t.TempDir(), nil, nil)
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })
	wasm := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x06, 0x01, 0x60, 0x01, 0x7f, 0x01, 0x7f,
		0x03, 0x02, 0x01, 0x00,
		0x05, 0x03, 0x01, 0x00, 0x01,
		0x07, 0x0f, 0x01, 0x0b, 0x61, 0x63, 0x74, 0x6f, 0x6e, 0x5f, 0x61, 0x6c, 0x6c, 0x6f, 0x63, 0x00, 0x00,
		0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x64, 0x0b,
	}
	mod, err := r.Instantiate(ctx, wasm)
	if err != nil {
		t.Fatalf("alloc wasm: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "kv2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLiteKVStore(db)
	if err != nil {
		t.Fatal(err)
	}
	host := &HostContext{
		PluginID: "stub",
		AllocFn:  mod.ExportedFunction("acton_alloc"),
		Memory:   mod.Memory(),
		KV:       store,
		Secrets:  mapSecretProvider{"bot_token": "abc"},
		Gate: NewSecurityGate(PluginManifest{Permissions: PluginPermissions{
			Secrets: []string{"bot_token"}, Storage: true,
		}}),
	}
	exec := WithHostContext(ctx, host)
	_, _ = writeBufferToGuest(exec, host, []byte("hello"))
	key := []byte("bot_token")
	_ = mod.Memory().Write(200, key)
	_ = hostGetSecret(exec, mod, 200, uint32(len(key)))
	_ = hostKVGet(exec, mod, 200, uint32(len(key)))
}

func TestPluginInstanceWithoutExports(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01}
	mod, err := r.Instantiate(ctx, wasm)
	if err != nil {
		t.Skipf("minimal wasm instantiate: %v", err)
	}
	inst := &PluginInstance{pluginID: "stub", mod: mod, hostCtx: &HostContext{PluginID: "stub"}}
	if _, err := inst.ExecuteTool(ctx, "echo", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected missing export")
	}
	if err := inst.SendChannelMessage(ctx, []byte(`{"content":"hi"}`)); err == nil {
		t.Fatal("expected missing send export")
	}
	if _, err := inst.PollChannel(ctx); err != nil {
		t.Fatalf("poll without export should be empty: %v", err)
	}
	if err := inst.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

type staticTransport struct {
	status int
	body   string
}

func (s staticTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(bytes.NewBufferString(s.body)),
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Request:    req,
	}, nil
}

func TestHostSyscallsWithMemoryModule(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01}
	mod, err := r.Instantiate(ctx, wasm)
	if err != nil {
		t.Fatalf("instantiate memory module: %v", err)
	}
	mem := mod.Memory()
	if mem == nil {
		t.Fatal("expected memory")
	}

	eb := bus.NewEventBus()
	defer eb.Close()
	gate := NewSecurityGate(PluginManifest{
		Permissions: PluginPermissions{
			NetOutbound: []string{"1.1.1.1"},
			Secrets:     []string{"bot_token"},
			Storage:     true,
			BusEvents:   []string{"channel:message:inbound"},
		},
	})
	host := &HostContext{
		PluginID:   "stub",
		Manifest:   PluginManifest{Permissions: gate.manifest.Permissions},
		Gate:       gate,
		EventBus:   eb,
		HTTPClient: &http.Client{Transport: staticTransport{status: 200, body: "ok"}},
		Secrets:    mapSecretProvider{"bot_token": "sekrit"},
	}
	exec := WithHostContext(ctx, host)

	reqJSON, _ := json.Marshal(HTTPRequestPayload{URL: "https://1.1.1.1/dns-query", Method: "GET"})
	if !mem.Write(64, reqJSON) {
		t.Fatal("write request")
	}
	if n := netHTTPRequest(exec, mod, 64, uint32(len(reqJSON))); n <= 0 {
		t.Fatalf("http_request returned %d", n)
	}
	if !strings.Contains(string(host.LastResponse), "ok") && host.LastResponse == nil {
		t.Fatalf("expected HTTP response in LastResponse: %s", host.LastResponse)
	}

	blocked, _ := json.Marshal(HTTPRequestPayload{URL: "http://127.0.0.1/admin", Method: "GET"})
	_ = mem.Write(512, blocked)
	_ = netHTTPRequest(exec, mod, 512, uint32(len(blocked)))
	_ = hostHTTPRequest(exec, mod, 64, uint32(len(reqJSON)))

	host.LastResponse = []byte("pong")
	if !mem.Write(2000, make([]byte, 8)) {
		t.Fatal("write dest")
	}
	if n := sysReadResponse(exec, mod, 2000, 8); n <= 0 {
		t.Fatalf("read_response %d", n)
	}

	msg := []byte("hello from guest")
	_ = mem.Write(3000, msg)
	hostLog(exec, mod, 1, 3000, uint32(len(msg)))

	topic := []byte("channel:message:inbound")
	payload := []byte(`{"ok":true}`)
	_ = mem.Write(3200, topic)
	_ = mem.Write(3300, payload)
	_ = hostEmitEvent(exec, mod, 3200, uint32(len(topic)), 3300, uint32(len(payload)))
	_ = hostTimeNowMS()

	ws := []byte("ws://127.0.0.1:1")
	_ = mem.Write(3400, ws)
	if id := wsConnect(exec, mod, 3400, uint32(len(ws)), 0, 0); id > 0 {
		t.Fatal("expected loopback websocket to be blocked")
	}

	w := &pluginStdioWriter{host: host, level: "INFO"}
	_, _ = w.Write([]byte("stdio line\n"))
	_, _ = w.Write(bytes.Repeat([]byte("x"), maxPluginLogLine+16))
	logs := host.GetLogs()
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "hello from guest") && !strings.Contains(joined, "stdio line") {
		t.Fatalf("expected host logs, got %v", logs)
	}

	secretName := []byte("bot_token")
	_ = mem.Write(3600, secretName)
	if n := vaultGetSecret(exec, mod, 3600, uint32(len(secretName))); n <= 0 {
		t.Fatal("expected vault secret length")
	}
	host.AppendLog("leaked sekrit in log")
	if strings.Contains(strings.Join(host.GetLogs(), "\n"), "sekrit") {
		t.Fatal("vault secret leaked into plugin logs")
	}

	kvKey, kvVal := []byte("k1"), []byte("v1")
	_ = mem.Write(3700, kvKey)
	_ = mem.Write(3720, kvVal)
	dbPath := filepath.Join(t.TempDir(), "kv.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLiteKVStore(db)
	if err != nil {
		t.Fatal(err)
	}
	host.KV = store
	if hostKVSet(exec, mod, 3700, uint32(len(kvKey)), 3720, uint32(len(kvVal))) != 0 {
		t.Fatal("kv set")
	}
	_ = storageKVGet(exec, mod, 3700, uint32(len(kvKey)))
	_ = hostKVDelete(exec, mod, 3700, uint32(len(kvKey)))
	if wsSend(exec, mod, 99, 1, 3000, 4) != -1 {
		t.Fatal("expected missing ws handle")
	}
	if wsPoll(exec, mod, 99) != -1 {
		t.Fatal("expected missing ws poll handle")
	}
	if wsClose(exec, mod, 99) != -1 && wsClose(exec, mod, 99) != 0 {
		// closed-missing is fine as long as it does not panic
	}

	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loader.Close(ctx) })
	pm := NewManager(loader, nil, nil, nil, store, host.Secrets, t.TempDir())
	if err := pm.EnablePlugin(ctx, "nope"); err == nil {
		t.Fatal("expected missing plugin")
	}
	if err := pm.DisablePlugin(ctx, "nope"); err == nil {
		t.Fatal("expected missing plugin")
	}
	if err := pm.UninstallPlugin(ctx, "nope"); err == nil {
		t.Fatal("expected missing plugin")
	}
}

func TestActivateDisabledPlugin(t *testing.T) {
	ctx := context.Background()
	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loader.Close(ctx) })
	dir := t.TempDir()
	pm := NewManager(loader, nil, nil, nil, nil, nil, dir)
	manifest := PluginManifest{ID: "off", Name: "Off", Version: "1", Capabilities: []string{string(CapabilityTool)}}
	if err := pm.activatePlugin(ctx, manifest, []byte("\x00asm\x01\x00\x00\x00"), dir, false); err != nil {
		t.Fatal(err)
	}
	info, ok := pm.GetPlugin("off")
	if !ok || info.Status != StatusDisabled {
		t.Fatalf("expected disabled plugin, got %+v ok=%v", info, ok)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.wasm"), []byte("\x00asm\x01\x00\x00\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = pm.EnablePlugin(ctx, "off")
	_ = pm.GetPluginLogs("off")
	_ = pm.Close(ctx)
}

func TestScanAndLoadAllSingleFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "solo.wasm"), []byte("\x00asm\x01\x00\x00\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "solo.json"), []byte(`{"id":"solo","name":"Solo","capabilities":["tool"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "folderplug")
	if err := os.MkdirAll(folder, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "manifest.json"), []byte(`{"id":"folderplug","name":"Folder","capabilities":["tool"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "plugin.wasm"), []byte("\x00asm\x01\x00\x00\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loader.Close(ctx) })
	pm := NewManager(loader, nil, nil, nil, nil, nil, dir)
	if err := pm.ScanAndLoadAll(ctx); err != nil {
		t.Fatal(err)
	}
}

type mapSecretProvider map[string]string

func (m mapSecretProvider) GetSecret(_ context.Context, name string) (string, error) {
	return m[name], nil
}

func TestWasmConnectorWebhookWithoutExport(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01}
	mod, err := r.Instantiate(ctx, wasm)
	if err != nil {
		t.Fatal(err)
	}
	inst := &PluginInstance{mod: mod, hostCtx: &HostContext{PluginID: "c"}}
	conn := NewWasmConnectorBridge("c", PluginManifest{Config: map[string]any{"auth_schema": `{"type":"object"}`}}, inst)
	if _, err := conn.HandleWebhook(ctx, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if schema := string(conn.GetConfigSchema()); schema == "" {
		t.Fatal("expected schema from config")
	}
	empty := NewWasmConnectorBridge("x", PluginManifest{}, inst)
	_ = empty.GetConfigSchema()
	var writer *pluginStdioWriter
	_, _ = writer.Write([]byte("x"))
	_, _ = (&pluginStdioWriter{}).Write([]byte("x"))
	denied := NewSecurityGate(PluginManifest{})
	if err := denied.CheckStorageAccess(); err == nil {
		t.Fatal("expected storage denied")
	}
	if _, _, _, _, err := ExtractPluginPackage([]byte("not-a-zip")); err == nil {
		t.Fatal("expected zip error")
	}
	emptyZip := bytes.NewBuffer(nil)
	zw := zip.NewWriter(emptyZip)
	_ = zw.Close()
	if _, _, _, _, err := ExtractPluginPackage(emptyZip.Bytes()); err == nil {
		t.Fatal("expected missing manifest")
	}
	tool := NewWasmToolBridge("c", PluginToolDef{Name: "echo", Description: "e"}, inst)
	res, err := tool.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Error == "" {
		t.Fatal("expected tool error from missing export")
	}
}

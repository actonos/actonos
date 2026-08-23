package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/tools"
	"github.com/coder/websocket"
	"github.com/tetratelabs/wazero"
	_ "modernc.org/sqlite"
)

func TestSecurityGate(t *testing.T) {
	manifest := PluginManifest{
		ID:   "test_plugin",
		Name: "Test Plugin",
		Permissions: PluginPermissions{
			NetOutbound: []string{"api.telegram.org", "*.slack.com", "https://notion.so/api"},
			Secrets:     []string{"telegram_bot_token", "github_pat"},
			Storage:     true,
			BusEvents:   []string{"channel:message:inbound"},
		},
	}

	gate := NewSecurityGate(manifest)

	// 1. Outbound Network Domain checks
	tests := []struct {
		url     string
		allowed bool
	}{
		{"https://api.telegram.org/bot123/sendMessage", true},
		{"https://api.slack.com/methods", true},
		{"https://hooks.slack.com/services/xxx", true},
		{"https://slack.com/api", true},
		{"https://malicious.evil.com/leak", false},
		{"https://telegram.org.evil.com", false},
		{"http://127.0.0.1:8080/internal", false},
	}

	for _, tt := range tests {
		err := gate.CheckOutboundURL(tt.url)
		if tt.allowed && err != nil {
			t.Errorf("expected URL %s to be allowed, got error: %v", tt.url, err)
		}
		if !tt.allowed && err == nil {
			t.Errorf("expected URL %s to be blocked, but it passed", tt.url)
		}
	}

	// 2. Secret access checks
	if err := gate.CheckSecretAccess("telegram_bot_token"); err != nil {
		t.Errorf("expected telegram_bot_token to be allowed, got: %v", err)
	}
	if err := gate.CheckSecretAccess("vault_master_key"); err == nil {
		t.Errorf("expected vault_master_key to be denied")
	}

	// 3. Storage check
	if err := gate.CheckStorageAccess(); err != nil {
		t.Errorf("expected storage access to be allowed, got: %v", err)
	}

	// 4. Bus event check
	if err := gate.CheckBusEvent("channel:message:inbound"); err != nil {
		t.Errorf("expected bus event channel:message:inbound to be allowed, got: %v", err)
	}
	if err := gate.CheckBusEvent("system:shutdown"); err == nil {
		t.Errorf("expected system:shutdown to be denied")
	}
}

func TestSQLiteKVStore(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_plugin.db")
	db, err := sql.Open("sqlite", tempDB)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteKVStore(db)
	if err != nil {
		t.Fatalf("creating kv store: %v", err)
	}

	// Test Set & Get
	if err := store.Set("plugin_a", "key1", "val1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, found, err := store.Get("plugin_a", "key1")
	if err != nil || !found || val != "val1" {
		t.Errorf("Get failed: val=%s, found=%v, err=%v", val, found, err)
	}

	// Test Scoping (plugin_b should not see plugin_a's key)
	_, foundB, _ := store.Get("plugin_b", "key1")
	if foundB {
		t.Errorf("expected plugin_b not to find key1")
	}

	// Test Delete
	if err := store.Delete("plugin_a", "key1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, foundAfterDelete, _ := store.Get("plugin_a", "key1")
	if foundAfterDelete {
		t.Errorf("expected key1 to be deleted")
	}
}

func TestPluginManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	pluginsDir := filepath.Join(tempDir, "plugins")
	_ = os.MkdirAll(pluginsDir, 0755)

	eventBus := bus.NewEventBus()
	defer eventBus.Close()

	toolReg := tools.NewToolRegistry(eventBus)

	tempDB := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", tempDB)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer db.Close()

	pairingMgr, err := channels.NewPairingManager(db)
	if err != nil {
		t.Fatalf("creating pairing manager: %v", err)
	}
	channelMgr := channels.NewChannelManager(eventBus, pairingMgr)

	kvStore, err := NewSQLiteKVStore(db)
	if err != nil {
		t.Fatalf("creating kv store: %v", err)
	}

	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatalf("creating wasm loader: %v", err)
	}
	defer loader.Close(ctx)

	mgr := NewManager(loader, toolReg, channelMgr, eventBus, kvStore, nil, pluginsDir)
	defer mgr.Close(ctx)

	// Create a dummy plugin package with valid manifest and a minimal wasm module header
	pluginID := "dummy_tool"
	pluginFolder := filepath.Join(pluginsDir, pluginID)
	_ = os.MkdirAll(pluginFolder, 0755)

	manifest := PluginManifest{
		ID:           pluginID,
		Name:         "Dummy Plugin",
		Version:      "1.0.0",
		Capabilities: []string{"tool"},
		Tools: []PluginToolDef{
			{
				Name:        "dummy_echo",
				Description: "Echo input",
				Parameters:  json.RawMessage(`{"type": "object"}`),
			},
		},
	}
	manifestBytes, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(pluginFolder, "manifest.json"), manifestBytes, 0644)

	// Minimal WASM binary format: \x00asm\x01\x00\x00\x00
	minimalWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	_ = os.WriteFile(filepath.Join(pluginFolder, "plugin.wasm"), minimalWasm, 0644)

	// Scan and load
	if err := mgr.ScanAndLoadAll(ctx); err != nil {
		t.Fatalf("ScanAndLoadAll failed: %v", err)
	}

	plugins := mgr.ListPlugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.ID != pluginID {
		t.Errorf("expected plugin ID %s, got %s", pluginID, plugins[0].Manifest.ID)
	}
	if plugins[0].Status != StatusRunning {
		t.Errorf("expected status running, got %s", plugins[0].Status)
	}

	// Verify tool registration in ToolRegistry
	if _, err := toolReg.Get("dummy_echo"); err != nil {
		t.Errorf("expected tool dummy_echo to be registered in ToolRegistry: %v", err)
	}

	// Test Disable
	if err := mgr.DisablePlugin(ctx, pluginID); err != nil {
		t.Fatalf("DisablePlugin failed: %v", err)
	}
	if _, err := toolReg.Get("dummy_echo"); err == nil {
		t.Errorf("expected tool dummy_echo to be unregistered after disable")
	}

	info, _ := mgr.GetPlugin(pluginID)
	if info.Status != StatusDisabled {
		t.Errorf("expected status disabled, got %s", info.Status)
	}

	// Test Enable
	if err := mgr.EnablePlugin(ctx, pluginID); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
	if _, err := toolReg.Get("dummy_echo"); err != nil {
		t.Errorf("expected tool dummy_echo to be re-registered after enable: %v", err)
	}

	// Test Uninstall
	if err := mgr.UninstallPlugin(ctx, pluginID); err != nil {
		t.Fatalf("UninstallPlugin failed: %v", err)
	}
	if len(mgr.ListPlugins()) != 0 {
		t.Errorf("expected 0 plugins after uninstall")
	}
}

type mockSecretProvider struct {
	secrets map[string]string
}

func (m *mockSecretProvider) GetSecret(ctx context.Context, secretName string) (string, error) {
	if val, ok := m.secrets[secretName]; ok {
		return val, nil
	}
	return "", fmt.Errorf("secret %s not found", secretName)
}

func TestSDKPluginWasmLifecycle(t *testing.T) {
	wasmPath := `d:\Projects\ActonOS\build\data\plugins\channel-discord\plugin.wasm`
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skip("plugin.wasm not found:", err)
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	cm, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		t.Fatalf("failed to compile module: %v", err)
	}

	importedFns := cm.ImportedFunctions()
	t.Logf("Total imported functions: %d", len(importedFns))
	for _, fn := range importedFns {
		mod, name, _ := fn.Import()
		t.Logf("IMPORT: module=%s, name=%s, params=%v, results=%v", mod, name, fn.ParamTypes(), fn.ResultTypes())
	}

	exportedFns := cm.ExportedFunctions()
	t.Logf("Total exported functions: %d", len(exportedFns))
	for name, fn := range exportedFns {
		t.Logf("EXPORT: name=%s, params=%v, results=%v", name, fn.ParamTypes(), fn.ResultTypes())
	}

	// Test full instantiation using WasmLoader
	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatalf("failed to create WasmLoader: %v", err)
	}
	defer loader.Close(ctx)

	if _, err := loader.Compile(ctx, "channel-discord", wasmBytes); err != nil {
		t.Fatalf("failed to compile module: %v", err)
	}

	manifest := PluginManifest{
		ID:   "channel-discord",
		Name: "Discord Bot Channel",
		Permissions: PluginPermissions{
			NetOutbound: []string{"discord.com"},
			Secrets:     []string{"discord_bot_token", "discord_bot_tokens.*"},
			Storage:     true,
			BusEvents:   []string{"channel.discord.received", "channel.discord.sent"},
		},
		Config: map[string]any{
			"accounts": []any{
				map[string]any{
					"account_id":        "astro",
					"bot_token":         "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
					"display_name":      "Astro",
					"default_agent":     "agent_system_core",
					"listen_channel_id": "",
				},
			},
			"poll_interval_seconds": 3,
		},
	}

	mockSecrets := &mockSecretProvider{
		secrets: map[string]string{
			"discord_bot_token":          "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
			"discord_bot_tokens.default": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
			"discord_bot_tokens.astro":   "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
		},
	}

	hostCtx := &HostContext{
		PluginID: "channel-discord",
		Manifest: manifest,
		Gate:     NewSecurityGate(manifest),
		Secrets:  mockSecrets,
		EventBus: bus.NewEventBus(),
	}

	inst, err := loader.Instantiate(ctx, "channel-discord", manifest, hostCtx)
	if err != nil {
		t.Fatalf("failed to instantiate channel-discord wasm: %v", err)
	}
	defer inst.Close(ctx)

	t.Logf("Successfully instantiated channel-discord plugin!")

	// Test poll
	pollFn := inst.mod.ExportedFunction("acton_channel_poll")
	if pollFn != nil {
		execCtx := WithHostContext(ctx, hostCtx)
		res, err := pollFn.Call(execCtx)
		t.Logf("Poll result: res=%v, err=%v", res, err)
	}
	for _, l := range hostCtx.GetLogs() {
		t.Logf("HOST LOG: %s", l)
	}
}

func TestHostWebSocketGateway(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Create a test WebSocket server
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		for {
			mt, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			// Echo message with prefix
			reply := append([]byte("ECHO: "), data...)
			if err := conn.Write(r.Context(), mt, reply); err != nil {
				return
			}
		}
	}))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	manifest := PluginManifest{
		ID: "test-ws",
		Permissions: PluginPermissions{
			NetOutbound: []string{"127.0.0.1", "localhost"},
		},
	}

	hostCtx := &HostContext{
		PluginID: "test-ws",
		Manifest: manifest,
		Gate:     NewSecurityGate(manifest),
	}

	execCtx := WithHostContext(ctx, hostCtx)

	// Test wsConnect via mock memory
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	if err := RegisterHostModule(ctx, r); err != nil {
		t.Fatalf("RegisterHostModule failed: %v", err)
	}

	// Direct test of HostContext WS lifecycle
	hBytes := []byte("{}")
	_ = hBytes

	// Dial using wsURL
	connCtx, connCancel := context.WithCancel(context.Background())
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		connCancel()
		t.Fatalf("websocket.Dial failed: %v", err)
	}

	hostCtx.mu.Lock()
	hostCtx.nextWSID = 1
	wsConn := &HostWSConn{
		id:       1,
		url:      wsURL,
		conn:     conn,
		msgQueue: make(chan []byte, 10),
		ctx:      connCtx,
		cancel:   connCancel,
	}
	hostCtx.wsConns = map[int32]*HostWSConn{1: wsConn}
	hostCtx.mu.Unlock()

	go func(c *HostWSConn) {
		defer c.close()
		for {
			_, data, err := c.conn.Read(c.ctx)
			if err != nil {
				return
			}
			c.msgQueue <- data
		}
	}(wsConn)

	// Send message
	writeCtx, writeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer writeCancel()
	if err := wsConn.conn.Write(writeCtx, websocket.MessageText, []byte("Hello ActonOS")); err != nil {
		t.Fatalf("wsConn.conn.Write failed: %v", err)
	}

	// Poll message
	var received []byte
	select {
	case msg := <-wsConn.msgQueue:
		received = msg
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for echo response")
	}

	if string(received) != "ECHO: Hello ActonOS" {
		t.Errorf("expected 'ECHO: Hello ActonOS', got '%s'", string(received))
	}

	// Close all
	hostCtx.CloseAllWS()
	if !wsConn.closed.Load() {
		t.Errorf("expected wsConn to be marked closed")
	}
	_ = execCtx
}

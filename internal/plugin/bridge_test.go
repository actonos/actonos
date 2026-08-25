package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
)

func TestWasmBridgesWithoutInstance(t *testing.T) {
	manifest := PluginManifest{
		ID:         "channel-zalo",
		Name:       "Zalo",
		Channels:   []PluginChannelDef{{Name: "zalo", DisplayName: "Zalo"}},
		Connectors: []PluginConnectorDef{{Name: "linear"}},
		Config: map[string]any{
			"poll_interval_seconds": float64(1),
			"auth_schema":           `{"type":"object"}`,
		},
		ConfigSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
	eb := bus.NewEventBus()
	defer eb.Close()

	ch := NewWasmChannelBridge("channel-zalo", manifest, nil, eb)
	if ch.Name() != "zalo" {
		t.Fatalf("channel name: %s", ch.Name())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatal(err)
	}
	ch.pollOnce(ctx)
	if err := ch.SendMessage(ctx, channels.OutboundMessage{Content: "hi"}); err == nil {
		t.Fatal("expected send error on inactive instance")
	}
	if err := ch.Stop(); err != nil {
		t.Fatal(err)
	}

	conn := NewWasmConnectorBridge("linear-plugin", manifest, nil)
	if conn.Name() != "linear" {
		t.Fatalf("connector name: %s", conn.Name())
	}
	if _, err := conn.HandleWebhook(ctx, []byte(`{}`)); err == nil {
		t.Fatal("expected webhook error on unloaded plugin")
	}
	if schema := string(conn.GetConfigSchema()); !strings.Contains(schema, "object") {
		t.Fatalf("schema: %s", schema)
	}

	tool := NewWasmToolBridge("p", PluginToolDef{Name: "echo", Description: "Echo", Parameters: json.RawMessage(`{"type":"object"}`)}, nil)
	if tool.Description() != "Echo" || tool.Category() != "wasm" {
		t.Fatalf("tool meta: %s %s", tool.Description(), tool.Category())
	}
	if string(tool.ParametersSchema()) == "" {
		t.Fatal("expected parameters schema")
	}
	if _, err := tool.Execute(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected execute error on inactive instance")
	}
}

func TestHostContextRedactsVaultSecretsFromLogs(t *testing.T) {
	h := &HostContext{}
	h.rememberSecret("super-secret-token")
	h.AppendLog("token=super-secret-token used")
	logs := h.GetLogs()
	if len(logs) != 1 {
		t.Fatalf("log count %d", len(logs))
	}
	if strings.Contains(logs[0], "super-secret-token") {
		t.Fatalf("secret leaked in plugin log: %s", logs[0])
	}
	if !strings.Contains(logs[0], "••••") {
		t.Fatalf("expected redaction marker, got %s", logs[0])
	}
}

func TestPluginRedirectCheckRejectsLoopback(t *testing.T) {
	gate := NewSecurityGate(PluginManifest{
		Permissions: PluginPermissions{NetOutbound: []string{"example.com"}},
	})
	check := pluginRedirectCheck(gate)
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	via, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err := check(req, []*http.Request{via}); err == nil {
		t.Fatal("expected redirect to loopback to be rejected")
	}
}

func TestPluginExecuteTimeoutMatchesDocs(t *testing.T) {
	if pluginToolTimeout != 15*time.Second {
		t.Fatalf("tool timeout %s, want 15s", pluginToolTimeout)
	}
}

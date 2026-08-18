package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func TestHTTPMCPClientHandlesJSONAndSSEResponses(t *testing.T) {
	for _, useSSE := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[useSSE], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request mcpRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				payload, _ := json.Marshal(mcpResponse{
					JSONRPC: "2.0",
					ID:      request.ID,
					Result:  json.RawMessage(`{"tools":[]}`),
				})
				if useSSE {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: "))
					_, _ = w.Write(payload)
					_, _ = w.Write([]byte("\n\n"))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(payload)
			}))
			defer server.Close()

			client, err := NewHTTPMCPClient(MCPServerConfig{ID: "remote", Transport: "http", URL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			tools, err := client.ListTools(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(tools) != 0 {
				t.Fatalf("expected empty tool list, got %+v", tools)
			}
		})
	}
}

type testMCPSecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *testMCPSecrets) SetSecret(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *testMCPSecrets) GetSecret(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func TestMCPHostHTTPConnectExecutePersistDisconnectAndRestore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		var method string
		_ = json.Unmarshal(raw["method"], &method)
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var id int64
		_ = json.Unmarshal(raw["id"], &id)
		result := json.RawMessage(`{}`)
		switch method {
		case "tools/list":
			result = json.RawMessage(`{"tools":[{"name":"echo","description":"Echo input","inputSchema":{"type":"object"}}]}`)
		case "tools/call":
			result = json.RawMessage(`{"content":[{"type":"text","text":"echoed"}]}`)
		}
		_ = json.NewEncoder(w).Encode(mcpResponse{JSONRPC: "2.0", ID: id, Result: result})
	}))
	defer server.Close()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	secrets := &testMCPSecrets{values: map[string]string{}}
	registry := NewToolRegistry(nil)
	host := NewMCPHostEngine(registry)
	if err := host.SetPersistence(db, secrets); err != nil {
		t.Fatal(err)
	}
	cfg := MCPServerConfig{
		ID: "remote_test", Transport: "http", URL: server.URL,
		Env: map[string]string{"TOKEN": "secret"},
	}
	if err := host.ConnectServer(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := host.ConnectServer(context.Background(), cfg); err == nil {
		t.Fatal("duplicate connection should fail")
	}
	tool, err := registry.Get("mcp_remote_test_echo")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name() != "mcp_remote_test_echo" || tool.Category() != "mcp" ||
		tool.Description() != "Echo input" || len(tool.ParametersSchema()) == 0 {
		t.Fatalf("unexpected MCP adapter metadata")
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"value":"hello"}`))
	if err != nil || result.Content != "echoed" {
		t.Fatalf("MCP tool execution failed: result=%+v err=%v", result, err)
	}
	if secrets.values["mcp.env.remote_test"] == "" {
		t.Fatal("MCP environment was not persisted in secret store")
	}
	var enabled int
	var persisted string
	if err := db.QueryRow("SELECT enabled, config_json FROM mcp_servers WHERE id = ?", cfg.ID).Scan(&enabled, &persisted); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 || stringContains(persisted, "secret") {
		t.Fatalf("unsafe MCP persistence: enabled=%d config=%s", enabled, persisted)
	}
	if err := host.DisconnectServer(cfg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Get("mcp_remote_test_echo"); err == nil {
		t.Fatal("MCP tool remained registered after disconnect")
	}
	if err := host.DisconnectServer(cfg.ID); err == nil {
		t.Fatal("missing MCP disconnect should fail")
	}

	if _, err := db.Exec("UPDATE mcp_servers SET enabled = 1 WHERE id = ?", cfg.ID); err != nil {
		t.Fatal(err)
	}
	restoredRegistry := NewToolRegistry(nil)
	restored := NewMCPHostEngine(restoredRegistry)
	if err := restored.SetPersistence(db, secrets); err != nil {
		t.Fatal(err)
	}
	restored.RestoreServers(context.Background())
	if _, err := restoredRegistry.Get("mcp_remote_test_echo"); err != nil {
		t.Fatalf("restored MCP tool missing: %v", err)
	}
	if err := restored.DisconnectServer(cfg.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMCPHostRejectsInvalidConfiguration(t *testing.T) {
	host := NewMCPHostEngine(NewToolRegistry(nil))
	for _, cfg := range []MCPServerConfig{
		{ID: "bad id", Transport: "http", URL: "https://example.com"},
		{ID: "valid", Transport: "websocket", URL: "https://example.com"},
	} {
		if err := host.ConnectServer(context.Background(), cfg); err == nil {
			t.Fatalf("invalid config accepted: %+v", cfg)
		}
	}
	if _, err := NewHTTPMCPClient(MCPServerConfig{}); err == nil {
		t.Fatal("missing HTTP URL should fail")
	}
}

func stringContains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

package tools

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
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

func TestMCPHostPublishesEventsOnConnectFailureAndRecovery(t *testing.T) {
	eventBus := bus.NewEventBus()
	errSub := eventBus.Subscribe(bus.EventMCPServerError)
	recoveredSub := eventBus.Subscribe(bus.EventMCPServerRecovered)

	registry := NewToolRegistry(nil)
	host := NewMCPHostEngine(registry)
	host.SetEventBus(eventBus)

	// A bogus stdio command should fail to start and publish an error event.
	badCfg := MCPServerConfig{ID: "broken_server", Transport: "stdio", Command: "this-binary-does-not-exist-anywhere"}
	if err := host.ConnectServer(context.Background(), badCfg); err == nil {
		t.Fatal("expected connect failure for nonexistent binary")
	}
	select {
	case ev := <-errSub:
		if ev.AgentID != badCfg.ID {
			t.Fatalf("expected error event for %s, got %s", badCfg.ID, ev.AgentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected EventMCPServerError to be published on connect failure")
	}

	statuses, err := listServersWithPersistence(t, host, badCfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if statuses.LastError == "" {
		t.Fatal("expected LastError to be recorded in ListServers after failed connect")
	}

	// A subsequent successful connect for the same ID should clear the error
	// and publish a recovered event.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		var method string
		_ = json.Unmarshal(raw["method"], &method)
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var id int64
		_ = json.Unmarshal(raw["id"], &id)
		result := json.RawMessage(`{"tools":[]}`)
		_ = json.NewEncoder(w).Encode(mcpResponse{JSONRPC: "2.0", ID: id, Result: result})
	}))
	defer server.Close()

	// Reconnect using the same server ID as an HTTP transport this time
	// (ConnectServer only rejects duplicates while still connected).
	goodCfg := MCPServerConfig{ID: badCfg.ID, Transport: "http", URL: server.URL}
	if err := host.ConnectServer(context.Background(), goodCfg); err != nil {
		t.Fatalf("expected reconnect to succeed: %v", err)
	}
	select {
	case ev := <-recoveredSub:
		if ev.AgentID != badCfg.ID {
			t.Fatalf("expected recovered event for %s, got %s", badCfg.ID, ev.AgentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected EventMCPServerRecovered to be published after successful reconnect")
	}
}

func listServersWithPersistence(t *testing.T, host *MCPHostEngine, serverID string) (MCPServerStatus, error) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return MCPServerStatus{}, err
	}
	defer db.Close()
	if err := host.SetPersistence(db, nil); err != nil {
		return MCPServerStatus{}, err
	}
	if _, err := db.Exec("INSERT INTO mcp_servers (id, config_json, enabled, updated_at) VALUES (?, '{}', 1, ?)", serverID, time.Now().UTC()); err != nil {
		return MCPServerStatus{}, err
	}
	statuses, err := host.ListServers(context.Background())
	if err != nil {
		return MCPServerStatus{}, err
	}
	for _, s := range statuses {
		if s.ID == serverID {
			return s, nil
		}
	}
	return MCPServerStatus{}, fmt.Errorf("server %s not found in ListServers", serverID)
}

func TestMCPClientInvokesOnCloseForUnexpectedDisconnectButNotForDeliberateClose(t *testing.T) {
	// Unexpected disconnect: the underlying stdout pipe closes on its own
	// (simulating the MCP server process crashing), readLoop should invoke
	// onClose exactly once.
	t.Run("unexpected", func(t *testing.T) {
		pr, pw := io.Pipe()
		client := &MCPClient{
			stdout:  bufio.NewReader(pr),
			pending: make(map[int64]chan mcpResponse),
		}
		called := make(chan error, 1)
		client.SetOnClose(func(err error) { called <- err })
		go client.readLoop()

		_ = pw.Close() // simulate the process's stdout closing unexpectedly

		select {
		case err := <-called:
			if err == nil {
				t.Fatal("expected a non-nil error describing the unexpected disconnect")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("expected onClose to be invoked after unexpected stdout close")
		}
	})

	// Deliberate close: calling Close() should suppress the onClose callback
	// entirely, since the disconnect was operator-initiated.
	t.Run("deliberate", func(t *testing.T) {
		pr, pw := io.Pipe()
		defer pw.Close()
		client := &MCPClient{
			stdout:  bufio.NewReader(pr),
			stdin:   pw,
			pending: make(map[int64]chan mcpResponse),
		}
		called := make(chan error, 1)
		client.SetOnClose(func(err error) { called <- err })
		go client.readLoop()

		if err := client.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		_ = pr.Close() // now let readLoop observe the close so it can exit

		select {
		case err := <-called:
			t.Fatalf("onClose should not fire after a deliberate Close(), got: %v", err)
		case <-time.After(300 * time.Millisecond):
			// expected: no callback fired
		}
	})
}

func TestMCPHostRemovesDeadClientAndPublishesErrorOnUnexpectedDisconnect(t *testing.T) {
	eventBus := bus.NewEventBus()
	errSub := eventBus.Subscribe(bus.EventMCPServerError)

	registry := NewToolRegistry(nil)
	host := NewMCPHostEngine(registry)
	host.SetEventBus(eventBus)

	pr, pw := io.Pipe()
	client := &MCPClient{
		config:  MCPServerConfig{ID: "dying_server"},
		stdout:  bufio.NewReader(pr),
		pending: make(map[int64]chan mcpResponse),
	}
	host.mu.Lock()
	host.clients["dying_server"] = client
	serverID := "dying_server"
	client.SetOnClose(func(closeErr error) {
		host.mu.Lock()
		if host.clients[serverID] == client {
			delete(host.clients, serverID)
		}
		host.recordMCPErrorLocked(serverID, serverID, closeErr)
		host.mu.Unlock()
	})
	host.mu.Unlock()
	go client.readLoop()

	_ = pw.Close() // simulate the MCP server process dying

	select {
	case ev := <-errSub:
		if ev.AgentID != serverID {
			t.Fatalf("expected error event for %s, got %s", serverID, ev.AgentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected EventMCPServerError after unexpected stdio disconnect")
	}

	// Give the async onClose handler a moment to remove the dead entry.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		host.mu.RLock()
		_, stillPresent := host.clients[serverID]
		host.mu.RUnlock()
		if !stillPresent {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected dead client to be removed from host.clients after unexpected disconnect")
}

func TestMCPUnexpectedCloseSchedulesReconnect(t *testing.T) {
	host := NewMCPHostEngine(NewToolRegistry(nil))
	client := &MCPClient{config: MCPServerConfig{ID: "rejoin", Transport: "http", URL: "http://127.0.0.1:1"}, pending: make(map[int64]chan mcpResponse)}
	host.mu.Lock()
	host.clients["rejoin"] = client
	host.configs["rejoin"] = client.config
	host.mu.Unlock()
	host.onUnexpectedClose("rejoin", client, errors.New("stdout closed"))
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		host.mu.RLock()
		_, hasErr := host.lastErrors["rejoin"]
		host.mu.RUnlock()
		if hasErr {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected reconnect attempt to record an MCP error after unexpected close")
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

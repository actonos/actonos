package tools

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

var (
	ErrMCPConnectionFailed = errors.New("mcp server connection failed")
	ErrMCPToolCallFailed   = errors.New("mcp tool call failed")
)

// MCPServerConfig defines configuration for connecting to an MCP server.
type MCPServerConfig struct {
	ID        string            `json:"id"`
	Transport string            `json:"transport"` // "stdio" or "sse"
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
}

// MCPClient handles JSON-RPC 2.0 communication with an MCP server process or endpoint.
type MCPClient struct {
	config  MCPServerConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	reqID   atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan mcpResponse
	closed  bool
	// deliberate is set by Close() to distinguish an operator-initiated
	// disconnect from an unexpected process crash / stdout error, so onClose
	// only fires (and only publishes an error event) for the latter.
	deliberate bool
	// onClose, when set, is invoked exactly once from readLoop when the
	// underlying transport ends unexpectedly (e.g. the MCP server process
	// died). It lets MCPHostEngine notice a mid-session disconnect that
	// would otherwise be invisible until the whole tool call failed.
	onClose func(err error)
	http    *http.Client
}

// SetOnClose registers a callback invoked once if the client's transport
// closes unexpectedly (not via an explicit Close()).
func (c *MCPClient) SetOnClose(fn func(err error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onClose = fn
}

// NewHTTPMCPClient creates a remote JSON-RPC MCP client over streamable HTTP.
func NewHTTPMCPClient(cfg MCPServerConfig) (*MCPClient, error) {
	if cfg.URL == "" {
		return nil, errors.New("mcp URL is required for HTTP transport")
	}
	return &MCPClient{
		config:  cfg,
		http:    &http.Client{Timeout: 30 * time.Second},
		pending: make(map[int64]chan mcpResponse),
	}, nil
}

type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewStdioMCPClient starts an MCP server subprocess over stdio.
func NewStdioMCPClient(cfg MCPServerConfig) (*MCPClient, error) {
	if cfg.Command == "" {
		return nil, errors.New("mcp command is required for stdio transport")
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "linux" && os.Getenv("RUNTIME_MODE") != "docker" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			return nil, fmt.Errorf("%w: bubblewrap is required for MCP stdio", ErrMCPConnectionFailed)
		}
		args := []string{
			"--ro-bind", "/usr", "/usr",
			"--ro-bind", "/bin", "/bin",
			"--ro-bind", "/lib", "/lib",
			"--proc", "/proc",
			"--dev", "/dev",
			"--tmpfs", "/tmp",
			"--unshare-all",
			"--new-session",
			"--die-with-parent",
			"--cap-drop", "ALL",
			"--",
			cfg.Command,
		}
		args = append(args, cfg.Args...)
		cmd = exec.Command("bwrap", args...)
	} else {
		if os.Getenv("RUNTIME_MODE") != "docker" && os.Getenv("ACTONOS_ALLOW_UNSANDBOXED_MCP") != "1" {
			return nil, fmt.Errorf("%w: MCP stdio requires Docker, bubblewrap, or explicit development override", ErrMCPConnectionFailed)
		}
		cmd = exec.Command(cfg.Command, cfg.Args...)
	}
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting mcp process: %w", err)
	}

	client := &MCPClient{
		config:  cfg,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[int64]chan mcpResponse),
	}

	go client.readLoop()
	go func() {
		scanner := bufio.NewScanner(io.LimitReader(stderr, 1<<20))
		for scanner.Scan() {
			slog.Warn("mcp server stderr", "server", cfg.ID, "message", scanner.Text())
		}
	}()

	return client, nil
}

func (c *MCPClient) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.mu.Lock()
			wasDeliberate := c.deliberate
			c.closed = true
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = make(map[int64]chan mcpResponse)
			onClose := c.onClose
			c.onClose = nil // fire at most once
			c.mu.Unlock()
			if !wasDeliberate && onClose != nil {
				if err == io.EOF {
					onClose(errors.New("mcp server process ended unexpectedly (stdout closed)"))
				} else {
					onClose(fmt.Errorf("mcp server stdout read error: %w", err))
				}
			}
			return
		}
		if len(line) > 4<<20 {
			slog.Warn("mcp response exceeded size limit", "server", c.config.ID, "bytes", len(line))
			_ = c.Close()
			return
		}

		var resp mcpResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
			ch <- resp
			close(ch)
		}
		c.mu.Unlock()
	}
}

func (c *MCPClient) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.http != nil {
		id := c.reqID.Add(1)
		payload, err := json.Marshal(mcpRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
		if err != nil {
			return nil, fmt.Errorf("marshalling HTTP MCP request: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("creating HTTP MCP request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("sending HTTP MCP request: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, fmt.Errorf("reading HTTP MCP response: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP MCP status %d: %s", resp.StatusCode, string(body))
		}
		if bytes.HasPrefix(bytes.TrimSpace(body), []byte("data:")) {
			for _, line := range bytes.Split(body, []byte{'\n'}) {
				line = bytes.TrimSpace(line)
				if bytes.HasPrefix(line, []byte("data:")) {
					body = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
					break
				}
			}
		}
		var response mcpResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("decoding HTTP MCP response: %w", err)
		}
		if response.Error != nil {
			return nil, fmt.Errorf("mcp error (%d): %s", response.Error.Code, response.Error.Message)
		}
		return response.Result, nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("mcp client connection closed")
	}

	id := c.reqID.Add(1)
	respChan := make(chan mcpResponse, 1)
	c.pending[id] = respChan

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp, ok := <-respChan:
		if !ok {
			return nil, errors.New("mcp response channel closed unexpectedly")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// Initialize performs the standard MCP initialization handshake.
func (c *MCPClient) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "ActonOS",
			"version": "1.0.0",
		},
	}

	if _, err := c.sendRequest(ctx, "initialize", params); err != nil {
		return err
	}
	return c.sendNotification("notifications/initialized", map[string]any{})
}

func (c *MCPClient) sendNotification(method string, params any) error {
	if c.http != nil {
		// Streamable HTTP servers do not require a response for notifications.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		return resp.Body.Close()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("mcp client connection closed")
	}
	data, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return fmt.Errorf("marshalling MCP notification: %w", err)
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("writing MCP notification: %w", err)
	}
	return nil
}

// ListTools queries the MCP server for available tools.
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPToolInfo, error) {
	resultBytes, err := c.sendRequest(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tools []MCPToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(resultBytes, &resp); err != nil {
		return nil, fmt.Errorf("parsing tools/list result: %w", err)
	}

	return resp.Tools, nil
}

// CallTool executes a specific tool on the MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (*ToolResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}

	resultBytes, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMCPToolCallFailed, err)
	}

	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}

	if err := json.Unmarshal(resultBytes, &resp); err != nil {
		return nil, fmt.Errorf("parsing tools/call result: %w", err)
	}

	var textContent string
	for _, c := range resp.Content {
		if c.Type == "text" {
			textContent += c.Text
		}
	}

	if resp.IsError {
		return nil, fmt.Errorf("%w: %s", ErrMCPToolCallFailed, textContent)
	}

	return &ToolResult{
		Content: textContent,
	}, nil
}

// Close closes the MCP client process and pipes.
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deliberate = true
	c.closed = true
	c.onClose = nil
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

// MCPToolInfo describes a tool returned by `tools/list`.
type MCPToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPToolAdapter wraps an MCP server tool into the standard Tool interface.
type MCPToolAdapter struct {
	serverID string
	client   *MCPClient
	info     MCPToolInfo
}

func (a *MCPToolAdapter) Name() string {
	return fmt.Sprintf("mcp_%s_%s", a.serverID, a.info.Name)
}

func (a *MCPToolAdapter) Description() string {
	return a.info.Description
}

func (a *MCPToolAdapter) Category() string {
	return "mcp"
}

func (a *MCPToolAdapter) ParametersSchema() json.RawMessage {
	return a.info.InputSchema
}

func (a *MCPToolAdapter) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	return a.client.CallTool(ctx, a.info.Name, inputJSON)
}

// MCPHostEngine manages multiple MCP server connections.
type MCPHostEngine struct {
	mu         sync.RWMutex
	registry   *ToolRegistry
	clients    map[string]*MCPClient
	db         *sql.DB
	secrets    MCPSecretStore
	eventBus   *bus.EventBus
	lastErrors map[string]mcpRuntimeError // serverID -> most recent error, cleared on successful connect
}

// mcpRuntimeError records the most recent connection/runtime failure for an
// MCP server so it can be surfaced via ListServers even after the failure
// itself has scrolled out of the log.
type mcpRuntimeError struct {
	Err string
	At  time.Time
}

// SetEventBus wires the shared EventBus so MCP connection failures and
// unexpected disconnects can be published as EventMCPServerError /
// EventMCPServerRecovered for the NotificationManager to pick up.
func (h *MCPHostEngine) SetEventBus(eventBus *bus.EventBus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.eventBus = eventBus
}

// MCPServerStatus is the non-secret administrative view of an MCP server.
type MCPServerStatus struct {
	ID          string    `json:"id"`
	Transport   string    `json:"transport"`
	Command     string    `json:"command,omitempty"`
	Args        []string  `json:"args,omitempty"`
	URL         string    `json:"url,omitempty"`
	Enabled     bool      `json:"enabled"`
	Connected   bool      `json:"connected"`
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MCPSecretStore stores MCP environment secrets encrypted at rest.
type MCPSecretStore interface {
	SetSecret(ctx context.Context, keyName, secretValue string) error
	GetSecret(ctx context.Context, keyName string) (string, error)
}

// NewMCPHostEngine creates a new MCP host engine.
func NewMCPHostEngine(registry *ToolRegistry) *MCPHostEngine {
	return &MCPHostEngine{
		registry:   registry,
		clients:    make(map[string]*MCPClient),
		lastErrors: make(map[string]mcpRuntimeError),
	}
}

// SetPersistence configures durable MCP server definitions and encrypted environments.
func (h *MCPHostEngine) SetPersistence(db *sql.DB, secrets MCPSecretStore) error {
	h.db = db
	h.secrets = secrets
	if db == nil {
		return nil
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS mcp_servers (
			id TEXT PRIMARY KEY,
			config_json TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("creating mcp server registry: %w", err)
	}
	return nil
}

// ListServers returns persisted MCP definitions without encrypted environment values.
func (h *MCPHostEngine) ListServers(ctx context.Context) ([]MCPServerStatus, error) {
	if h.db == nil {
		return []MCPServerStatus{}, nil
	}
	rows, err := h.db.QueryContext(ctx, "SELECT id, config_json, enabled, updated_at FROM mcp_servers ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("listing mcp servers: %w", err)
	}
	defer rows.Close()
	h.mu.RLock()
	defer h.mu.RUnlock()
	items := make([]MCPServerStatus, 0)
	for rows.Next() {
		var id, raw string
		var enabled int
		var updated time.Time
		if err := rows.Scan(&id, &raw, &enabled, &updated); err != nil {
			return nil, fmt.Errorf("scanning mcp server: %w", err)
		}
		var cfg MCPServerConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			continue
		}
		_, connected := h.clients[id]
		lastErr, hasLastErr := h.lastErrors[id]
		status := MCPServerStatus{
			ID: id, Transport: cfg.Transport, Command: cfg.Command, Args: cfg.Args,
			URL: cfg.URL, Enabled: enabled == 1, Connected: connected, UpdatedAt: updated,
		}
		if hasLastErr {
			status.LastError = lastErr.Err
			status.LastErrorAt = lastErr.At
		}
		items = append(items, status)
	}
	return items, rows.Err()
}

// SetServerEnabled connects or disconnects a persisted MCP definition.
func (h *MCPHostEngine) SetServerEnabled(ctx context.Context, serverID string, enabled bool) error {
	if enabled {
		h.mu.RLock()
		_, connected := h.clients[serverID]
		h.mu.RUnlock()
		if connected {
			if h.db != nil {
				_, _ = h.db.ExecContext(ctx, "UPDATE mcp_servers SET enabled = 1, updated_at = ? WHERE id = ?", time.Now().UTC(), serverID)
			}
			return nil
		}
		if h.db == nil {
			return errors.New("mcp persistence is not configured")
		}
		var raw string
		if err := h.db.QueryRowContext(ctx, "SELECT config_json FROM mcp_servers WHERE id = ?", serverID).Scan(&raw); err != nil {
			return fmt.Errorf("loading mcp server: %w", err)
		}
		var cfg MCPServerConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("decoding mcp server: %w", err)
		}
		if h.secrets != nil {
			if envJSON, err := h.secrets.GetSecret(ctx, "mcp.env."+serverID); err == nil {
				_ = json.Unmarshal([]byte(envJSON), &cfg.Env)
			}
		}
		return h.ConnectServer(ctx, cfg)
	}
	h.mu.RLock()
	_, connected := h.clients[serverID]
	h.mu.RUnlock()
	if connected {
		return h.DisconnectServer(serverID)
	}
	if h.db == nil {
		return errors.New("mcp persistence is not configured")
	}
	result, err := h.db.ExecContext(ctx, "UPDATE mcp_servers SET enabled = 0, updated_at = ? WHERE id = ?", time.Now().UTC(), serverID)
	if err != nil {
		return fmt.Errorf("disabling mcp server: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("mcp server %s not found", serverID)
	}
	return nil
}

// RestoreServers reconnects enabled MCP servers after daemon restart.
func (h *MCPHostEngine) RestoreServers(ctx context.Context) {
	if h.db == nil {
		return
	}
	rows, err := h.db.QueryContext(ctx, "SELECT id, config_json FROM mcp_servers WHERE enabled = 1")
	if err != nil {
		slog.Warn("failed to restore mcp servers", "error", err)
		return
	}
	var configs []MCPServerConfig
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			continue
		}
		var cfg MCPServerConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			continue
		}
		if h.secrets != nil {
			if envJSON, secretErr := h.secrets.GetSecret(ctx, "mcp.env."+id); secretErr == nil {
				_ = json.Unmarshal([]byte(envJSON), &cfg.Env)
			}
		}
		configs = append(configs, cfg)
	}
	_ = rows.Close()
	for _, cfg := range configs {
		if err := h.ConnectServer(ctx, cfg); err != nil {
			slog.Warn("mcp server restore failed", "server", cfg.ID, "error", err)
			h.recordMCPError(cfg.ID, cfg.Command, err)
		}
	}
}

// recordMCPError stores the latest failure for a server and publishes
// EventMCPServerError so the NotificationManager can surface it on the web
// UI, instead of the failure only ever reaching the server log. Must be
// called without holding h.mu.
func (h *MCPHostEngine) recordMCPError(serverID, name string, err error) {
	h.mu.Lock()
	h.recordMCPErrorLocked(serverID, name, err)
	h.mu.Unlock()
}

// recordMCPErrorLocked is the lock-already-held variant, used by callers
// (like ConnectServer) that already hold h.mu.
func (h *MCPHostEngine) recordMCPErrorLocked(serverID, name string, err error) {
	h.lastErrors[serverID] = mcpRuntimeError{Err: err.Error(), At: time.Now().UTC()}
	eventBus := h.eventBus
	if name == "" {
		name = serverID
	}
	if eventBus != nil {
		eventBus.Publish(bus.NewEvent(bus.EventMCPServerError, serverID, map[string]any{
			"server_id": serverID,
			"name":      name,
			"error":     err.Error(),
		}))
	}
}

// recordMCPRecovered clears any stored failure for a server and, if it had
// previously failed, publishes EventMCPServerRecovered. Must be called
// without holding h.mu.
func (h *MCPHostEngine) recordMCPRecovered(serverID, name string) {
	h.mu.Lock()
	h.recordMCPRecoveredLocked(serverID, name)
	h.mu.Unlock()
}

// recordMCPRecoveredLocked is the lock-already-held variant, used by callers
// (like ConnectServer) that already hold h.mu.
func (h *MCPHostEngine) recordMCPRecoveredLocked(serverID, name string) {
	_, hadError := h.lastErrors[serverID]
	delete(h.lastErrors, serverID)
	eventBus := h.eventBus
	if name == "" {
		name = serverID
	}
	if hadError && eventBus != nil {
		eventBus.Publish(bus.NewEvent(bus.EventMCPServerRecovered, serverID, map[string]any{
			"server_id": serverID,
			"name":      name,
		}))
	}
}

// ConnectServer starts an MCP server, performs handshake, and registers its tools.
func (h *MCPHostEngine) ConnectServer(ctx context.Context, cfg MCPServerConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.clients[cfg.ID]; exists {
		return fmt.Errorf("mcp server %s already connected", cfg.ID)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(cfg.ID) {
		return errors.New("mcp server id must contain only letters, digits, underscore, or hyphen")
	}
	if cfg.Transport != "" && cfg.Transport != "stdio" && cfg.Transport != "http" && cfg.Transport != "sse" {
		return fmt.Errorf("unsupported mcp transport %q", cfg.Transport)
	}

	var client *MCPClient
	var err error
	if cfg.Transport == "http" || cfg.Transport == "sse" {
		client, err = NewHTTPMCPClient(cfg)
	} else {
		client, err = NewStdioMCPClient(cfg)
	}
	if err != nil {
		wrapped := fmt.Errorf("connecting to mcp server %s: %w", cfg.ID, err)
		h.recordMCPErrorLocked(cfg.ID, cfg.ID, wrapped)
		return wrapped
	}

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Initialize(initCtx); err != nil {
		_ = client.Close()
		wrapped := fmt.Errorf("mcp handshake failed for %s: %w", cfg.ID, err)
		h.recordMCPErrorLocked(cfg.ID, cfg.ID, wrapped)
		return wrapped
	}

	tools, err := client.ListTools(initCtx)
	if err != nil {
		_ = client.Close()
		wrapped := fmt.Errorf("listing mcp tools for %s: %w", cfg.ID, err)
		h.recordMCPErrorLocked(cfg.ID, cfg.ID, wrapped)
		return wrapped
	}

	var registered []string
	for _, toolInfo := range tools {
		adapter := &MCPToolAdapter{
			serverID: cfg.ID,
			client:   client,
			info:     toolInfo,
		}
		if err := h.registry.Register(adapter); err != nil {
			for _, name := range registered {
				h.registry.Unregister(name)
			}
			_ = client.Close()
			wrapped := fmt.Errorf("registering mcp tool %s: %w", adapter.Name(), err)
			h.recordMCPErrorLocked(cfg.ID, cfg.ID, wrapped)
			return wrapped
		}
		registered = append(registered, adapter.Name())
		slog.Info("registered mcp tool", "server", cfg.ID, "tool", adapter.Name())
	}
	h.clients[cfg.ID] = client
	h.recordMCPRecoveredLocked(cfg.ID, cfg.ID)
	serverID := cfg.ID
	client.SetOnClose(func(closeErr error) {
		h.mu.Lock()
		if h.clients[serverID] == client {
			delete(h.clients, serverID)
		}
		h.recordMCPErrorLocked(serverID, serverID, closeErr)
		h.mu.Unlock()
		slog.Warn("mcp server disconnected unexpectedly", "server", serverID, "error", closeErr)
	})
	if h.db != nil {
		persisted := cfg
		persisted.Env = nil
		raw, _ := json.Marshal(persisted)
		_, _ = h.db.ExecContext(ctx, `
			INSERT INTO mcp_servers (id, config_json, enabled, updated_at)
			VALUES (?, ?, 1, ?)
			ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json,
				enabled = 1, updated_at = excluded.updated_at
		`, cfg.ID, string(raw), time.Now().UTC())
		if h.secrets != nil && len(cfg.Env) > 0 {
			envJSON, _ := json.Marshal(cfg.Env)
			_ = h.secrets.SetSecret(ctx, "mcp.env."+cfg.ID, string(envJSON))
		}
	}

	return nil
}

// DisconnectServer stops and unregisters an MCP server.
func (h *MCPHostEngine) DisconnectServer(serverID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	client, exists := h.clients[serverID]
	if !exists {
		return fmt.Errorf("mcp server %s not found", serverID)
	}

	_ = client.Close()
	delete(h.clients, serverID)
	delete(h.lastErrors, serverID)
	if h.db != nil {
		_, _ = h.db.Exec("UPDATE mcp_servers SET enabled = 0, updated_at = ? WHERE id = ?", time.Now().UTC(), serverID)
	}

	// Unregister associated tools
	prefix := fmt.Sprintf("mcp_%s_", serverID)
	for _, t := range h.registry.List() {
		if t.Category == "mcp" && len(t.Name) > len(prefix) && t.Name[:len(prefix)] == prefix {
			h.registry.Unregister(t.Name)
		}
	}

	return nil
}

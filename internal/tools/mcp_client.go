package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
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
	config    MCPServerConfig
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	reqID     atomic.Int64
	mu        sync.Mutex
	pending   map[int64]chan mcpResponse
	closed    bool
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

	cmd := exec.Command(cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
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

	return client, nil
}

func (c *MCPClient) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.mu.Lock()
			c.closed = true
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = make(map[int64]chan mcpResponse)
			c.mu.Unlock()
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

	_, err := c.sendRequest(ctx, "initialize", params)
	return err
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
		return &ToolResult{
			Error:   textContent,
			Content: textContent,
		}, nil
	}

	return &ToolResult{
		Content: textContent,
	}, nil
}

// Close closes the MCP client process and pipes.
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
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
	mu       sync.RWMutex
	registry *ToolRegistry
	clients  map[string]*MCPClient
}

// NewMCPHostEngine creates a new MCP host engine.
func NewMCPHostEngine(registry *ToolRegistry) *MCPHostEngine {
	return &MCPHostEngine{
		registry: registry,
		clients:  make(map[string]*MCPClient),
	}
}

// ConnectServer starts an MCP server, performs handshake, and registers its tools.
func (h *MCPHostEngine) ConnectServer(ctx context.Context, cfg MCPServerConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.clients[cfg.ID]; exists {
		return fmt.Errorf("mcp server %s already connected", cfg.ID)
	}

	client, err := NewStdioMCPClient(cfg)
	if err != nil {
		return fmt.Errorf("connecting to mcp server %s: %w", cfg.ID, err)
	}

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Initialize(initCtx); err != nil {
		_ = client.Close()
		return fmt.Errorf("mcp handshake failed for %s: %w", cfg.ID, err)
	}

	tools, err := client.ListTools(initCtx)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("listing mcp tools for %s: %w", cfg.ID, err)
	}

	h.clients[cfg.ID] = client

	for _, toolInfo := range tools {
		adapter := &MCPToolAdapter{
			serverID: cfg.ID,
			client:   client,
			info:     toolInfo,
		}
		_ = h.registry.Register(adapter)
		slog.Info("registered mcp tool", "server", cfg.ID, "tool", adapter.Name())
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

	// Unregister associated tools
	prefix := fmt.Sprintf("mcp_%s_", serverID)
	for _, t := range h.registry.List() {
		if t.Category == "mcp" && len(t.Name) > len(prefix) && t.Name[:len(prefix)] == prefix {
			h.registry.Unregister(t.Name)
		}
	}

	return nil
}

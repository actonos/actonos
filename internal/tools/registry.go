package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
)

var (
	ErrToolNotFound      = errors.New("tool not found in registry")
	ErrToolAlreadyExists = errors.New("tool already registered")
	ErrExecutionFailed   = errors.New("tool execution failed")
	ErrToolUnauthorized  = errors.New("tool is not authorized for agent")
)

type executionContextKey string

const (
	traceIDContextKey  executionContextKey = "trace_id"
	approvalContextKey executionContextKey = "approval_id"
)

// AgentToolPolicy is the execution-boundary projection of an agent manifest.
type AgentToolPolicy struct {
	AuthorizedTools   []string
	ApprovalThreshold string
	AllowedPaths      []string
}

// PolicyResolver resolves an agent's current execution policy.
type PolicyResolver func(ctx context.Context, agentID string) (AgentToolPolicy, error)

// ToolResult represents the output from executing a tool.
type ToolResult struct {
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// Tool represents a callable tool within the ActonOS system.
type Tool interface {
	Name() string
	Description() string
	Category() string // "native", "mcp", "wasm", "skill"
	ParametersSchema() json.RawMessage
	Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error)
}

// ToolInfo is a serializable representation of a registered tool for APIs and dashboards.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Schema      json.RawMessage `json:"schema"`
}

// ToolAuditLogger defines interface for writing tool audit logs.
type ToolAuditLogger interface {
	LogAudit(traceID, agentID, toolName, riskLevel, status, errorMsg string, durationMS int64)
}

// ToolRegistry manages all registered tools (Native, MCP, WASM, Skills).
type ToolRegistry struct {
	mu             sync.RWMutex
	tools          map[string]Tool
	bus            *bus.EventBus
	auditLogger    ToolAuditLogger
	policyResolver PolicyResolver
	approvals      *ApprovalManager
}

// SetPolicyResolver enables authoritative execution-time permission checks.
func (r *ToolRegistry) SetPolicyResolver(resolver PolicyResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policyResolver = resolver
}

// SetApprovalManager enables durable human approval gates.
func (r *ToolRegistry) SetApprovalManager(manager *ApprovalManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvals = manager
}

// WithTraceID propagates an end-to-end trace identifier into tool execution.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

// WithApprovalID binds a previously approved exact action to execution.
func WithApprovalID(ctx context.Context, approvalID string) context.Context {
	return context.WithValue(ctx, approvalContextKey, approvalID)
}

// TraceIDFromContext retrieves the current trace identifier.
func TraceIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(traceIDContextKey).(string)
	return value
}

// NewToolRegistry creates a new ToolRegistry instance.
func NewToolRegistry(eventBus *bus.EventBus) *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
		bus:   eventBus,
	}
}

// SetAuditLogger connects the system audit logger.
func (r *ToolRegistry) SetAuditLogger(al ToolAuditLogger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditLogger = al
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: %s", ErrToolAlreadyExists, name)
	}

	r.tools[name] = tool
	return nil
}

// Unregister removes a tool from the registry.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get retrieves a tool by name.
func (r *ToolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return tool, nil
}

// List returns a list of all registered tools.
func (r *ToolRegistry) List() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Category:    t.Category(),
			Schema:      t.ParametersSchema(),
		})
	}
	return out
}

// ListByCategory filters registered tools by category ("native", "mcp", "wasm", "skill").
func (r *ToolRegistry) ListByCategory(category string) []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []ToolInfo
	for _, t := range r.tools {
		if t.Category() == category {
			out = append(out, ToolInfo{
				Name:        t.Name(),
				Description: t.Description(),
				Category:    t.Category(),
				Schema:      t.ParametersSchema(),
			})
		}
	}
	return out
}

// ToLLMToolDefinitions converts authorized tools into LLM-compatible tool definitions.
func (r *ToolRegistry) ToLLMToolDefinitions(authorizedTools []string) []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	allowAll := false
	authMap := make(map[string]bool)
	for _, a := range authorizedTools {
		if a == "*" {
			allowAll = true
			break
		}
		authMap[a] = true
	}

	var defs []llm.ToolDefinition
	for name, t := range r.tools {
		if allowAll || authMap[name] {
			defs = append(defs, llm.ToolDefinition{
				Type: "function",
				Function: llm.FunctionDefinition{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.ParametersSchema(),
				},
			})
		}
	}
	return defs
}

// NormalizeToolInput converts raw string, double-JSON-encoded, or malformed inputs into valid JSON object bytes.
func NormalizeToolInput(inputJSON json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(inputJSON))
	if len(trimmed) == 0 || trimmed == `""` || trimmed == "{}" {
		return json.RawMessage(`{}`)
	}

	// 1. If wrapped in outer quotes (JSON string literal like "\"{\\\"url\\\": ...}\"" or "\"https://example.com\"")
	if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		var unescaped string
		if err := json.Unmarshal([]byte(trimmed), &unescaped); err == nil {
			unescapedTrimmed := strings.TrimSpace(unescaped)
			// If unescaped string is a JSON object or array
			if (strings.HasPrefix(unescapedTrimmed, "{") && strings.HasSuffix(unescapedTrimmed, "}")) ||
				(strings.HasPrefix(unescapedTrimmed, "[") && strings.HasSuffix(unescapedTrimmed, "]")) {
				return json.RawMessage(unescapedTrimmed)
			}
			// If it's a URL string like "https://..." or "http://..."
			if strings.HasPrefix(unescapedTrimmed, "http://") || strings.HasPrefix(unescapedTrimmed, "https://") {
				obj, _ := json.Marshal(map[string]any{"url": unescapedTrimmed})
				return json.RawMessage(obj)
			}
			// Wrap raw string with universal property names
			obj, _ := json.Marshal(map[string]any{
				"url":     unescapedTrimmed,
				"path":    unescapedTrimmed,
				"command": unescapedTrimmed,
				"input":   unescapedTrimmed,
			})
			return json.RawMessage(obj)
		}
	}

	// 2. If it's already a JSON object { ... }
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return inputJSON
	}

	// 3. Fallback: Wrap raw string
	obj, _ := json.Marshal(map[string]any{
		"url":     trimmed,
		"path":    trimmed,
		"command": trimmed,
		"input":   trimmed,
	})
	return json.RawMessage(obj)
}

// Execute executes a tool by name with input JSON parameters.
func (r *ToolRegistry) Execute(ctx context.Context, agentID, name string, inputJSON json.RawMessage) (*ToolResult, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	// Normalize input to handle string-encoded or malformed LLM tool arguments
	normalizedInput := NormalizeToolInput(inputJSON)
	if name == "native_exec" {
		if err := validateCommandToolInput(normalizedInput); err != nil {
			return nil, fmt.Errorf("verifying command: %w", err)
		}
	}

	r.mu.RLock()
	resolver := r.policyResolver
	approvalManager := r.approvals
	r.mu.RUnlock()

	var policy AgentToolPolicy
	if resolver != nil {
		policy, err = resolver(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("resolving tool policy for agent %s: %w", agentID, err)
		}
		if !toolAllowed(policy.AuthorizedTools, name) {
			return nil, fmt.Errorf("%w: agent=%s tool=%s", ErrToolUnauthorized, agentID, name)
		}
		if strings.HasPrefix(name, "native_file_") {
			if err := validateAllowedPath(policy.AllowedPaths, normalizedInput); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrToolUnauthorized, err)
			}
		}
	}

	if r.bus != nil {
		r.bus.Publish(bus.NewEvent(bus.EventToolExecutionStarted, agentID, map[string]any{
			"tool_name": name,
			"input":     string(normalizedInput),
		}))
	}

	riskLevel := ToolRiskLevel(name)
	traceID := TraceIDFromContext(ctx)
	if traceID == "" {
		traceID = newTraceID()
		ctx = WithTraceID(ctx, traceID)
	}

	if resolver != nil && approvalRequired(policy.ApprovalThreshold, riskLevel) {
		approvalID, _ := ctx.Value(approvalContextKey).(string)
		if approvalID == "" {
			request, requestErr := approvalManager.Request(ctx, traceID, agentID, name, riskLevel, normalizedInput)
			if requestErr != nil {
				return nil, fmt.Errorf("requesting approval: %w", requestErr)
			}
			if r.auditLogger != nil {
				r.auditLogger.LogAudit(traceID, agentID, name, riskLevel, "Blocked", ErrApprovalRequired.Error(), 0)
			}
			return nil, &ApprovalRequiredError{Approval: *request}
		}
		if approvalManager == nil {
			return nil, errors.New("approval manager is unavailable")
		}
		if err := approvalManager.ValidateApproved(ctx, approvalID, agentID, name, normalizedInput); err != nil {
			return nil, fmt.Errorf("validating approval: %w", err)
		}
	}

	startTime := time.Now()
	res, err := tool.Execute(ctx, normalizedInput)

	duration := time.Since(startTime)
	if err != nil {
		if r.auditLogger != nil {
			r.auditLogger.LogAudit(traceID, agentID, name, riskLevel, "Failed", err.Error(), duration.Milliseconds())
		}
		if r.bus != nil {
			r.bus.Publish(bus.NewEvent(bus.EventToolExecutionError, agentID, map[string]any{
				"tool_name":   name,
				"error":       err.Error(),
				"duration_ms": duration.Milliseconds(),
			}))
		}
		return nil, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}

	if r.auditLogger != nil {
		r.auditLogger.LogAudit(traceID, agentID, name, riskLevel, "Success", "", duration.Milliseconds())
	}
	if r.bus != nil {
		r.bus.Publish(bus.NewEvent(bus.EventToolExecutionResult, agentID, map[string]any{
			"tool_name":   name,
			"result":      res.Content,
			"duration_ms": duration.Milliseconds(),
		}))
	}

	return res, nil
}

// ToolRiskLevel returns the deterministic risk class for a tool.
func ToolRiskLevel(name string) string {
	switch name {
	case "native_exec", "native_file_delete", "native_file_write", "native_cron_schedule":
		return "High"
	case "native_browser_navigate", "native_browser_screenshot", "native_http_fetch", "native_channel_notify":
		return "Medium"
	}
	if strings.HasPrefix(name, "mcp_") || strings.HasPrefix(name, "wasm_") {
		return "High"
	}
	return "Low"
}

func toolAllowed(authorized []string, name string) bool {
	for _, allowed := range authorized {
		if allowed == "*" || allowed == name {
			return true
		}
	}
	return false
}

func approvalRequired(threshold, risk string) bool {
	order := map[string]int{"Low": 1, "Medium": 2, "High": 3}
	requiredAt, ok := order[threshold]
	if !ok {
		requiredAt = order["Medium"]
	}
	return order[risk] >= requiredAt
}

func validateAllowedPath(allowedPaths []string, input json.RawMessage) error {
	for _, allowed := range allowedPaths {
		if allowed == "*" {
			return nil
		}
	}
	var request struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &request); err != nil {
		return fmt.Errorf("decoding file path: %w", err)
	}
	requested := filepath.Clean(request.Path)
	for _, allowed := range allowedPaths {
		cleanAllowed := filepath.Clean(allowed)
		rel, err := filepath.Rel(cleanAllowed, requested)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside the agent's allowed workspace paths", request.Path)
}

func newTraceID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buffer)
}

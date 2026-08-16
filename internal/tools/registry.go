package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
)

var (
	ErrToolNotFound      = errors.New("tool not found in registry")
	ErrToolAlreadyExists = errors.New("tool already registered")
	ErrExecutionFailed   = errors.New("tool execution failed")
)

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

// ToolRegistry manages all registered tools (Native, MCP, WASM, Skills).
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	bus   *bus.EventBus
}

// NewToolRegistry creates a new ToolRegistry instance.
func NewToolRegistry(eventBus *bus.EventBus) *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
		bus:   eventBus,
	}
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

// Execute executes a tool by name with input JSON parameters.
func (r *ToolRegistry) Execute(ctx context.Context, agentID, name string, inputJSON json.RawMessage) (*ToolResult, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	if r.bus != nil {
		r.bus.Publish(bus.NewEvent(bus.EventToolExecutionStarted, agentID, map[string]any{
			"tool_name": name,
			"input":     string(inputJSON),
		}))
	}

	startTime := time.Now()
	res, err := tool.Execute(ctx, inputJSON)

	duration := time.Since(startTime)
	if err != nil {
		if r.bus != nil {
			r.bus.Publish(bus.NewEvent(bus.EventToolExecutionError, agentID, map[string]any{
				"tool_name":   name,
				"error":       err.Error(),
				"duration_ms": duration.Milliseconds(),
			}))
		}
		return nil, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
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

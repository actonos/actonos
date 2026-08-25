package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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
	agentIDContextKey  executionContextKey = "agent_id"
)

// WithAgentID attaches the calling agent ID to context.
func WithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentIDContextKey, agentID)
}

// AgentIDFromContext retrieves the calling agent ID from context.
func AgentIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(agentIDContextKey).(string); ok {
		return val
	}
	return ""
}

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
	Name                string             `json:"name"`
	Description         string             `json:"description"`
	Category            string             `json:"category"`
	Schema              json.RawMessage    `json:"schema"`
	Enabled             bool               `json:"enabled"`
	RequirementsMet     bool               `json:"requirements_met"`
	Requirements        *SkillRequirements `json:"requirements,omitempty"`
	MissingRequirements []string           `json:"missing_requirements,omitempty"`
}

// SkillCatalogEntry is the compact prompt-facing view of an enabled skill.
type SkillCatalogEntry struct {
	Name        string
	Description string
	Path        string
}

func toolToInfo(t Tool) ToolInfo {
	info := ToolInfo{
		Name:            t.Name(),
		Description:     t.Description(),
		Category:        t.Category(),
		Schema:          t.ParametersSchema(),
		Enabled:         true,
		RequirementsMet: true,
	}

	if st, ok := t.(*SkillTool); ok {
		info.Enabled = st.IsEnabled()
		info.RequirementsMet = st.RequirementsMet()
		reqs := st.Requirements()
		info.Requirements = &reqs
		info.MissingRequirements = st.MissingRequirements()
	}

	return info
}

// ToolAuditLogger defines interface for writing tool audit logs.
type ToolAuditLogger interface {
	LogAudit(traceID, agentID, toolName, riskLevel, status, errorMsg string, durationMS int64)
}

// FileMutationSink receives successful native workspace writes and deletes.
type FileMutationSink interface {
	NotifyFileMutation(ctx context.Context, absolutePath, agentID string, deleted bool) error
}

// WorkspaceMutationSink receives successful user workspace changes. Unlike
// FileMutationSink, it is keyed only by opaque file ID and never exposes a
// host path.
type WorkspaceMutationSink interface {
	NotifyWorkspaceMutation(ctx context.Context, fileID, agentID string, deleted bool) error
}

// ToolRegistry manages all registered tools (Native, MCP, WASM, Skills).
type ToolRegistry struct {
	mu                 sync.RWMutex
	tools              map[string]Tool
	bus                *bus.EventBus
	auditLogger        ToolAuditLogger
	policyResolver     PolicyResolver
	approvals          *ApprovalManager
	fileMutations      FileMutationSink
	workspaceMutations WorkspaceMutationSink
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

func (r *ToolRegistry) SetFileMutationSink(sink FileMutationSink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fileMutations = sink
}

func (r *ToolRegistry) SetWorkspaceMutationSink(sink WorkspaceMutationSink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceMutations = sink
}

// SetChannelSender wires up the messaging channel dispatcher for native_channel_notify.
func (r *ToolRegistry) SetChannelSender(s ChannelMessageSender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tool, ok := r.tools["native_channel_notify"].(*ChannelNotifyTool); ok {
		tool.SetSender(s)
	}
}

// WithTraceID propagates an end-to-end trace identifier into tool execution.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

// WithApprovalID binds a previously approved exact action to execution.
func WithApprovalID(ctx context.Context, approvalID string) context.Context {
	return context.WithValue(ctx, approvalContextKey, approvalID)
}

type taskIDContextKey struct{}

// WithTaskID attaches the autonomous mission id so approval grants can be
// scoped to "don't ask again for this task".
func WithTaskID(ctx context.Context, taskID string) context.Context {
	if strings.TrimSpace(taskID) == "" {
		return ctx
	}
	return context.WithValue(ctx, taskIDContextKey{}, taskID)
}

// TaskIDFromContext returns the mission id on ctx, including the legacy
// string key used by heartbeat.
func TaskIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(taskIDContextKey{}).(string); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	if val, _ := ctx.Value("task_id").(string); strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return ""
}

type bypassApprovalContextKey struct{}

// WithBypassApproval marks the context to bypass interactive approval requirements (e.g. for background cron jobs).
func WithBypassApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, bypassApprovalContextKey{}, true)
}

// IsApprovalBypassed checks if approval checks should be bypassed for this execution.
func IsApprovalBypassed(ctx context.Context) bool {
	val, _ := ctx.Value(bypassApprovalContextKey{}).(bool)
	return val
}

type deniedToolsContextKey struct{}

// ErrToolDeniedInContext is returned when a tool is barred from the current
// execution context regardless of the agent's own authorization.
var ErrToolDeniedInContext = errors.New("tool is not permitted in this execution context")

// WithDeniedTools bars specific tools for the lifetime of this context. It is a
// hard execution-boundary guard, not a prompt hint: autonomous loops use it so a
// model cannot take self-directed actions (such as scheduling new cron jobs) that
// the operator never asked for. Denials accumulate across nested calls.
func WithDeniedTools(ctx context.Context, names ...string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	denied := map[string]bool{}
	if existing, ok := ctx.Value(deniedToolsContextKey{}).(map[string]bool); ok {
		for name := range existing {
			denied[name] = true
		}
	}
	for _, name := range names {
		denied[name] = true
	}
	return context.WithValue(ctx, deniedToolsContextKey{}, denied)
}

// IsToolDenied reports whether a tool is barred in this context.
func IsToolDenied(ctx context.Context, name string) bool {
	denied, ok := ctx.Value(deniedToolsContextKey{}).(map[string]bool)
	return ok && denied[name]
}

// DeniedTools lists the tools barred in this context.
func DeniedTools(ctx context.Context) []string {
	denied, ok := ctx.Value(deniedToolsContextKey{}).(map[string]bool)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(denied))
	for name := range denied {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type allowedToolsContextKey struct{}

// WithAllowedTools hard-restricts execution to exactly these tools for the
// lifetime of this context. Unlike WithDeniedTools (a blocklist), this is an
// allowlist: nesting calls can only narrow the set further (intersection),
// never broaden it. Autonomous routine loops (e.g. the heartbeat's zero-noise
// cycle) use this so a model cannot go off-script and call tools nobody asked
// it to use (browser navigation, channel notify, cron scheduling, ...).
func WithAllowedTools(ctx context.Context, names ...string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	if existing, ok := ctx.Value(allowedToolsContextKey{}).(map[string]bool); ok {
		narrowed := map[string]bool{}
		for name := range allowed {
			if existing[name] {
				narrowed[name] = true
			}
		}
		allowed = narrowed
	}
	return context.WithValue(ctx, allowedToolsContextKey{}, allowed)
}

// IsToolAllowedInContext reports whether a tool passes the context allowlist.
// With no allowlist set, every tool is allowed (subject to IsToolDenied).
func IsToolAllowedInContext(ctx context.Context, name string) bool {
	allowed, ok := ctx.Value(allowedToolsContextKey{}).(map[string]bool)
	if !ok {
		return true
	}
	return allowed[name]
}

// AllowedTools lists the tools permitted in this context, or nil when
// unrestricted.
func AllowedTools(ctx context.Context) []string {
	allowed, ok := ctx.Value(allowedToolsContextKey{}).(map[string]bool)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

// RegisterOrReplace adds a tool to the registry, replacing any existing tool with the same name.
func (r *ToolRegistry) RegisterOrReplace(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Unregister removes a tool from the registry.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// NormalizeToolName aliases common LLM hallucinated tool names to registered native tools.
func NormalizeToolName(name string) string {
	clean := strings.TrimSpace(name)
	lower := strings.ToLower(clean)
	switch lower {
	case "websearch", "web_search", "google_search", "search", "browse", "web":
		return "native_web_search"
	case "readfile", "read_file", "read", "file_read", "fetch_file", "open_file":
		return "native_file_read"
	case "writefile", "write_file", "create_file", "file_write", "save_file", "put_file":
		return "native_file_write"
	case "editfile", "edit_file", "file_edit", "replace_file", "patch_file", "native_file_edit":
		return "native_file_edit"
	case "listfiles", "list_files", "list_dir", "ls", "dir", "file_list", "find_files", "files_search", "native_file_list", "native_file_search":
		return "native_file_search"
	case "movefile", "move_file", "file_move", "rename_file", "native_file_move":
		return "native_file_move"
	case "copyfile", "copy_file", "file_copy", "native_file_copy":
		return "native_file_copy"
	case "deletefile", "delete_file", "remove_file", "file_delete", "rm", "native_file_delete":
		return "native_file_delete"
	case "workspace_search", "search_workspace", "user_file_search":
		return "native_workspace_search"
	case "workspace_read", "read_workspace", "user_file_read":
		return "native_workspace_read"
	case "workspace_write", "write_workspace", "user_file_write":
		return "native_workspace_write"
	case "workspace_delete", "delete_workspace", "user_file_delete":
		return "native_workspace_delete"
	case "task_enqueue", "enqueue_task", "enqueue_mission", "create_task", "create_mission", "native_task_create":
		return "native_task_enqueue"
	case "subshell", "bash", "sh", "exec", "powershell", "terminal", "run_command", "shell":
		return "native_subshell"
	case "browser", "browser_open", "web_browser":
		return "native_browser"
	case "view_skill", "read_skill":
		return "skill_view"
	}
	return clean
}

// Get retrieves a tool by name (supporting canonical aliases).
func (r *ToolRegistry) Get(name string) (Tool, error) {
	canonicalName := NormalizeToolName(name)

	r.mu.RLock()
	tool, ok := r.tools[canonicalName]
	if !ok {
		tool, ok = r.tools[name]
	}
	r.mu.RUnlock()

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
		out = append(out, toolToInfo(t))
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
			out = append(out, toolToInfo(t))
		}
	}
	return out
}

// SetToolEnabled enables or disables an installed skill tool.
func (r *ToolRegistry) SetToolEnabled(name string, enabled bool) error {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	if st, ok := tool.(*SkillTool); ok {
		return st.SetEnabled(enabled)
	}

	return fmt.Errorf("tool '%s' does not support enable/disable toggle", name)
}

// ToLLMToolDefinitions converts authorized tools into LLM-compatible tool definitions.
// excludedTools are omitted even when the agent has wildcard authorization.
func (r *ToolRegistry) ToLLMToolDefinitions(authorizedTools []string, excludedTools ...string) []llm.ToolDefinition {
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
	excluded := make(map[string]bool, len(excludedTools))
	for _, name := range excludedTools {
		excluded[name] = true
	}

	var defs []llm.ToolDefinition
	for name, t := range r.tools {
		if st, ok := t.(*SkillTool); ok {
			if !st.IsEnabled() || !st.RequirementsMet() {
				continue // Skip disabled or unsatisfied skills for LLM
			}
		}

		if (allowAll || authMap[name]) && !excluded[name] {
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

// EnabledSkillCatalog returns enabled skills whose requirements are satisfied,
// optionally filtered to an agent's authorized tool list. Wildcard "*" includes
// every enabled skill; an empty authorized list includes none.
func (r *ToolRegistry) EnabledSkillCatalog(authorizedTools ...string) []SkillCatalogEntry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	allowAll := false
	authMap := make(map[string]bool, len(authorizedTools))
	for _, a := range authorizedTools {
		if a == "*" {
			allowAll = true
		}
		if a != "" {
			authMap[a] = true
		}
	}

	var out []SkillCatalogEntry
	for _, t := range r.tools {
		st, ok := t.(*SkillTool)
		if !ok || !st.IsEnabled() || !st.RequirementsMet() {
			continue
		}
		name := st.Name()
		if !allowAll && !authMap[name] {
			continue
		}
		entry := SkillCatalogEntry{
			Name:        name,
			Description: strings.TrimSpace(st.Description()),
		}
		if folder := st.FolderPath(); folder != "" {
			entry.Path = filepath.ToSlash(filepath.Join("skills", filepath.Base(folder), "SKILL.md"))
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// NormalizeToolInput converts raw string, double-JSON-encoded, or malformed inputs into valid JSON object bytes.
func NormalizeToolInput(inputJSON json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(inputJSON))
	if len(trimmed) == 0 || trimmed == `""` || trimmed == "{}" {
		return json.RawMessage(`{}`)
	}

	// 1. If wrapped in outer quotes (JSON string literal like "\"{\\\"path\\\": ...}\"" or "\"https://example.com\"")
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
			// If it starts with { or contains JSON keys, return as raw unescaped JSON without wrapping
			if strings.HasPrefix(unescapedTrimmed, "{") || strings.Contains(unescapedTrimmed, `":`) {
				return json.RawMessage(unescapedTrimmed)
			}
			// Wrap simple plain string with universal property names
			obj, _ := json.Marshal(map[string]any{
				"url":     unescapedTrimmed,
				"path":    unescapedTrimmed,
				"command": unescapedTrimmed,
				"input":   unescapedTrimmed,
			})
			return json.RawMessage(obj)
		}
	}

	// 2. If it's already a JSON object or array
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		return inputJSON
	}

	// If it starts with { or contains JSON keys, return as raw message rather than wrapping
	if strings.HasPrefix(trimmed, "{") || strings.Contains(trimmed, `":`) {
		return json.RawMessage(trimmed)
	}

	// 3. Fallback: Wrap raw plain string
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
	if agentID != "" && AgentIDFromContext(ctx) == "" {
		ctx = WithAgentID(ctx, agentID)
	}
	if IsToolDenied(ctx, name) {
		return nil, fmt.Errorf("%w: agent=%s tool=%s", ErrToolDeniedInContext, agentID, name)
	}
	if !IsToolAllowedInContext(ctx, name) {
		return nil, fmt.Errorf("%w: agent=%s tool=%s", ErrToolDeniedInContext, agentID, name)
	}

	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	if st, ok := tool.(*SkillTool); ok {
		if !st.IsEnabled() {
			return nil, fmt.Errorf("skill '%s' is currently disabled", name)
		}
		if met, missing := st.CheckRequirements(); !met {
			return nil, fmt.Errorf("skill '%s' requirements not satisfied: %s", name, strings.Join(missing, "; "))
		}
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
			"input":     toolInputForEvent(name, normalizedInput),
		}))
	}

	riskLevel := ToolRiskLevel(name)
	traceID := TraceIDFromContext(ctx)
	if traceID == "" {
		traceID = newTraceID()
		ctx = WithTraceID(ctx, traceID)
	}

	if resolver != nil && approvalRequired(policy.ApprovalThreshold, riskLevel) && !IsApprovalBypassed(ctx) {
		skipExactApproval := false
		if approvalManager != nil && GrantEligibleTool(name) {
			grant, grantErr := approvalManager.ActiveGrant(ctx, agentID, name, TaskIDFromContext(ctx))
			if grantErr != nil {
				return nil, fmt.Errorf("checking approval grant: %w", grantErr)
			}
			if grant != nil {
				skipExactApproval = true
				if r.auditLogger != nil {
					r.auditLogger.LogAudit(traceID, agentID, name, riskLevel, "Granted", grant.ID, 0)
				}
			}
		}
		if !skipExactApproval {
			approvalID, _ := ctx.Value(approvalContextKey).(string)
			if approvalID == "" {
				request, requestErr := approvalManager.Request(ctx, traceID, agentID, name, riskLevel, normalizedInput)
				if requestErr != nil {
					return nil, fmt.Errorf("requesting approval: %w", requestErr)
				}
				if r.auditLogger != nil {
					r.auditLogger.LogAudit(traceID, agentID, name, riskLevel, "Blocked", ErrApprovalRequired.Error(), 0)
				}
				// Only publish the bus event (and therefore surface a new web
				// notification) for a genuinely new approval. A reused pending
				// approval means the operator was already asked once.
				if r.bus != nil && request.IsNew() {
					r.bus.Publish(bus.NewEvent("approval:required", agentID, map[string]any{
						"approval": *request,
					}))
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
			"result":      toolResultForEvent(name, res),
			"duration_ms": duration.Milliseconds(),
		}))
	}

	if res != nil && (name == "native_file_write" || name == "native_file_delete") {
		r.mu.RLock()
		sink := r.fileMutations
		r.mu.RUnlock()
		if sink != nil && res.Data != nil {
			if absolutePath, ok := res.Data["absolute_path"].(string); ok && absolutePath != "" {
				_ = sink.NotifyFileMutation(context.Background(), absolutePath, agentID, name == "native_file_delete")
			}
		}
	}
	if res != nil && (name == "native_workspace_write" || name == "native_workspace_delete" || name == "native_exec") {
		r.mu.RLock()
		sink := r.workspaceMutations
		r.mu.RUnlock()
		if sink != nil && res.Data != nil {
			if fileID, ok := res.Data["workspace_file_id"].(string); ok && fileID != "" {
				_ = sink.NotifyWorkspaceMutation(context.Background(), fileID, agentID, name == "native_workspace_delete")
			}
			switch ids := res.Data["workspace_file_ids"].(type) {
			case []string:
				for _, fileID := range ids {
					if fileID != "" {
						_ = sink.NotifyWorkspaceMutation(context.Background(), fileID, agentID, false)
					}
				}
			}
		}
	}

	return res, nil
}

func toolInputForEvent(name string, input json.RawMessage) string {
	if name != "native_file_write" && name != "native_workspace_write" {
		return string(input)
	}
	var payload map[string]any
	if json.Unmarshal(input, &payload) != nil {
		return `{"content":"[redacted]"}`
	}
	for _, key := range []string{"content", "content_base64"} {
		if value, exists := payload[key]; exists {
			length := len(fmt.Sprint(value))
			payload[key] = fmt.Sprintf("[redacted:%d chars]", length)
		}
	}
	redacted, err := json.Marshal(payload)
	if err != nil {
		return `{"content":"[redacted]"}`
	}
	return string(redacted)
}

func toolResultForEvent(name string, result *ToolResult) string {
	if result == nil {
		return ""
	}
	switch name {
	case "native_file_read", "native_workspace_read", "native_workspace_search":
		return "[workspace content omitted]"
	default:
		return result.Content
	}
}

// ToolRiskLevel returns the deterministic risk class for a tool.
func ToolRiskLevel(name string) string {
	switch name {
	case "native_exec", "native_file_delete", "native_workspace_delete", "native_cron_schedule":
		return "High"
	case "native_file_write", "native_workspace_write", "native_browser_navigate", "native_browser_screenshot", "native_http_fetch", "native_channel_notify":
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
	switch strings.ToLower(threshold) {
	case "low", "none", "auto":
		// Low approval friction: all authorized tools auto-execute without requiring approval
		return false
	case "medium":
		// Medium approval: workspace writes and safe tools auto-execute; only High risk tools (exec, delete, cron, extensions) require human approval
		return risk == "High"
	case "high", "strict", "always":
		// High approval: all state-modifying / sensitive tools (Medium and High risk) require approval. Only read-only Low risk tools auto-execute.
		return risk == "High" || risk == "Medium"
	default:
		return risk == "High"
	}
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

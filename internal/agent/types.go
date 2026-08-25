package agent

import (
	"time"

	"github.com/actonos/actonos/internal/llm"
)

// AgentStatus represents the lifecycle status of an agent.
type AgentStatus string

const (
	StatusActive  AgentStatus = "active"
	StatusStopped AgentStatus = "stopped"
	StatusError   AgentStatus = "error"
)

// ApprovalLevel represents human-in-the-loop requirement.
type ApprovalLevel string

const (
	ApprovalLow    ApprovalLevel = "Low"    // Auto-execute all authorized tools without human approval.
	ApprovalMedium ApprovalLevel = "Medium" // Auto-execute workspace writes/reads and web tools; require human approval for High-risk/dangerous actions (exec, delete, cron, extensions).
	ApprovalHigh   ApprovalLevel = "High"   // Require human approval for all state-modifying actions (workspace writes, delete, exec, etc.).
)

// DelegationScope restricts sub-agents and tool execution.
type DelegationScope struct {
	MaxMonthlyBudgetUSD   float64       `json:"max_monthly_budget_usd"`
	MaxConcurrentRuns     int           `json:"max_concurrent_runs,omitempty"`
	MaxTokensPerHour      int           `json:"max_tokens_per_hour,omitempty"`
	AllowedWorkspacePaths []string      `json:"allowed_workspace_paths"`
	RequireHumanApproval  ApprovalLevel `json:"require_human_approval_level"`
}

// TriggerRule defines triggers for agent activation.
type TriggerRule struct {
	Type       string `json:"type"` // "channel_mention", "cron_schedule", "webhook"
	Channel    string `json:"channel,omitempty"`
	Filter     string `json:"filter,omitempty"`
	Expression string `json:"expression,omitempty"`
}

const (
	// DefaultSystemAgentID is the fixed identifier for the built-in system operator agent.
	DefaultSystemAgentID = "agent_system_core"
)

// AgentHeartbeatConfig holds per-agent autonomous heartbeat directives and schedule parameters.
type AgentHeartbeatConfig struct {
	Enabled             bool   `json:"enabled"`
	IntervalMinutes     int    `json:"interval_minutes,omitempty"`
	Directives          string `json:"directives,omitempty"`
	TargetChannel       string `json:"target_channel,omitempty"`
	TargetAccountID     string `json:"target_account_id,omitempty"`
	ActiveHoursStart    string `json:"active_hours_start,omitempty"`
	ActiveHoursEnd      string `json:"active_hours_end,omitempty"`
	ActiveHoursTimezone string `json:"active_hours_timezone,omitempty"`
}

// AgentManifest contains the complete declaration and configuration of an agent.
type AgentManifest struct {
	AgentID            string                `json:"agent_id"`
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	AvatarIcon         string                `json:"avatar_icon"`
	Status             AgentStatus           `json:"status"`
	IsSystem           bool                  `json:"is_system,omitempty"`
	ModelConfig        llm.ModelConfig       `json:"model_config"`
	SystemInstructions string                `json:"system_instructions"`
	AuthorizedTools    []string              `json:"authorized_tools"`
	// ListenChannels defines which chat channels this agent responds to.
	// ["*"] means all channels (default). Specific channel IDs like
	// ["telegram", "discord"] or ["telegram:bot_1"] restrict the agent.
	ListenChannels  []string              `json:"listen_channels"`
	HeartbeatConfig *AgentHeartbeatConfig `json:"heartbeat_config,omitempty"`
	DelegationScope DelegationScope       `json:"delegation_scope"`
	TriggerRules    []TriggerRule         `json:"trigger_rules"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// SubTask represents a decomposed work item assigned to a sub-agent.
type SubTask struct {
	ID              string         `json:"id"`
	ParentAgentID   string         `json:"parent_agent_id"`
	AssignedAgentID string         `json:"assigned_agent_id,omitempty"`
	Title           string         `json:"title"`
	Prompt          string         `json:"prompt"`
	InputContext    map[string]any `json:"input_context,omitempty"`
	AuthorizedTools []string       `json:"authorized_tools,omitempty"`
	Timeout         time.Duration  `json:"timeout,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
}

// SubTaskResult captures the output, metrics, and error of a completed sub-task.
type SubTaskResult struct {
	TaskID        string         `json:"task_id"`
	AgentID       string         `json:"agent_id"`
	Output        string         `json:"output"`
	Data          map[string]any `json:"data,omitempty"`
	ToolCallsMade []llm.ToolCall `json:"tool_calls_made,omitempty"`
	TokensUsed    int            `json:"tokens_used"`
	ExecutionTime time.Duration  `json:"execution_time"`
	Status        string         `json:"status"` // "success", "failed", "timeout"
	Error         string         `json:"error,omitempty"`
}

// AgentState tracks volatile runtime state for an active agent.
type AgentState struct {
	AgentID        string          `json:"agent_id"`
	WorkingMemory  map[string]any  `json:"working_memory"`
	ActiveSubTasks map[string]bool `json:"active_subtasks"`
	LastActiveAt   time.Time       `json:"last_active_at"`
}

// StreamEventType describes the granular state of streaming execution.
type StreamEventType string

const (
	EventStreamThought    StreamEventType = "thought"
	EventStreamReasoning  StreamEventType = "reasoning"
	EventStreamToken      StreamEventType = "token"
	// EventStreamTokenReset tells the client to discard the tokens streamed during
	// the current ReAct iteration. Emitted when an iteration turns out to be a
	// tool-calling turn, so its preamble prose must not become the final answer.
	EventStreamTokenReset StreamEventType = "token_reset"
	EventStreamToolCall   StreamEventType = "tool_call"
	EventStreamToolResult StreamEventType = "tool_result"
	EventStreamAudit      StreamEventType = "audit"
	EventStreamDone       StreamEventType = "done"
	EventStreamError      StreamEventType = "error"
)

// AuditLogEntry tracks an immutable security/governance record of an agent action.
type AuditLogEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	AgentID      string    `json:"agent_id"`
	Action       string    `json:"action"`
	ToolName     string    `json:"tool_name,omitempty"`
	Parameters   string    `json:"parameters,omitempty"`
	Status       string    `json:"status"`
	Verification string    `json:"verification"` // e.g. "Tier 1 AST Clean", "Memory Search", "Sandbox Isolation"
	DurationMs   int64     `json:"duration_ms"`
}

// AgentStreamEvent encapsulates an event emitted during ReAct cognitive execution.
type AgentStreamEvent struct {
	Type           StreamEventType `json:"type"`
	Thought        string          `json:"thought,omitempty"`
	Reasoning      string          `json:"reasoning,omitempty"`
	Content        string          `json:"content,omitempty"`
	Tool           string          `json:"tool,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	Args           string          `json:"args,omitempty"`
	Result         string          `json:"result,omitempty"`
	Status         string          `json:"status,omitempty"`
	LatencyMs      int64           `json:"latency_ms,omitempty"`
	AuditLog       *AuditLogEntry  `json:"audit_log,omitempty"`
	Model          string          `json:"model,omitempty"`
	Usage          *llm.Usage      `json:"usage,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Title          string          `json:"title,omitempty"`
	Error          string          `json:"error,omitempty"`
}

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
	ApprovalLow    ApprovalLevel = "Low"    // Require approval for every tool action.
	ApprovalMedium ApprovalLevel = "Medium" // Auto-run read-only actions; approve network/write/destructive actions.
	ApprovalHigh   ApprovalLevel = "High"   // Auto-run Low/Medium actions; approve only destructive or privileged actions.
)

// DelegationScope restricts sub-agents and tool execution.
type DelegationScope struct {
	MaxMonthlyBudgetUSD   float64       `json:"max_monthly_budget_usd"`
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

// AgentManifest contains the complete declaration and configuration of an agent.
type AgentManifest struct {
	AgentID            string          `json:"agent_id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	AvatarIcon         string          `json:"avatar_icon"`
	Status             AgentStatus     `json:"status"`
	IsSystem           bool            `json:"is_system,omitempty"`
	ModelConfig        llm.ModelConfig `json:"model_config"`
	SystemInstructions string          `json:"system_instructions"`
	AuthorizedTools    []string        `json:"authorized_tools"`
	// ListenChannels defines which chat channels this agent responds to.
	// ["*"] means all channels (default). Specific channel IDs like
	// ["telegram", "discord"] restrict the agent to only those channels.
	ListenChannels  []string        `json:"listen_channels"`
	DelegationScope DelegationScope `json:"delegation_scope"`
	TriggerRules    []TriggerRule   `json:"trigger_rules"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
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
	Entropy        float64         `json:"entropy"`
	LastActiveAt   time.Time       `json:"last_active_at"`
}

// StreamEventType describes the granular state of streaming execution.
type StreamEventType string

const (
	EventStreamThought    StreamEventType = "thought"
	EventStreamToken      StreamEventType = "token"
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
	Parameters   any       `json:"parameters,omitempty"`
	Status       string    `json:"status"`
	Verification string    `json:"verification"` // e.g. "Tier 1 AST Clean", "Memory Search", "Sandbox Isolation"
	DurationMs   int64     `json:"duration_ms"`
}

// AgentStreamEvent encapsulates an event emitted during ReAct cognitive execution.
type AgentStreamEvent struct {
	Type           StreamEventType `json:"type"`
	Content        string          `json:"content,omitempty"`
	Thought        string          `json:"thought,omitempty"`
	Tool           string          `json:"tool,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	Args           any             `json:"args,omitempty"`
	Result         string          `json:"result,omitempty"`
	Status         string          `json:"status,omitempty"`
	LatencyMs      int64           `json:"latency_ms,omitempty"`
	AuditLog       *AuditLogEntry  `json:"audit_log,omitempty"`
	Usage          *llm.Usage      `json:"usage,omitempty"`
	Model          string          `json:"model,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Title          string          `json:"title,omitempty"`
	Error          string          `json:"error,omitempty"`
}

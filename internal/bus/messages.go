package bus

import (
	"time"

	"github.com/google/uuid"
)

// Standard Event Type constants across ActonOS subsystems.
const (
	// Agent events
	EventAgentCreated       = "agent.created"
	EventAgentUpdated       = "agent.updated"
	EventAgentDeleted       = "agent.deleted"
	EventAgentStatusChanged = "agent.status_changed"
	EventAgentActionStarted = "agent.action_started"
	EventAgentActionDone    = "agent.action_done"
	EventAgentActionFailed  = "agent.action_failed"

	// Swarm events
	EventSubTaskSpawned   = "swarm.subtask_spawned"
	EventSubTaskCompleted = "swarm.subtask_completed"
	EventSubTaskFailed    = "swarm.subtask_failed"

	// Tool & Skill events
	EventToolExecutionStarted = "tool.started"
	EventToolExecutionResult  = "tool.result"
	EventToolExecutionError   = "tool.error"
	EventSkillProgress        = "skill.progress"
	EventSkillInstalled       = "skill.installed"
	EventSkillUninstalled     = "skill.uninstalled"

	// Plugin events
	EventPluginProgress    = "plugin.progress"
	EventPluginInstalled   = "plugin.installed"
	EventPluginUninstalled = "plugin.uninstalled"

	// Auth & Token events
	EventTokenRefreshed = "auth.token_refreshed"
	EventTokenExpired   = "auth.token_expired"
	EventTokenFailed    = "auth.token_failed"

	// Integration health events (chat channels, connectors, MCP servers).
	// Published whenever a connection fails to establish, is lost
	// unexpectedly, or recovers, so operators can be notified on the web UI
	// instead of the failure only ever reaching the server log.
	EventChannelAdapterError     = "channel.adapter_error"
	EventChannelAdapterRecovered = "channel.adapter_recovered"
	EventChannelMessage          = "channel.message_inbound"
	EventMCPServerError          = "mcp.server_error"
	EventMCPServerRecovered      = "mcp.server_recovered"

	// Memory & Storage events
	EventMemoryStored  = "memory.stored"
	EventMemoryDecayed = "memory.decayed"
	EventMemoryQuery   = "memory.query"

	// System events
	EventSystemBoot       = "system.boot"
	EventSystemShutdown   = "system.shutdown"
	EventSystemHeartbeat  = "system.heartbeat"
	EventUserMessage      = "user.message"
)

// Event represents a structured event transmitted over the EventBus.
type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	AgentID   string         `json:"agent_id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   any            `json:"payload,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// NewEvent creates a new Event instance with a generated UUID and current timestamp.
func NewEvent(eventType string, agentID string, payload any) Event {
	return Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		AgentID:   agentID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		Metadata:  make(map[string]any),
	}
}

package channels

import (
	"context"
)

// ChannelAccount represents a specific connected account within a channel (e.g. Bot 1, Phone 2).
type ChannelAccount struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Channel       string            `json:"channel"` // "telegram", "whatsapp", "discord", "webhook"
	Token         string            `json:"token,omitempty"`
	PhoneID       string            `json:"phone_id,omitempty"`
	WebhookSecret string            `json:"webhook_secret,omitempty"`
	Enabled       bool              `json:"enabled"`
	BoundAgentIDs []string          `json:"bound_agent_ids"` // ["*"] for all, or ["agent_support", "agent_devops"]
	DefaultChatID string            `json:"default_chat_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// InboundMessage represents an incoming message received from a channel.
type InboundMessage struct {
	ChannelID   string            `json:"channel_id"` // "telegram", "whatsapp", "discord", "webhook"
	AccountID   string            `json:"account_id"` // specific account identifier
	SenderID    string            `json:"sender_id"`
	SenderName  string            `json:"sender_name"`
	TargetAgent string            `json:"target_agent"` // Extracted @mention or bound agent
	Content     string            `json:"content"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a response to be dispatched out to a channel.
type OutboundMessage struct {
	ChannelID string `json:"channel_id"` // "telegram", "whatsapp", "discord", "all"
	AccountID string `json:"account_id"` // specific account ID or ""/"all" for broadcast
	Recipient string `json:"recipient"`  // specific recipient or ""/"all"
	Content   string `json:"content"`
}

// ChannelAdapter defines the contract for multi-platform chat and messaging integrations.
type ChannelAdapter interface {
	// Name returns the channel type name (e.g. "telegram", "discord", "webhook").
	Name() string

	// Start begins listening on the channel.
	Start(ctx context.Context) error

	// Stop gracefully terminates channel listeners.
	Stop() error

	// SendMessage sends an outbound message to a channel recipient.
	SendMessage(ctx context.Context, msg OutboundMessage) error
}

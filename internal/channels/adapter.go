package channels

import (
	"context"
)

// InboundMessage represents an incoming message received from a channel.
type InboundMessage struct {
	ChannelID   string            `json:"channel_id"`   // "telegram", "discord", "webhook"
	SenderID    string            `json:"sender_id"`
	SenderName  string            `json:"sender_name"`
	TargetAgent string            `json:"target_agent"` // Extracted @mention or default agent
	Content     string            `json:"content"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a response to be dispatched out to a channel.
type OutboundMessage struct {
	ChannelID string `json:"channel_id"`
	Recipient string `json:"recipient"`
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

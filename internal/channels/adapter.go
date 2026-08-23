package channels

import (
	"context"
	"strings"
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
	RoutingMode   string            `json:"routing_mode,omitempty"` // "exclusive", "mention", "fallback"
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// InboundMessage represents an incoming message received from a channel.
type InboundMessage struct {
	ChannelID   string            `json:"channel_id"` // "telegram", "whatsapp", "discord", "webhook"
	AccountID   string            `json:"account_id"` // specific account identifier
	SenderID    string            `json:"sender_id"`
	SenderName  string            `json:"sender_name"`
	TargetAgent string            `json:"target_agent"` // Extracted @mention, explicit target, or ""
	MentionText string            `json:"mention_text,omitempty"`
	Content     string            `json:"content"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a response to be dispatched out to a channel.
type OutboundMessage struct {
	ChannelID string            `json:"channel_id"` // "telegram", "whatsapp", "discord", "all"
	AccountID string            `json:"account_id"` // specific account ID or ""/"all" for broadcast
	Recipient string            `json:"recipient"`  // specific recipient or ""/"all"
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ExtractAgentMention parses @agent_name, /agent agent_name, or @agent_id from user text.
// Returns the extracted agent identifier (or "") and the cleaned content without the mention prefix.
func ExtractAgentMention(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	// Pattern 1: /agent <name> <content> or /ask <name> <content>
	if strings.HasPrefix(trimmed, "/agent ") || strings.HasPrefix(trimmed, "/ask ") {
		parts := strings.SplitN(trimmed, " ", 3)
		if len(parts) >= 2 {
			agentName := strings.TrimSpace(parts[1])
			content := ""
			if len(parts) == 3 {
				content = strings.TrimSpace(parts[2])
			}
			return agentName, content
		}
	}

	// Pattern 2: @<name> <content> at the start
	if strings.HasPrefix(trimmed, "@") {
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			mention := strings.TrimPrefix(fields[0], "@")
			mention = strings.TrimRight(mention, ":,")
			content := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			return mention, content
		}
	}

	return "", trimmed
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

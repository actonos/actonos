package channels

import "strings"

// Canonical metadata aliases shared with ActonOS Plugin SDK channel plugins.
const (
	metaAccountID        = "account_id"
	metaChatID           = "chat_id"
	metaChannelID        = "channel_id"
	metaMessageID        = "message_id"
	metaMsgID            = "msg_id"
	metaReplyToID        = "reply_to_id"
	metaReplyToMsgID     = "reply_to_msg_id"
	metaReplyToMessageID = "reply_to_message_id"
	metaReplyToTS        = "reply_to_ts"
	metaThreadID         = "thread_id"
	metaThreadTS         = "thread_ts"
	metaReaction         = "reaction"
	metaTyping           = "typing"
	metaAction           = "action"
	metaTS               = "ts"
	metaTimestamp        = "timestamp"
	metaFrom             = "from"
)

var mediaMetaKeys = []string{"photo", "image_url", "document", "file_url", "voice", "voice_url", "audio_url"}

// FirstNonEmpty returns the first non-empty trimmed string.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// Normalize lifts metadata aliases into typed envelope fields and mirrors those
// fields back into Metadata so mixed old/new plugins keep working.
func (m *InboundMessage) Normalize() {
	if m == nil {
		return
	}
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	m.AccountID = FirstNonEmpty(m.AccountID, m.Metadata[metaAccountID])
	if m.AccountID != "" {
		m.Metadata[metaAccountID] = m.AccountID
	}

	m.ChatID = FirstNonEmpty(m.ChatID, m.Metadata[metaChatID], m.Metadata[metaChannelID], m.Metadata[metaFrom])
	if m.ChatID != "" {
		setMetaIfEmpty(m.Metadata, metaChatID, m.ChatID)
		setMetaIfEmpty(m.Metadata, metaChannelID, m.ChatID)
	}

	m.MessageID = FirstNonEmpty(m.MessageID, m.Metadata[metaMessageID], m.Metadata[metaMsgID], m.Metadata[metaTS])
	if m.MessageID != "" {
		setMetaIfEmpty(m.Metadata, metaMessageID, m.MessageID)
		setMetaIfEmpty(m.Metadata, metaMsgID, m.MessageID)
	}

	m.ThreadID = FirstNonEmpty(m.ThreadID, m.Metadata[metaThreadID], m.Metadata[metaThreadTS])
	if m.ThreadID != "" {
		setMetaIfEmpty(m.Metadata, metaThreadID, m.ThreadID)
		setMetaIfEmpty(m.Metadata, metaThreadTS, m.ThreadID)
	}

	m.Timestamp = FirstNonEmpty(m.Timestamp, m.Metadata[metaTimestamp], m.Metadata["date"], m.Metadata[metaTS])
	if m.Timestamp != "" {
		setMetaIfEmpty(m.Metadata, metaTimestamp, m.Timestamp)
	}

	m.Reaction = FirstNonEmpty(m.Reaction, m.Metadata[metaReaction])
	if m.Reaction != "" {
		m.Metadata[metaReaction] = m.Reaction
	}

	if m.Kind == "" {
		switch {
		case m.Reaction != "" && strings.TrimSpace(m.Content) == "":
			m.Kind = MessageKindReaction
		case hasMediaMeta(m.Metadata):
			m.Kind = MessageKindMedia
		default:
			m.Kind = MessageKindText
		}
	}
}

// Normalize lifts metadata aliases into typed envelope fields for outbound dispatch.
func (m *OutboundMessage) Normalize() {
	if m == nil {
		return
	}
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	m.AccountID = FirstNonEmpty(m.AccountID, m.Metadata[metaAccountID])
	if m.AccountID != "" {
		m.Metadata[metaAccountID] = m.AccountID
	}

	m.ChatID = FirstNonEmpty(m.ChatID, m.Metadata[metaChatID], m.Metadata[metaChannelID], m.Recipient)
	if m.ChatID != "" {
		setMetaIfEmpty(m.Metadata, metaChatID, m.ChatID)
		setMetaIfEmpty(m.Metadata, metaChannelID, m.ChatID)
	}

	m.ReplyToID = FirstNonEmpty(
		m.ReplyToID,
		m.Metadata[metaReplyToID],
		m.Metadata[metaReplyToMsgID],
		m.Metadata[metaReplyToMessageID],
		m.Metadata[metaReplyToTS],
	)
	if m.ReplyToID != "" {
		setMetaIfEmpty(m.Metadata, metaReplyToID, m.ReplyToID)
		setMetaIfEmpty(m.Metadata, metaReplyToMsgID, m.ReplyToID)
	}

	m.ThreadID = FirstNonEmpty(m.ThreadID, m.Metadata[metaThreadID], m.Metadata[metaThreadTS])
	if m.ThreadID != "" {
		setMetaIfEmpty(m.Metadata, metaThreadID, m.ThreadID)
		setMetaIfEmpty(m.Metadata, metaThreadTS, m.ThreadID)
	}

	m.Reaction = FirstNonEmpty(m.Reaction, m.Metadata[metaReaction])
	if m.Reaction != "" {
		m.Metadata[metaReaction] = m.Reaction
	}

	m.Action = FirstNonEmpty(m.Action, m.Metadata[metaAction])
	if m.Action != "" {
		m.Metadata[metaAction] = m.Action
	}

	if !m.Typing {
		m.Typing = m.Metadata[metaTyping] == "true" || m.Kind == MessageKindTyping || m.Action == "typing"
	}
	if m.Typing {
		m.Metadata[metaTyping] = "true"
	}

	if m.Kind == "" {
		switch {
		case m.Typing && strings.TrimSpace(m.Content) == "" && m.Reaction == "" && !hasMediaMeta(m.Metadata):
			m.Kind = MessageKindTyping
		case m.Reaction != "" && strings.TrimSpace(m.Content) == "" && !hasMediaMeta(m.Metadata):
			m.Kind = MessageKindReaction
		case hasMediaMeta(m.Metadata):
			m.Kind = MessageKindMedia
		default:
			m.Kind = MessageKindText
		}
	}
}

// RecipientForReply is the conversation target plugins should deliver to.
func RecipientForReply(in InboundMessage) string {
	in.Normalize()
	return FirstNonEmpty(in.ChatID, in.Metadata[metaChatID], in.Metadata[metaChannelID], in.SenderID)
}

// ConversationID is the session key for a chat (channel/chat id, not always the user id).
func ConversationID(in InboundMessage) string {
	return RecipientForReply(in)
}

// IsControlEvent reports inbound frames that must not start an agent turn.
func (m InboundMessage) IsControlEvent() bool {
	switch m.Kind {
	case MessageKindTyping, MessageKindReaction:
		return true
	default:
		return false
	}
}

// NewTyping builds an outbound typing / chat-action frame for an inbound conversation.
func NewTyping(in InboundMessage) OutboundMessage {
	in.Normalize()
	out := OutboundMessage{
		ChannelID: in.ChannelID,
		AccountID: in.AccountID,
		Recipient: RecipientForReply(in),
		ChatID:    FirstNonEmpty(in.ChatID, RecipientForReply(in)),
		Kind:      MessageKindTyping,
		Typing:    true,
		Action:    "typing",
		Metadata:  cloneMeta(in.Metadata),
	}
	out.Normalize()
	return out
}

// NewReply builds a quoted text reply to an inbound user message.
func NewReply(in InboundMessage, content string) OutboundMessage {
	in.Normalize()
	out := OutboundMessage{
		ChannelID: in.ChannelID,
		AccountID: in.AccountID,
		Recipient: RecipientForReply(in),
		Content:   content,
		Kind:      MessageKindText,
		ChatID:    FirstNonEmpty(in.ChatID, RecipientForReply(in)),
		ReplyToID: in.MessageID,
		ThreadID:  in.ThreadID,
		Metadata:  cloneMeta(in.Metadata),
	}
	out.Normalize()
	return out
}

// NewReaction builds an outbound reaction on the inbound source message.
func NewReaction(in InboundMessage, emoji string) OutboundMessage {
	in.Normalize()
	out := OutboundMessage{
		ChannelID: in.ChannelID,
		AccountID: in.AccountID,
		Recipient: RecipientForReply(in),
		ChatID:    FirstNonEmpty(in.ChatID, RecipientForReply(in)),
		Kind:      MessageKindReaction,
		ReplyToID: in.MessageID,
		Reaction:  strings.TrimSpace(emoji),
		Metadata:  cloneMeta(in.Metadata),
	}
	out.Normalize()
	return out
}

func hasMediaMeta(meta map[string]string) bool {
	if meta == nil {
		return false
	}
	for _, k := range mediaMetaKeys {
		if strings.TrimSpace(meta[k]) != "" {
			return true
		}
	}
	return false
}

func setMetaIfEmpty(meta map[string]string, key, value string) {
	if meta == nil || value == "" {
		return
	}
	if strings.TrimSpace(meta[key]) == "" {
		meta[key] = value
	}
}

func cloneMeta(src map[string]string) map[string]string {
	if len(src) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(src)+4)
	for k, v := range src {
		out[k] = v
	}
	return out
}

package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/google/uuid"
)

// ChannelSessionManager handles deterministic, intelligent session management and conversation history for multi-channel interactions.
type ChannelSessionManager struct {
	db        *sql.DB
	embedding MessageEmbeddingSink
}

type MessageEmbeddingSink interface {
	EnqueueMessage(ctx context.Context, messageID, agentID, conversationID string) error
}

// NewChannelSessionManager creates a ChannelSessionManager instance.
func NewChannelSessionManager(db *sql.DB) *ChannelSessionManager {
	return &ChannelSessionManager{db: db}
}

func (sm *ChannelSessionManager) SetEmbeddingSink(sink MessageEmbeddingSink) {
	sm.embedding = sink
}

// GetOrCreateSession ensures a deterministic conversation session exists for a given channel & sender.
func (sm *ChannelSessionManager) GetOrCreateSession(ctx context.Context, channelID, senderID, senderName, firstMessage, agentID string) (string, error) {
	if sm.db == nil {
		return fmt.Sprintf("conv_%s_%s", channelID, senderID), nil
	}

	convID := fmt.Sprintf("conv_%s_%s", channelID, senderID)
	now := time.Now().UTC()

	var existingTitle string
	err := sm.db.QueryRowContext(ctx, "SELECT title FROM conversations WHERE id = ?", convID).Scan(&existingTitle)
	if err == sql.ErrNoRows {
		// New conversation title with clear channel identity
		channelIcon := "📱"
		if channelID == "whatsapp" {
			channelIcon = "💬"
		} else if channelID == "discord" {
			channelIcon = "🎮"
		} else if channelID == "mission" {
			channelIcon = "🎯"
		} else if channelID == "system" {
			channelIcon = "⚡"
		}

		dispName := senderName
		if dispName == "" {
			dispName = senderID
		}
		title := fmt.Sprintf("%s %s: %s", channelIcon, strings.ToUpper(channelID[:1])+channelID[1:], dispName)
		if channelID == "mission" {
			title = fmt.Sprintf("🎯 Mission: %s", dispName)
		} else if channelID == "system" {
			title = fmt.Sprintf("⚡ System: %s", dispName)
		} else if len(firstMessage) > 0 && len(firstMessage) <= 30 {
			title = fmt.Sprintf("%s %s: %s", channelIcon, strings.ToUpper(channelID[:1])+channelID[1:], firstMessage)
		}

		_, err := sm.db.ExecContext(ctx, `
			INSERT INTO conversations (id, agent_id, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, convID, agentID, title, now, now)
		if err != nil {
			return convID, err
		}
	} else if err == nil {
		// Update touch time and active agent
		_, _ = sm.db.ExecContext(ctx, "UPDATE conversations SET updated_at = ?, agent_id = ? WHERE id = ?", now, agentID, convID)
	}

	return convID, nil
}

// SaveMessage records a message in the session history.
func (sm *ChannelSessionManager) SaveMessage(ctx context.Context, convID, agentID, role, content string, toolCalls any) error {
	if sm.db == nil {
		return nil
	}
	msgID := "msg_" + uuid.New().String()
	now := time.Now().UTC()

	var toolCallsJSON string
	if toolCalls != nil {
		data, _ := json.Marshal(toolCalls)
		toolCallsJSON = string(data)
	}

	_, err := sm.db.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, agent_id, role, content, tool_calls_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, msgID, convID, agentID, role, content, toolCallsJSON, now)
	if err == nil && role == string(llm.RoleUser) && sm.embedding != nil && isExternalChannelConversation(convID) {
		_ = sm.embedding.EnqueueMessage(context.Background(), msgID, agentID, convID)
	}
	return err
}

func isExternalChannelConversation(conversationID string) bool {
	for _, prefix := range []string{"conv_telegram_", "conv_whatsapp_", "conv_discord_"} {
		if strings.HasPrefix(conversationID, prefix) {
			return true
		}
	}
	return false
}

// LoadRecentHistory retrieves recent conversation history for working memory context window.
func (sm *ChannelSessionManager) LoadRecentHistory(ctx context.Context, convID string, limit int) []llm.Message {
	if sm.db == nil || limit <= 0 {
		return nil
	}

	rows, err := sm.db.QueryContext(ctx, `
		SELECT role, content
		FROM messages
		WHERE conversation_id = ?
		ORDER BY created_at DESC, rowid DESC
		LIMIT ?
	`, convID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	// Only user and assistant turns are replayed. Persisted tool_calls/tool results
	// cannot be reconstructed faithfully here (tool_call_id is not read back), and a
	// tool_call without its matching result — or a result without its call — makes
	// the provider reject the entire request. Replaying prose only keeps the history
	// useful while guaranteeing a well-formed message sequence.
	var reversed []llm.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			continue
		}
		if role != string(llm.RoleUser) && role != string(llm.RoleAssistant) {
			continue
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		reversed = append(reversed, llm.Message{
			Role:    llm.Role(role),
			Content: content,
		})
	}

	// Reverse to chronological order
	result := make([]llm.Message, len(reversed))
	for i, j := 0, len(reversed)-1; i < len(reversed); i, j = i+1, j-1 {
		result[i] = reversed[j]
	}
	return result
}

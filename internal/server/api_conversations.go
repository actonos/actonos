package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Conversation struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	Title        string    `json:"title"`
	Channel      string    `json:"channel,omitempty"`
	IsPinned     bool      `json:"is_pinned"`
	MessageCount int       `json:"message_count"`
	LastMessage  string    `json:"last_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MessageRecord struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCallsJSON  string    `json:"tool_calls_json,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func detectConversationChannel(convID, currentChannel string) string {
	if currentChannel != "" && currentChannel != "web" {
		return currentChannel
	}
	if strings.HasPrefix(convID, "conv_telegram_") {
		return "telegram"
	}
	if strings.HasPrefix(convID, "conv_whatsapp_") {
		return "whatsapp"
	}
	if strings.HasPrefix(convID, "conv_discord_") {
		return "discord"
	}
	if strings.HasPrefix(convID, "conv_task_") {
		return "mission"
	}
	if strings.HasPrefix(convID, "conv_system_") {
		return "system"
	}
	if strings.HasPrefix(convID, "conv_webhook_") {
		return "webhook"
	}
	if currentChannel != "" {
		return currentChannel
	}
	return "web"
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")

	query := `
		SELECT c.id, c.agent_id, c.title, COALESCE(c.channel, 'web'), COALESCE(c.is_pinned, 0), c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id) as message_count,
		       COALESCE((SELECT content FROM messages m WHERE m.conversation_id = c.id ORDER BY m.created_at DESC LIMIT 1), '') as last_message
		FROM conversations c
		ORDER BY c.is_pinned DESC, c.updated_at DESC
	`
	var rows *sql.Rows
	var err error

	if agentID != "" {
		query = `
			SELECT c.id, c.agent_id, c.title, COALESCE(c.channel, 'web'), COALESCE(c.is_pinned, 0), c.created_at, c.updated_at,
			       (SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id) as message_count,
			       COALESCE((SELECT content FROM messages m WHERE m.conversation_id = c.id ORDER BY m.created_at DESC LIMIT 1), '') as last_message
			FROM conversations c
			WHERE c.agent_id = ?
			ORDER BY c.is_pinned DESC, c.updated_at DESC
		`
		rows, err = s.memory.DB().SQLDB().QueryContext(r.Context(), query, agentID)
	} else {
		rows, err = s.memory.DB().SQLDB().QueryContext(r.Context(), query)
	}

	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"conversations": []any{}, "count": 0})
		return
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.AgentID, &c.Title, &c.Channel, &c.IsPinned, &c.CreatedAt, &c.UpdatedAt, &c.MessageCount, &c.LastMessage); err == nil {
			c.Channel = detectConversationChannel(c.ID, c.Channel)
			convs = append(convs, c)
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"conversations": convs,
		"count":         len(convs),
	})
}

func generateConversationTitle(prompt string) string {
	clean := strings.TrimSpace(prompt)
	clean = strings.TrimLeft(clean, "#*- \t\r\n")
	if idx := strings.Index(clean, "\n"); idx != -1 {
		clean = strings.TrimSpace(clean[:idx])
	}
	runes := []rune(clean)
	if len(runes) == 0 {
		return "New Session"
	}
	if len(runes) > 42 {
		return string(runes[:42]) + "..."
	}
	return string(runes)
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
		Title   string `json:"title"`
		Channel string `json:"channel"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.AgentID == "" {
		req.AgentID = "agent_system_core"
	}
	if req.Title == "" {
		req.Title = "New Session"
	}
	if req.Channel == "" {
		req.Channel = "web"
	}

	id := "conv_" + uuid.New().String()
	now := time.Now().UTC()

	query := `INSERT INTO conversations (id, agent_id, title, channel, is_pinned, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?)`
	_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, id, req.AgentID, req.Title, req.Channel, now, now)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CREATE_CONV_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, Conversation{
		ID:        id,
		AgentID:   req.AgentID,
		Title:     req.Title,
		Channel:   req.Channel,
		IsPinned:  false,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")

	var c Conversation
	query := `SELECT id, agent_id, title, COALESCE(channel, 'web'), COALESCE(is_pinned, 0), created_at, updated_at FROM conversations WHERE id = ?`
	err := s.memory.DB().SQLDB().QueryRowContext(r.Context(), query, convID).Scan(&c.ID, &c.AgentID, &c.Title, &c.Channel, &c.IsPinned, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "conversation not found")
		return
	}
	c.Channel = detectConversationChannel(c.ID, c.Channel)

	// Fetch messages
	msgQuery := `SELECT id, conversation_id, role, content, COALESCE(tool_calls_json, ''), created_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC`
	rows, err := s.memory.DB().SQLDB().QueryContext(r.Context(), msgQuery, convID)
	var messages []MessageRecord
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m MessageRecord
			if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ToolCallsJSON, &m.CreatedAt); err == nil {
				messages = append(messages, m)
			}
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"conversation": c,
		"messages":     messages,
	})
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")

	_, _ = s.memory.DB().SQLDB().ExecContext(r.Context(), `DELETE FROM messages WHERE conversation_id = ?`, convID)
	_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), `DELETE FROM conversations WHERE id = ?`, convID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")
	var req struct {
		Title    *string `json:"title"`
		IsPinned *bool   `json:"is_pinned"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	now := time.Now().UTC()
	if req.Title != nil && req.IsPinned != nil {
		query := `UPDATE conversations SET title = ?, is_pinned = ?, updated_at = ? WHERE id = ?`
		_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, *req.Title, *req.IsPinned, now, convID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
			return
		}
	} else if req.Title != nil {
		query := `UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`
		_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, *req.Title, now, convID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
			return
		}
	} else if req.IsPinned != nil {
		query := `UPDATE conversations SET is_pinned = ? WHERE id = ?`
		_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, *req.IsPinned, convID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
			return
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (s *Server) handleTogglePinConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")
	var req struct {
		IsPinned bool `json:"is_pinned"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	query := `UPDATE conversations SET is_pinned = ? WHERE id = ?`
	_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, req.IsPinned, convID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"status": "updated", "is_pinned": req.IsPinned})
}

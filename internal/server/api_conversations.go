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
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MessageRecord struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCallsJSON  string    `json:"tool_calls_json,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")

	query := `SELECT id, agent_id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC`
	var rows *sql.Rows
	var err error

	if agentID != "" {
		query = `SELECT id, agent_id, title, created_at, updated_at FROM conversations WHERE agent_id = ? ORDER BY updated_at DESC`
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
		if err := rows.Scan(&c.ID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err == nil {
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

	id := "conv_" + uuid.New().String()
	now := time.Now().UTC()

	query := `INSERT INTO conversations (id, agent_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, id, req.AgentID, req.Title, now, now)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CREATE_CONV_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, Conversation{
		ID:        id,
		AgentID:   req.AgentID,
		Title:     req.Title,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")

	var c Conversation
	query := `SELECT id, agent_id, title, created_at, updated_at FROM conversations WHERE id = ?`
	err := s.memory.DB().SQLDB().QueryRowContext(r.Context(), query, convID).Scan(&c.ID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "conversation not found")
		return
	}

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
		Title string `json:"title"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	query := `UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`
	_, err := s.memory.DB().SQLDB().ExecContext(r.Context(), query, req.Title, time.Now().UTC(), convID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "updated", "title": req.Title})
}

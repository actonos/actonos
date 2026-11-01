package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) flushChatSSE(w http.ResponseWriter, flusher http.Flusher, writeSSE *bool, event string, payload map[string]any) {
	if writeSSE == nil || !*writeSSE {
		return
	}
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(dataBytes)); err != nil {
		*writeSSE = false
		return
	}
	flusher.Flush()
}

func resolveStreamedAssistantContent(tokenBuf, doneContent, responseContent string, toolCalls []llm.ToolCall) string {
	for _, candidate := range []string{responseContent, doneContent, tokenBuf} {
		if text := strings.TrimSpace(candidate); text != "" {
			return text
		}
	}
	if len(toolCalls) > 0 {
		return completedOperationsFallback
	}
	return ""
}

func (s *Server) persistChatAssistantMessage(convID, agentID, content string, toolCalls []llm.ToolCall) {
	if s.memory == nil || strings.TrimSpace(convID) == "" {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		if len(toolCalls) == 0 {
			return
		}
		content = completedOperationsFallback
	}
	toolCallsJSON := ""
	if len(toolCalls) > 0 {
		encoded, err := json.Marshal(toolCalls)
		if err == nil {
			toolCallsJSON = string(encoded)
		}
	}
	now := time.Now().UTC()
	_, _ = s.memory.DB().SQLDB().ExecContext(context.Background(), `
		INSERT INTO messages (id, conversation_id, agent_id, role, content, tool_calls_json, created_at)
		VALUES (?, ?, ?, 'assistant', ?, ?, ?)
	`, "msg_"+uuid.NewString(), convID, agentID, content, toolCallsJSON, now)
	_, _ = s.memory.DB().SQLDB().ExecContext(context.Background(), `
		UPDATE conversations SET updated_at = ? WHERE id = ?
	`, now, convID)
}

func (s *Server) persistStreamedAssistantIfNeeded(
	convID, agentID, tokenBuf, doneContent string,
	toolCalls []llm.ToolCall,
	resp *llm.Response,
	err error,
) {
	if err == nil && resp != nil && (strings.TrimSpace(resp.Content) != "" || len(resp.ToolCalls) > 0) {
		return
	}
	content := resolveStreamedAssistantContent(tokenBuf, doneContent, "", toolCalls)
	if content == "" && err != nil {
		var approvalErr *tools.ApprovalRequiredError
		if !errors.As(err, &approvalErr) {
			content = fmt.Sprintf("Execution error: %v", err)
		}
	}
	s.persistChatAssistantMessage(convID, agentID, content, toolCalls)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if agentID == "" || agentID == "default" {
		agentID = "agent_system_core"
	}

	var req struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
		Stream         bool   `json:"stream"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Message == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "message cannot be empty")
		return
	}

	now := time.Now().UTC()
	convID := req.ConversationID
	var convTitle string

	if s.memory != nil {
		// If no conversation ID provided, automatically create a new session
		if convID == "" {
			convID = "conv_" + uuid.New().String()
			convTitle = generateConversationTitle(req.Message)
			_, _ = s.memory.DB().SQLDB().ExecContext(
				r.Context(),
				`INSERT INTO conversations (id, agent_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
				convID, agentID, convTitle, now, now,
			)
		} else {
			// Check if conversation exists and check its title
			var existingTitle string
			err := s.memory.DB().SQLDB().QueryRowContext(
				r.Context(),
				`SELECT title FROM conversations WHERE id = ?`,
				convID,
			).Scan(&existingTitle)

			if err == sql.ErrNoRows {
				convTitle = generateConversationTitle(req.Message)
				_, _ = s.memory.DB().SQLDB().ExecContext(
					r.Context(),
					`INSERT INTO conversations (id, agent_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
					convID, agentID, convTitle, now, now,
				)
			} else {
				convTitle = existingTitle
				if existingTitle == "New Session" || existingTitle == "New Conversation" || existingTitle == "" {
					convTitle = generateConversationTitle(req.Message)
					_, _ = s.memory.DB().SQLDB().ExecContext(
						r.Context(),
						`UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`,
						convTitle, now, convID,
					)
				} else {
					_, _ = s.memory.DB().SQLDB().ExecContext(
						r.Context(),
						`UPDATE conversations SET updated_at = ? WHERE id = ?`,
						now, convID,
					)
				}
			}
		}

		// Insert user message with agent_id
		userMsgID := "msg_" + uuid.New().String()
		_, insertErr := s.memory.DB().SQLDB().ExecContext(
			r.Context(),
			`INSERT INTO messages (id, conversation_id, agent_id, role, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			userMsgID, convID, agentID, "user", req.Message, now,
		)
		if insertErr == nil && s.embedding != nil {
			_ = s.embedding.EnqueueMessage(context.Background(), userMsgID, agentID, convID)
		}
	}

	// Load short-term Working Memory (recent history) for this conversation
	var history []llm.Message
	if s.memory != nil && convID != "" {
		rows, err := s.memory.DB().SQLDB().QueryContext(r.Context(), `
			SELECT role, content
			FROM messages
			WHERE conversation_id = ? AND role IN ('user', 'assistant')
			ORDER BY created_at DESC
			LIMIT 50
		`, convID)
		if err == nil {
			var reversed []llm.Message
			for rows.Next() {
				var role, content string
				if err := rows.Scan(&role, &content); err == nil {
					reversed = append(reversed, llm.Message{Role: llm.Role(role), Content: content})
				}
			}
			rows.Close()
			// Exclude the last message if it's the current user message
			if len(reversed) > 0 && reversed[0].Role == "user" && reversed[0].Content == req.Message {
				reversed = reversed[1:]
			}
			for i, j := 0, len(reversed)-1; i < len(reversed); i, j = i+1, j-1 {
				history = append(history, reversed[j])
			}
		}
	}

	if req.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			s.respondError(w, http.StatusInternalServerError, "STREAM_UNSUPPORTED", "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		fmt.Fprintf(w, ": ready\n\n")
		flusher.Flush()

		eventChan := make(chan agent.AgentStreamEvent, 64)
		type streamResult struct {
			response *llm.Response
			err      error
		}
		resultCh := make(chan streamResult, 1)
		go func() {
			chatCtx := agent.WithExecutionSource(agent.WithConversationContext(r.Context(), convID), "stream")
			response, err := s.engine.ExecuteStepStreamWithHistory(chatCtx, agentID, req.Message, history, eventChan)
			resultCh <- streamResult{response: response, err: err}
		}()

		var finalContent strings.Builder
		var allToolCalls []llm.ToolCall
		var finalModel string
		var finalTokens int
		var doneContent string
		writeSSE := true

		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
	streamLoop:
		for {
			var ev agent.AgentStreamEvent
			var ok bool
			select {
			case ev, ok = <-eventChan:
				if !ok {
					break streamLoop
				}
			case <-keepalive.C:
				if writeSSE {
					if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
						writeSSE = false
					} else {
						flusher.Flush()
					}
				}
				continue
			case <-r.Context().Done():
				// Keep draining so the engine can finish and persist, even if the
				// browser refreshed or the proxy dropped the SSE socket.
				writeSSE = false
				continue
			}
			ev.ConversationID = convID
			ev.Title = convTitle

			switch ev.Type {
			case agent.EventStreamThought:
				s.flushChatSSE(w, flusher, &writeSSE, "thought", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"thought":         ev.Thought,
				})

			case agent.EventStreamReasoning:
				s.flushChatSSE(w, flusher, &writeSSE, "reasoning", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"reasoning":       ev.Reasoning,
				})

			case agent.EventStreamToken:
				finalContent.WriteString(ev.Content)
				s.flushChatSSE(w, flusher, &writeSSE, "token", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"content":         ev.Content,
				})

			case agent.EventStreamToolCall:
				allToolCalls = append(allToolCalls, llm.ToolCall{
					ID:   ev.ToolCallID,
					Type: "function",
					Function: llm.FunctionCall{
						Name: ev.Tool,
					},
				})
				s.flushChatSSE(w, flusher, &writeSSE, "tool_call", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"tool":            ev.Tool,
					"tool_call_id":    ev.ToolCallID,
					"args":            ev.Args,
				})

			case agent.EventStreamToolResult:
				s.flushChatSSE(w, flusher, &writeSSE, "tool_result", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"tool":            ev.Tool,
					"tool_call_id":    ev.ToolCallID,
					"result":          ev.Result,
					"status":          ev.Status,
					"latency_ms":      ev.LatencyMs,
				})

			case agent.EventStreamTokenReset:
				// The iteration turned out to be a tool-calling turn: its prose was
				// preamble, not the answer. Drop it here too so the persisted message
				// does not mix preamble into the final content.
				finalContent.Reset()
				s.flushChatSSE(w, flusher, &writeSSE, "token_reset", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
				})

			case agent.EventStreamAudit:
				if ev.AuditLog != nil {
					s.flushChatSSE(w, flusher, &writeSSE, "audit", map[string]any{
						"conversation_id": convID,
						"title":           convTitle,
						"audit_log":       ev.AuditLog,
					})
				}

			case agent.EventStreamDone:
				if ev.Model != "" {
					finalModel = ev.Model
				}
				if ev.Usage != nil {
					finalTokens = ev.Usage.TotalTokens
				}
				if ev.Content != "" {
					doneContent = ev.Content
				}
				s.flushChatSSE(w, flusher, &writeSSE, "done", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"content":         ev.Content,
					"tokens_used":     finalTokens,
					"model":           finalModel,
					"timestamp":       time.Now().UTC(),
				})

			case agent.EventStreamError:
				s.flushChatSSE(w, flusher, &writeSSE, "error", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"error":           ev.Error,
				})

			case agent.EventStreamApprovalRequired:
				s.flushChatSSE(w, flusher, &writeSSE, "approval_required", map[string]any{
					"conversation_id": convID,
					"title":           convTitle,
					"tool":            ev.Tool,
					"tool_call_id":    ev.ToolCallID,
					"args":            ev.Args,
					"status":          ev.Status,
					"approval":        ev.Approval,
				})
			}
		}

		result := <-resultCh
		s.persistStreamedAssistantIfNeeded(convID, agentID, finalContent.String(), doneContent, allToolCalls, result.response, result.err)
		return
	}

	// Non-streaming completion
	chatCtx := agent.WithConversationContext(r.Context(), convID)
	resp, err := s.engine.ExecuteStepWithHistory(chatCtx, agentID, req.Message, history)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CHAT_FAILED", err.Error())
		return
	}

	if resp != nil {
		s.persistChatAssistantMessage(convID, agentID, resp.Content, resp.ToolCalls)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"title":           convTitle,
		"content":         resp.Content,
		"role":            "assistant",
		"model":           resp.Model,
		"tool_calls":      resp.ToolCalls,
		"usage":           resp.Usage,
	})
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if agentID == "" || agentID == "default" {
		agentID = agent.DefaultSystemAgentID
	}
	var req struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "message cannot be empty")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "response writer does not support streaming")
		return
	}

	convID := req.ConversationID
	now := time.Now().UTC()
	title := generateConversationTitle(req.Message)
	var history []llm.Message
	if s.memory != nil {
		if convID == "" {
			convID = "conv_" + uuid.NewString()
		}
		rows, queryErr := s.memory.DB().SQLDB().QueryContext(r.Context(), `
			SELECT role, content FROM messages
			WHERE conversation_id = ? AND role IN ('user', 'assistant')
			ORDER BY created_at DESC LIMIT 50
		`, convID)
		if queryErr == nil {
			var reversed []llm.Message
			for rows.Next() {
				var role, content string
				if scanErr := rows.Scan(&role, &content); scanErr == nil {
					reversed = append(reversed, llm.Message{Role: llm.Role(role), Content: content})
				}
			}
			_ = rows.Close()
			for index := len(reversed) - 1; index >= 0; index-- {
				history = append(history, reversed[index])
			}
		}
		_, _ = s.memory.DB().SQLDB().ExecContext(r.Context(), `
			INSERT INTO conversations (id, agent_id, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at
		`, convID, agentID, title, now, now)
		userMsgID := "msg_" + uuid.NewString()
		_, insertErr := s.memory.DB().SQLDB().ExecContext(r.Context(), `
			INSERT INTO messages (id, conversation_id, agent_id, role, content, created_at)
			VALUES (?, ?, ?, 'user', ?, ?)
		`, userMsgID, convID, agentID, req.Message, now)
		if insertErr == nil && s.embedding != nil {
			_ = s.embedding.EnqueueMessage(context.Background(), userMsgID, agentID, convID)
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	events := make(chan agent.AgentStreamEvent, 64)
	type streamResult struct {
		response *llm.Response
		err      error
	}
	resultCh := make(chan streamResult, 1)
	go func() {
		chatCtx := agent.WithExecutionSource(agent.WithConversationContext(r.Context(), convID), "stream")
		response, err := s.engine.ExecuteStepStreamWithHistory(chatCtx, agentID, req.Message, history, events)
		resultCh <- streamResult{response: response, err: err}
	}()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	writeSSE := true
eventLoop:
	for {
		select {
		case event, ok := <-events:
			if !ok {
				break eventLoop
			}
			event.ConversationID = convID
			event.Title = title
			if writeSSE {
				payload, err := json.Marshal(event)
				if err != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
					writeSSE = false
				} else {
					flusher.Flush()
				}
			}
		case <-keepalive.C:
			if writeSSE {
				if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
					writeSSE = false
				} else {
					flusher.Flush()
				}
			}
		case <-r.Context().Done():
			writeSSE = false
		}
	}
	result := <-resultCh
	var doneContent string
	var tokenBuf string
	var streamedTools []llm.ToolCall
	if result.response != nil {
		doneContent = result.response.Content
		streamedTools = result.response.ToolCalls
	}
	s.persistStreamedAssistantIfNeeded(convID, agentID, tokenBuf, doneContent, streamedTools, result.response, result.err)
}

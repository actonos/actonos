package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createAgentRequest struct {
	Name               string                      `json:"name"`
	Description        string                      `json:"description"`
	AvatarIcon         string                      `json:"avatar_icon"`
	Status             agent.AgentStatus           `json:"status,omitempty"`
	ModelConfig        llm.ModelConfig             `json:"model_config"`
	SystemInstructions string                      `json:"system_instructions"`
	AuthorizedTools    []string                    `json:"authorized_tools"`
	ListenChannels     []string                    `json:"listen_channels"`
	HeartbeatConfig    *agent.AgentHeartbeatConfig `json:"heartbeat_config,omitempty"`
	DelegationScope    agent.DelegationScope       `json:"delegation_scope"`
	TriggerRules       []agent.TriggerRule         `json:"trigger_rules"`
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agentMgr.List(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}

func (s *Server) handleListAgentTemplates(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	query := r.URL.Query().Get("q")
	templates := agent.ListTemplates(category, query)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"templates": templates,
		"count":     len(templates),
	})
}

func (s *Server) handleGetAgentTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	if templateID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "template ID is required")
		return
	}
	tmpl, err := agent.GetTemplateByID(templateID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, tmpl)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	status := req.Status
	if status == "" {
		status = agent.StatusActive
	}

	manifest := agent.AgentManifest{
		Name:               req.Name,
		Description:        req.Description,
		AvatarIcon:         req.AvatarIcon,
		Status:             status,
		ModelConfig:        req.ModelConfig,
		SystemInstructions: req.SystemInstructions,
		AuthorizedTools:    req.AuthorizedTools,
		ListenChannels:     req.ListenChannels,
		HeartbeatConfig:    req.HeartbeatConfig,
		DelegationScope:    req.DelegationScope,
		TriggerRules:       req.TriggerRules,
	}

	created, err := s.agentMgr.Create(r.Context(), manifest)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "CREATE_AGENT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	ag, err := s.agentMgr.Get(r.Context(), agentID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "AGENT_NOT_FOUND", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, ag)
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	var req createAgentRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	status := req.Status
	if status == "" {
		if existing, err := s.agentMgr.Get(r.Context(), agentID); err == nil && existing.Status != "" {
			status = existing.Status
		} else {
			status = agent.StatusActive
		}
	}

	manifest := agent.AgentManifest{
		AgentID:            agentID,
		Name:               req.Name,
		Description:        req.Description,
		AvatarIcon:         req.AvatarIcon,
		Status:             status,
		ModelConfig:        req.ModelConfig,
		SystemInstructions: req.SystemInstructions,
		AuthorizedTools:    req.AuthorizedTools,
		ListenChannels:     req.ListenChannels,
		HeartbeatConfig:    req.HeartbeatConfig,
		DelegationScope:    req.DelegationScope,
		TriggerRules:       req.TriggerRules,
	}

	updated, err := s.agentMgr.Update(r.Context(), manifest)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "UPDATE_AGENT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if err := s.agentMgr.Delete(r.Context(), agentID); err != nil {
		if errors.Is(err, agent.ErrProtectedAgent) {
			s.respondError(w, http.StatusForbidden, "PROTECTED_AGENT", "Cannot delete protected system agent")
			return
		}
		s.respondError(w, http.StatusBadRequest, "DELETE_AGENT_FAILED", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStartAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if err := s.agentMgr.Start(r.Context(), agentID); err != nil {
		s.respondError(w, http.StatusBadRequest, "START_AGENT_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "active", "agent_id": agentID})
}

func (s *Server) handleStopAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if err := s.agentMgr.Stop(r.Context(), agentID); err != nil {
		s.respondError(w, http.StatusBadRequest, "STOP_AGENT_FAILED", err.Error())
		return
	}
	if s.engine != nil {
		s.engine.CancelAgentWork(agentID)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "stopped", "agent_id": agentID})
}

const completedOperationsFallback = "Completed requested operations successfully."

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

// Proactive Cron Job Handlers
func (s *Server) handleListCronJobs(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"jobs": []any{}, "count": 0})
		return
	}

	jobs := s.cronSched.ListJobs()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"jobs":  jobs,
		"count": len(jobs),
	})
}

func (s *Server) handleSaveCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondError(w, http.StatusNotImplemented, "CRON_NOT_AVAILABLE", "cron scheduler not configured")
		return
	}

	var req struct {
		ID              string `json:"id"`
		AgentID         string `json:"agent_id"`
		Name            string `json:"name"`
		CronExpr        string `json:"cron_expr"`
		Prompt          string `json:"prompt"`
		TargetChannel   string `json:"target_channel"`
		Channel         string `json:"channel"`
		TargetRecipient string `json:"target_recipient"`
		Recipient       string `json:"recipient"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	targetChan := req.TargetChannel
	if targetChan == "" {
		targetChan = req.Channel
	}
	if targetChan == "" {
		targetChan = "telegram"
	}

	targetRec := req.TargetRecipient
	if targetRec == "" {
		targetRec = req.Recipient
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	job := agent.CronJob{
		ID:              req.ID,
		AgentID:         req.AgentID,
		Name:            req.Name,
		CronExpr:        req.CronExpr,
		Prompt:          req.Prompt,
		TargetChannel:   targetChan,
		TargetRecipient: targetRec,
		Enabled:         enabled,
	}

	if job.ID == "" {
		job.ID = fmt.Sprintf("job_%d", time.Now().Unix())
	}
	if job.Name == "" {
		job.Name = job.ID
	}
	if job.AgentID == "" {
		job.AgentID = "agent_system_core"
	}

	if err := s.cronSched.RegisterJob(job); err != nil {
		s.respondError(w, http.StatusBadRequest, "SCHEDULE_JOB_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "scheduled",
		"job":    job,
	})
}

func (s *Server) handleDeleteCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondError(w, http.StatusNotImplemented, "CRON_NOT_AVAILABLE", "cron scheduler not configured")
		return
	}

	jobID := chi.URLParam(r, "id")
	s.cronSched.RemoveJob(jobID)
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "job_id": jobID})
}

func (s *Server) handleRunCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondError(w, http.StatusNotImplemented, "CRON_NOT_AVAILABLE", "cron scheduler not configured")
		return
	}

	jobID := chi.URLParam(r, "id")
	jobs := s.cronSched.ListJobs()
	var targetJob *agent.CronJob
	for _, j := range jobs {
		if j.ID == jobID {
			targetJob = &j
			break
		}
	}

	if targetJob == nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "cron job not found")
		return
	}

	// Trigger agent execution in background with proactive notification framing
	go func(job agent.CronJob) {
		agentID := job.AgentID
		if agentID == "" || agentID == "default" {
			agentID = "agent_system_core"
		}
		executionPrompt := agent.BuildCronExecutionPrompt(&job)
		resp, err := s.engine.ExecuteStep(context.Background(), agentID, executionPrompt)
		if err != nil {
			return
		}

		targetChan := job.TargetChannel
		if targetChan == "" {
			targetChan = "all"
		}

		if s.channelMgr != nil {
			_ = s.channelMgr.SendMessage(context.Background(), channels.OutboundMessage{
				ChannelID: targetChan,
				Recipient: job.TargetRecipient,
				Content:   fmt.Sprintf("⏰ **[Cron Reminder: %s]**\n\n%s", job.Name, resp.Content),
			})
		}
	}(*targetJob)

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "triggered",
		"message": fmt.Sprintf("Cron task '%s' triggered successfully", targetJob.Name),
	})
}

// Soul & Markdown Memory Handlers
func (s *Server) handleGetSoul(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]string{"soul": "", "content": ""})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		agentID = r.URL.Query().Get("agent_id")
	}
	if agentID == "" {
		agentID = agent.DefaultSystemAgentID
	}

	soul := s.profileMgr.GetAgentSoul(agentID)
	s.respondJSON(w, http.StatusOK, map[string]string{
		"agent_id": agentID,
		"soul":     soul,
		"content":  soul,
	})
}

func (s *Server) handleSaveSoul(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PROFILE_NOT_AVAILABLE", "profile manager not configured")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	var req struct {
		AgentID string `json:"agent_id"`
		Soul    string `json:"soul"`
		Content string `json:"content"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if agentID == "" {
		agentID = req.AgentID
	}
	if agentID == "" {
		agentID = r.URL.Query().Get("agent_id")
	}
	if agentID == "" {
		agentID = agent.DefaultSystemAgentID
	}

	soulContent := req.Soul
	if soulContent == "" {
		soulContent = req.Content
	}

	if err := s.profileMgr.SaveAgentSoul(r.Context(), agentID, soulContent); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_SOUL_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":   "saved",
		"agent_id": agentID,
	})
}

func (s *Server) handleGetMemoryMD(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]string{"memory_md": ""})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		agentID = r.URL.Query().Get("agent_id")
	}
	if agentID == "" {
		agentID = agent.DefaultSystemAgentID
	}

	memMD := s.profileMgr.GetAgentMemoryMD(agentID)
	s.respondJSON(w, http.StatusOK, map[string]string{
		"agent_id":  agentID,
		"memory_md": memMD,
	})
}

func (s *Server) handleClearMemoryMD(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		agentID = r.URL.Query().Get("agent_id")
	}
	if agentID == "" {
		agentID = agent.DefaultSystemAgentID
	}

	if err := s.profileMgr.ClearAgentMemoryMD(r.Context(), agentID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "CLEAR_MEMORY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":   "cleared",
		"agent_id": agentID,
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

func (s *Server) handleListAllCronHistory(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondJSON(w, http.StatusOK, []agent.CronExecutionRecord{})
		return
	}
	history, err := s.cronSched.ListAllExecutionHistory(50)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if history == nil {
		history = []agent.CronExecutionRecord{}
	}
	s.respondJSON(w, http.StatusOK, history)
}

func (s *Server) handleGetCronJobHistory(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondJSON(w, http.StatusOK, []agent.CronExecutionRecord{})
		return
	}
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_ID", "job id is required")
		return
	}
	history, err := s.cronSched.GetExecutionHistory(jobID, 50)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if history == nil {
		history = []agent.CronExecutionRecord{}
	}
	s.respondJSON(w, http.StatusOK, history)
}

// GET /api/agents/{agentID}/insights
func (s *Server) handleListAgentInsights(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"proposals": []agent.SelfImprovementProposal{},
			"count":     0,
		})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	status := r.URL.Query().Get("status")

	items, err := s.reflectionEngine.ListProposals(r.Context(), agentID, status)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "INSIGHTS_QUERY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"proposals": items,
		"count":     len(items),
	})
}

// POST /api/agents/{agentID}/insights/{insightID}/apply
func (s *Server) handleApplyAgentInsight(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "REFLECTION_UNAVAILABLE", "reflection engine is not initialized")
		return
	}

	insightID := chi.URLParam(r, "insightID")
	if err := s.reflectionEngine.ApplyProposal(r.Context(), insightID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPLY_INSIGHT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":     "applied",
		"insight_id": insightID,
	})
}

// POST /api/agents/{agentID}/insights/{insightID}/dismiss
func (s *Server) handleDismissAgentInsight(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "REFLECTION_UNAVAILABLE", "reflection engine is not initialized")
		return
	}

	insightID := chi.URLParam(r, "insightID")
	if err := s.reflectionEngine.DismissProposal(r.Context(), insightID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DISMISS_INSIGHT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":     "dismissed",
		"insight_id": insightID,
	})
}

// POST /api/agents/{agentID}/insights/self-review
func (s *Server) handleTriggerSelfReview(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "REFLECTION_UNAVAILABLE", "reflection engine is not initialized")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	proposals, err := s.reflectionEngine.RunSelfReviewCycle(r.Context(), agentID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "SELF_REVIEW_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "self_review_completed",
		"proposals": proposals,
		"count":     len(proposals),
	})
}

// GET /api/agents/{agentID}/memories
func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	layer := memory.MemoryLayer(r.URL.Query().Get("layer"))
	limit := 50
	if limStr := r.URL.Query().Get("limit"); limStr != "" {
		if l, err := strconv.Atoi(limStr); err == nil && l > 0 {
			limit = l
		}
	}

	mems, err := s.memory.ListMemories(r.Context(), agentID, layer, limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "LIST_MEMORIES_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID,
		"memories": mems,
		"count":    len(mems),
	})
}

// POST /api/agents/{agentID}/memories/{memoryID}/pin
func (s *Server) handlePinMemory(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	memoryID := chi.URLParam(r, "memoryID")
	if err := s.memory.PinMemory(r.Context(), memoryID, true); err != nil {
		s.respondError(w, http.StatusInternalServerError, "PIN_MEMORY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":    "pinned",
		"memory_id": memoryID,
	})
}

// DELETE /api/agents/{agentID}/memories/{memoryID}/pin
func (s *Server) handleUnpinMemory(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	memoryID := chi.URLParam(r, "memoryID")
	if err := s.memory.UnpinMemory(r.Context(), memoryID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "UNPIN_MEMORY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":    "unpinned",
		"memory_id": memoryID,
	})
}

// PUT /api/agents/{agentID}/memories/{memoryID}/importance
func (s *Server) handleSetMemoryImportance(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	memoryID := chi.URLParam(r, "memoryID")
	var req struct {
		Importance string `json:"importance"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	tier := memory.ImportanceTier(req.Importance)
	if tier == "" {
		tier = memory.ImportanceNormal
	}

	if err := s.memory.SetMemoryImportance(r.Context(), memoryID, tier); err != nil {
		s.respondError(w, http.StatusInternalServerError, "UPDATE_IMPORTANCE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":     "updated",
		"memory_id":  memoryID,
		"importance": string(tier),
	})
}


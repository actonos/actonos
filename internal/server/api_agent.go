package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createAgentRequest struct {
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	AvatarIcon          string                `json:"avatar_icon"`
	ModelConfig         llm.ModelConfig       `json:"model_config"`
	SystemInstructions string                `json:"system_instructions"`
	AuthorizedTools     []string              `json:"authorized_tools"`
	ListenChannels      []string              `json:"listen_channels"`
	DelegationScope     agent.DelegationScope `json:"delegation_scope"`
	TriggerRules        []agent.TriggerRule   `json:"trigger_rules"`
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

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	manifest := agent.AgentManifest{
		Name:                req.Name,
		Description:         req.Description,
		AvatarIcon:          req.AvatarIcon,
		ModelConfig:         req.ModelConfig,
		SystemInstructions: req.SystemInstructions,
		AuthorizedTools:     req.AuthorizedTools,
		ListenChannels:      req.ListenChannels,
		DelegationScope:     req.DelegationScope,
		TriggerRules:        req.TriggerRules,
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

	manifest := agent.AgentManifest{
		AgentID:             agentID,
		Name:                req.Name,
		Description:         req.Description,
		AvatarIcon:          req.AvatarIcon,
		ModelConfig:         req.ModelConfig,
		SystemInstructions: req.SystemInstructions,
		AuthorizedTools:     req.AuthorizedTools,
		ListenChannels:      req.ListenChannels,
		DelegationScope:     req.DelegationScope,
		TriggerRules:        req.TriggerRules,
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
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "stopped", "agent_id": agentID})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")

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

	// Persist user message if conversation exists
	if req.ConversationID != "" && s.memory != nil {
		now := time.Now().UTC()
		userMsgID := "msg_" + uuid.New().String()[:12]
		_, _ = s.memory.DB().SQLDB().ExecContext(
			r.Context(),
			`INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
			userMsgID, req.ConversationID, "user", req.Message, now,
		)
		_, _ = s.memory.DB().SQLDB().ExecContext(
			r.Context(),
			`UPDATE conversations SET updated_at = ? WHERE id = ?`,
			now, req.ConversationID,
		)
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

		resp, err := s.engine.ExecuteStep(r.Context(), agentID, req.Message)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return
		}

		// Persist assistant message
		if req.ConversationID != "" && s.memory != nil && resp != nil {
			now := time.Now().UTC()
			asstMsgID := "msg_" + uuid.New().String()[:12]
			toolCallsJSON, _ := json.Marshal(resp.ToolCalls)
			_, _ = s.memory.DB().SQLDB().ExecContext(
				r.Context(),
				`INSERT INTO messages (id, conversation_id, role, content, tool_calls_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
				asstMsgID, req.ConversationID, "assistant", resp.Content, string(toolCallsJSON), now,
			)
		}

		// Stream content in chunk events
		tokenJSON, _ := json.Marshal(map[string]any{"content": resp.Content, "tool_calls": resp.ToolCalls})
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", string(tokenJSON))
		flusher.Flush()

		doneJSON, _ := json.Marshal(map[string]any{
			"tokens_used": resp.Usage.TotalTokens,
			"model":       resp.Model,
			"timestamp":   time.Now().UTC(),
		})
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", string(doneJSON))
		flusher.Flush()
		return
	}

	// Non-streaming completion
	resp, err := s.engine.ExecuteStep(r.Context(), agentID, req.Message)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CHAT_FAILED", err.Error())
		return
	}

	// Persist assistant message
	if req.ConversationID != "" && s.memory != nil && resp != nil {
		now := time.Now().UTC()
		asstMsgID := "msg_" + uuid.New().String()[:12]
		toolCallsJSON, _ := json.Marshal(resp.ToolCalls)
		_, _ = s.memory.DB().SQLDB().ExecContext(
			r.Context(),
			`INSERT INTO messages (id, conversation_id, role, content, tool_calls_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			asstMsgID, req.ConversationID, "assistant", resp.Content, string(toolCallsJSON), now,
		)
	}

	s.respondJSON(w, http.StatusOK, resp)
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

	var job agent.CronJob
	if err := s.decodeJSON(r, &job); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if job.ID == "" {
		job.ID = fmt.Sprintf("job_%d", time.Now().Unix())
	}
	if job.AgentID == "" {
		job.AgentID = "default"
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

	// Trigger agent execution in background or synchronously
	go func(job agent.CronJob) {
		agentID := job.AgentID
		if agentID == "" || agentID == "default" {
			agentID = "agent_system_core"
		}
		resp, err := s.engine.ExecuteStep(context.Background(), agentID, job.Prompt)
		if err != nil {
			return
		}
		if job.TargetChannel == "telegram" && s.tgAdapter != nil && job.TargetRecipient != "" {
			_ = s.tgAdapter.SendMessage(context.Background(), channels.OutboundMessage{
				ChannelID: "telegram",
				Recipient: job.TargetRecipient,
				Content:   fmt.Sprintf("⏰ **[Automated Cron Task: %s]**\n\n%s", job.Name, resp.Content),
			})
		} else if job.TargetChannel == "whatsapp" && s.waAdapter != nil && job.TargetRecipient != "" {
			_ = s.waAdapter.SendMessage(context.Background(), channels.OutboundMessage{
				ChannelID: "whatsapp",
				Recipient: job.TargetRecipient,
				Content:   fmt.Sprintf("⏰ *[Automated Cron Task: %s]*\n\n%s", job.Name, resp.Content),
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
		s.respondJSON(w, http.StatusOK, map[string]string{"soul": ""})
		return
	}

	soul := s.profileMgr.GetSoul()
	s.respondJSON(w, http.StatusOK, map[string]string{"soul": soul})
}

func (s *Server) handleSaveSoul(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PROFILE_NOT_AVAILABLE", "profile manager not configured")
		return
	}

	var req struct {
		Soul string `json:"soul"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.profileMgr.SaveSoul(r.Context(), req.Soul); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_SOUL_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleGetMemoryMD(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]string{"memory_md": ""})
		return
	}

	memMD := s.profileMgr.GetMemoryMD()
	s.respondJSON(w, http.StatusOK, map[string]string{"memory_md": memMD})
}


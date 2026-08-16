package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
	"github.com/go-chi/chi/v5"
)

type createAgentRequest struct {
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	AvatarIcon          string                `json:"avatar_icon"`
	ModelConfig         llm.ModelConfig       `json:"model_config"`
	SystemInstructions string                `json:"system_instructions"`
	AuthorizedTools     []string              `json:"authorized_tools"`
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
		Message string `json:"message"`
		Stream  bool   `json:"stream"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Message == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "message cannot be empty")
		return
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

		// Stream content in chunk events
		tokenJSON, _ := json.Marshal(map[string]string{"content": resp.Content})
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

	s.respondJSON(w, http.StatusOK, resp)
}

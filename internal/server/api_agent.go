package server

import (
	"errors"
	"net/http"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
	"github.com/go-chi/chi/v5"
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

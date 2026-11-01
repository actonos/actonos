package server

import (
	"net/http"

	"github.com/actonos/actonos/internal/agent"
	"github.com/go-chi/chi/v5"
)

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

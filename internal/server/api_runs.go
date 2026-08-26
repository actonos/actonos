package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/actonos/actonos/internal/agent"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListAgentRuns(w http.ResponseWriter, r *http.Request) {
	if s.runStore == nil {
		s.respondError(w, http.StatusNotImplemented, "RUN_STORE_NOT_ENABLED", "agent run store is not configured")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	filter := agent.RunListFilter{
		AgentID: r.URL.Query().Get("agent_id"),
		Source:  r.URL.Query().Get("source"),
		Limit:   limit,
	}
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = agent.RunStatus(status)
	}
	runs, err := s.runStore.ListFiltered(r.Context(), filter)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "RUN_LIST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetAgentRun(w http.ResponseWriter, r *http.Request) {
	if s.runStore == nil {
		s.respondError(w, http.StatusNotImplemented, "RUN_STORE_NOT_ENABLED", "agent run store is not configured")
		return
	}
	run, err := s.runStore.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) {
		s.respondError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
		return
	}
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "RUN_GET_FAILED", err.Error())
		return
	}
	s.annotateRunName(r.Context(), run)
	s.respondJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (s *Server) handleCancelAgentRun(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		s.respondError(w, http.StatusNotImplemented, "ENGINE_NOT_ENABLED", "agent engine is not configured")
		return
	}
	runID := chi.URLParam(r, "id")
	if err := s.engine.CancelRun(r.Context(), runID); err != nil {
		s.respondError(w, http.StatusBadRequest, "RUN_CANCEL_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "cancelled", "id": runID})
}

func (s *Server) handleListRunEvents(w http.ResponseWriter, r *http.Request) {
	if s.runStore == nil {
		s.respondError(w, http.StatusNotImplemented, "RUN_STORE_NOT_ENABLED", "agent run store is not configured")
		return
	}
	events, err := s.runStore.Events(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "RUN_EVENTS_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"events": events})
}

package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListAgentRuns(w http.ResponseWriter, r *http.Request) {
	if s.runStore == nil {
		s.respondError(w, http.StatusNotImplemented, "RUN_STORE_NOT_ENABLED", "agent run store is not configured")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.runStore.List(r.Context(), limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "RUN_LIST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"runs": runs})
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

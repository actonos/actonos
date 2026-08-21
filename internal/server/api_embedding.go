package server

import (
	"net/http"
)

func (s *Server) handleGetEmbeddingStatus(w http.ResponseWriter, r *http.Request) {
	if s.embedding == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
		return
	}
	status, err := s.embedding.Status(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "EMBEDDING_STATUS_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, status)
}

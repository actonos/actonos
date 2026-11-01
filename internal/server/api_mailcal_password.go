package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// PUT /api/mailcal/accounts/{id}/password  {password}
//
// The password goes straight into the vault; the answer never echoes it and the account
// record only ever holds the name of the vault key.
func (s *Server) handleMailcalPutPassword(w http.ResponseWriter, r *http.Request) {
	if s.mailcalCreds == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MAILCAL_CREDENTIALS_UNAVAILABLE", "the vault is not available")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if err := s.mailcalCreds.SetPassword(r.Context(), chi.URLParam(r, "id"), req.Password); err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]bool{"configured": true})
}

package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/go-chi/chi/v5"
)

func (s *Server) mailcalOrFail(w http.ResponseWriter) *mailcal.Service {
	if s.mailcal == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MAILCAL_UNAVAILABLE", "mail and calendar support is not initialized")
		return nil
	}
	return s.mailcal
}

func (s *Server) respondMailcalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mailcal.ErrAccountNotFound), errors.Is(err, mailcal.ErrDraftNotFound):
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, mailcal.ErrInvalidAccount), errors.Is(err, mailcal.ErrInvalidDraft):
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	default:
		s.respondError(w, http.StatusInternalServerError, "MAILCAL_FAILED", err.Error())
	}
}

// GET /api/mailcal/status
func (s *Server) handleMailcalStatus(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	accounts, err := svc.Accounts().List(r.Context())
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"providers": len(svc.Providers()),
		"accounts":  len(accounts),
		"enabled":   svc.Settings().Enabled(r.Context()),
	})
}

// GET /api/mailcal/providers
func (s *Server) handleMailcalProviders(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"providers": svc.Providers()})
}

// GET /api/mailcal/accounts
func (s *Server) handleMailcalListAccounts(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	accounts, err := svc.Accounts().List(r.Context())
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	if accounts == nil {
		accounts = []mailcal.Account{}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "count": len(accounts)})
}

type mailcalAccountRequest struct {
	Label    string          `json:"label"`
	PluginID string          `json:"plugin_id"`
	Kind     string          `json:"kind"`
	Email    string          `json:"email"`
	Config   map[string]any  `json:"config"`
	Auth     mailcal.AuthRef `json:"auth"`
}

// POST /api/mailcal/accounts
func (s *Server) handleMailcalCreateAccount(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	var req mailcalAccountRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := svc.ValidateAccountAgainstProvider(req.PluginID, req.Kind); err != nil {
		s.respondMailcalError(w, err)
		return
	}
	acct, err := svc.Accounts().Create(r.Context(), mailcal.NewAccount{
		Label: req.Label, PluginID: req.PluginID, Kind: req.Kind, Email: req.Email, Config: req.Config, Auth: req.Auth,
	})
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	if err := svc.SyncTools(r.Context()); err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusCreated, acct)
}

// GET /api/mailcal/accounts/{id}
func (s *Server) handleMailcalGetAccount(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	acct, err := svc.Accounts().Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, acct)
}

// PUT /api/mailcal/accounts/{id}
func (s *Server) handleMailcalUpdateAccount(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	var req mailcalAccountRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	acct, err := svc.Accounts().Update(r.Context(), chi.URLParam(r, "id"), req.Label, req.Email, req.Config)
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, acct)
}

// DELETE /api/mailcal/accounts/{id}
func (s *Server) handleMailcalDeleteAccount(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	id := chi.URLParam(r, "id")
	if err := svc.Accounts().Delete(r.Context(), id); err != nil {
		s.respondMailcalError(w, err)
		return
	}
	if s.mailcalOAuth != nil {
		if err := s.mailcalOAuth.Forget(r.Context(), id); err != nil {
			slog.Warn("mailcal: could not delete the stored token of a removed account", "account", id, "error", err)
		}
	}
	if err := svc.SyncTools(r.Context()); err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// POST /api/mailcal/accounts/{id}/test
func (s *Server) handleMailcalTestAccount(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	res, err := svc.TestAccount(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, res)
}

// GET /api/mailcal/drafts/{id}
func (s *Server) handleMailcalGetDraft(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	d, err := svc.Drafts().Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, d)
}

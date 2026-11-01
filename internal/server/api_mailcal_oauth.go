package server

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/go-chi/chi/v5"
)

const mailcalOAuthCookie = "acton_mailcal_oauth"

func (s *Server) mailcalOAuthOrFail(w http.ResponseWriter) *mailcal.OAuthFlow {
	if s.mailcalOAuth == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MAILCAL_OAUTH_UNAVAILABLE", "sign-in is not initialized")
		return nil
	}
	return s.mailcalOAuth
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// POST /api/mailcal/accounts/{id}/oauth/start  (authenticated)
func (s *Server) handleMailcalOAuthStart(w http.ResponseWriter, r *http.Request) {
	flow := s.mailcalOAuthOrFail(w)
	if flow == nil {
		return
	}
	redirect := requestOrigin(r) + "/api/mailcal/oauth/callback"
	authURL, nonce, err := flow.Start(r.Context(), chi.URLParam(r, "id"), redirect)
	if err != nil {
		if errors.Is(err, mailcal.ErrOAuthNotConfigured) {
			s.respondError(w, http.StatusPreconditionFailed, "OAUTH_CLIENT_MISSING", err.Error())
			return
		}
		s.respondMailcalError(w, err)
		return
	}
	// SameSite=Lax is sent on the top-level GET that brings the user back from the provider;
	// the session cookie is SameSite=Strict and is not, which is why the callback is public.
	http.SetCookie(w, &http.Cookie{Name: mailcalOAuthCookie, Value: nonce, Path: "/api/mailcal/oauth", MaxAge: 600,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestOrigin(r)[:5] == "https"})
	s.respondJSON(w, http.StatusOK, map[string]string{"auth_url": authURL, "redirect_uri": redirect})
}

// GET /api/mailcal/oauth/callback  (PUBLIC: protected by single-use state, PKCE and the nonce cookie)
func (s *Server) handleMailcalOAuthCallback(w http.ResponseWriter, r *http.Request) {
	flow := s.mailcalOAuth
	back := func(result string) {
		http.SetCookie(w, &http.Cookie{Name: mailcalOAuthCookie, Value: "", Path: "/api/mailcal/oauth", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, "/mailcal?"+url.Values{"oauth": {result}}.Encode(), http.StatusFound)
	}
	if flow == nil || r.URL.Query().Get("error") != "" {
		back("denied")
		return
	}
	nonce := ""
	if c, err := r.Cookie(mailcalOAuthCookie); err == nil {
		nonce = c.Value
	}
	if _, err := flow.Complete(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), nonce); err != nil {
		switch {
		case errors.Is(err, mailcal.ErrOAuthState):
			back("state")
		case errors.Is(err, mailcal.ErrNoRefreshToken):
			back("no_refresh_token")
		default:
			back("failed") // details stay in the server log, never in the URL
		}
		return
	}
	back("ok")
}

// PUT /api/mailcal/oauth/clients/{provider}  {client_id, client_secret}
func (s *Server) handleMailcalPutOAuthClient(w http.ResponseWriter, r *http.Request) {
	if s.mailcalClients == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MAILCAL_OAUTH_UNAVAILABLE", "the vault is not available")
		return
	}
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := s.mailcalClients.Set(r.Context(), chi.URLParam(r, "provider"), req.ClientID, req.ClientSecret); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]bool{"configured": true})
}

// GET /api/mailcal/oauth/clients/{provider}  (never returns the id or secret)
func (s *Server) handleMailcalGetOAuthClient(w http.ResponseWriter, r *http.Request) {
	configured := s.mailcalClients != nil && s.mailcalClients.Configured(r.Context(), chi.URLParam(r, "provider"))
	s.respondJSON(w, http.StatusOK, map[string]bool{"configured": configured})
}

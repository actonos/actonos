package server

import (
	"net/http"

	"github.com/actonos/actonos/internal/auth"
	"github.com/go-chi/chi/v5"
)

type IntegrationInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	Connected   bool   `json:"connected"`
	RiskLevel   string `json:"risk_level"`
	Description string `json:"description"`
}

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	// Standard SaaS Connectors from Section 3 & 11 of the specification
	integrations := []IntegrationInfo{
		{
			ID:          "google_workspace",
			Name:        "Google Workspace (Gmail, Drive, Calendar)",
			Category:    "Productivity",
			Icon:        "mail",
			Connected:   false,
			RiskLevel:   "Medium",
			Description: "Access emails, schedule calendar events, and read/write Google Docs & Drive files.",
		},
		{
			ID:          "notion",
			Name:        "Notion Workspace",
			Category:    "Knowledge",
			Icon:        "book-open",
			Connected:   false,
			RiskLevel:   "Medium",
			Description: "Search documents, create database entries, and append meeting notes.",
		},
		{
			ID:          "github",
			Name:        "GitHub",
			Category:    "Development",
			Icon:        "github",
			Connected:   false,
			RiskLevel:   "High",
			Description: "Inspect repositories, file issues, create pull requests, and review code.",
		},
		{
			ID:          "slack",
			Name:        "Slack",
			Category:    "Messaging",
			Icon:        "message-circle",
			Connected:   false,
			RiskLevel:   "Medium",
			Description: "Send channel messages, read mentions, and dispatch bot notifications.",
		},
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"integrations": integrations,
		"count":        len(integrations),
	})
}

func (s *Server) handleGetAuthURL(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	cfg := auth.OAuthProviderConfig{
		ProviderID:   provider,
		ClientID:     "actonos_demo_client",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"openid", "email", "profile"},
	}

	stateStore := auth.NewStateStore(0)
	engine := auth.NewOAuthEngine(stateStore)

	authURL, state, _, err := engine.BuildAuthURL(cfg, "http://localhost:8080/api/auth/callback", cfg.Scopes)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "AUTH_URL_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"provider": provider,
		"auth_url": authURL,
		"state":    state,
	})
}

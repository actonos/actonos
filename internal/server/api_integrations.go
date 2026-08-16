package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

var (
	integrationsMu sync.RWMutex
	connectedMap   = map[string]bool{
		"google_workspace": false,
		"notion":           false,
		"github":           false,
		"slack":            false,
	}
)

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	integrationsMu.RLock()
	defer integrationsMu.RUnlock()

	integrations := []IntegrationInfo{
		{
			ID:          "google_workspace",
			Name:        "Google Workspace (Gmail, Drive, Calendar)",
			Category:    "Productivity",
			Icon:        "mail",
			Connected:   connectedMap["google_workspace"],
			RiskLevel:   "Medium",
			Description: "Access emails, schedule calendar events, and read/write Google Docs & Drive files.",
		},
		{
			ID:          "notion",
			Name:        "Notion Workspace",
			Category:    "Knowledge",
			Icon:        "book-open",
			Connected:   connectedMap["notion"],
			RiskLevel:   "Medium",
			Description: "Search documents, create database entries, and append meeting notes.",
		},
		{
			ID:          "github",
			Name:        "GitHub",
			Category:    "Development",
			Icon:        "github",
			Connected:   connectedMap["github"],
			RiskLevel:   "High",
			Description: "Inspect repositories, file issues, create pull requests, and review code.",
		},
		{
			ID:          "slack",
			Name:        "Slack",
			Category:    "Messaging",
			Icon:        "message-circle",
			Connected:   connectedMap["slack"],
			RiskLevel:   "Medium",
			Description: "Send channel messages, read mentions, and dispatch bot notifications.",
		},
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"integrations": integrations,
		"count":        len(integrations),
	})
}

func (s *Server) handleToggleIntegration(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	integrationsMu.Lock()
	connectedMap[provider] = !connectedMap[provider]
	newState := connectedMap[provider]
	integrationsMu.Unlock()

	s.respondJSON(w, http.StatusOK, map[string]any{
		"provider":  provider,
		"connected": newState,
	})
}

func (s *Server) handleGetAuthURL(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	stateStore := auth.NewStateStore(10 * time.Minute)
	oauthEngine := auth.NewOAuthEngine(stateStore)

	cfg := auth.OAuthProviderConfig{
		ProviderID: provider,
		AuthURL:    "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:   "https://oauth2.googleapis.com/token",
		ClientID:   "actonos-client-app",
		Scopes:     []string{"read", "write"},
	}

	authURL, state, _, err := oauthEngine.BuildAuthURL(cfg, "http://localhost:8080/api/auth/callback", []string{"read", "write"})
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

// Channels & Inbound Webhooks
type ChannelConfigResponse struct {
	TelegramEnabled bool   `json:"telegram_enabled"`
	TelegramBot     string `json:"telegram_bot"`
	DiscordEnabled  bool   `json:"discord_enabled"`
	DiscordBot      string `json:"discord_bot"`
	WebhookSecret   string `json:"webhook_secret"`
	WebhookURL      string `json:"webhook_url"`
}

func (s *Server) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	configDir := "./data/config"
	readKey := func(filename string) string {
		data, err := os.ReadFile(filepath.Join(configDir, filename))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	tg := readKey("telegram.token")
	dc := readKey("discord.token")
	wh := readKey("webhook.secret")
	if wh == "" {
		wh = "acton_sec_89fa2bc4d1"
	}

	s.respondJSON(w, http.StatusOK, ChannelConfigResponse{
		TelegramEnabled: tg != "",
		TelegramBot:     maskKey(tg),
		DiscordEnabled:  dc != "",
		DiscordBot:      maskKey(dc),
		WebhookSecret:   wh,
		WebhookURL:      "/api/webhooks/inbound",
	})
}

func (s *Server) handleSaveChannels(w http.ResponseWriter, r *http.Request) {
	configDir := "./data/config"
	_ = os.MkdirAll(configDir, 0755)

	var req struct {
		TelegramToken string `json:"telegram_token,omitempty"`
		DiscordToken  string `json:"discord_token,omitempty"`
		WebhookSecret string `json:"webhook_secret,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.TelegramToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(req.TelegramToken)), 0600)
	}
	if req.DiscordToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "discord.token"), []byte(strings.TrimSpace(req.DiscordToken)), 0600)
	}
	if req.WebhookSecret != "" {
		_ = os.WriteFile(filepath.Join(configDir, "webhook.secret"), []byte(strings.TrimSpace(req.WebhookSecret)), 0600)
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

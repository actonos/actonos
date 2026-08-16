package server

import (
	"encoding/json"
	"io"
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

// ChannelAccount represents a single connected account for a channel type.
type ChannelAccount struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Token   string `json:"token,omitempty"`
	PhoneID string `json:"phone_id,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ChannelConfigResponse returns channel configurations with multi-account support.
type ChannelConfigResponse struct {
	Telegram      []ChannelAccount `json:"telegram"`
	Discord       []ChannelAccount `json:"discord"`
	WhatsApp      []ChannelAccount `json:"whatsapp"`
	WebhookSecret string           `json:"webhook_secret"`
	WebhookURL    string           `json:"webhook_url"`
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

	// Load multi-account JSON if available, fallback to single-token files
	tgAccounts := loadChannelAccounts(configDir, "telegram")
	if len(tgAccounts) == 0 {
		if tg := readKey("telegram.token"); tg != "" {
			tgAccounts = []ChannelAccount{{
				ID: "default", Label: "Primary Bot", Token: maskKey(tg), Enabled: true,
			}}
		}
	}

	dcAccounts := loadChannelAccounts(configDir, "discord")
	if len(dcAccounts) == 0 {
		if dc := readKey("discord.token"); dc != "" {
			dcAccounts = []ChannelAccount{{
				ID: "default", Label: "Primary Bot", Token: maskKey(dc), Enabled: true,
			}}
		}
	}

	waAccounts := loadChannelAccounts(configDir, "whatsapp")
	if len(waAccounts) == 0 {
		waToken := readKey("whatsapp.token")
		waPhone := readKey("whatsapp.phone_id")
		if waToken != "" && waPhone != "" {
			waAccounts = []ChannelAccount{{
				ID: "default", Label: "Primary", Token: maskKey(waToken),
				PhoneID: waPhone, Enabled: true,
			}}
		}
	}

	wh := readKey("webhook.secret")
	if wh == "" {
		wh = "acton_sec_89fa2bc4d1"
	}

	s.respondJSON(w, http.StatusOK, ChannelConfigResponse{
		Telegram:      tgAccounts,
		Discord:       dcAccounts,
		WhatsApp:      waAccounts,
		WebhookSecret: wh,
		WebhookURL:    "/api/webhooks/inbound",
	})
}

// loadChannelAccounts reads the multi-account JSON file for a channel type.
func loadChannelAccounts(configDir, channelType string) []ChannelAccount {
	data, err := os.ReadFile(filepath.Join(configDir, channelType+"_accounts.json"))
	if err != nil {
		return nil
	}
	var accounts []ChannelAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil
	}
	// Mask tokens for API responses
	for i := range accounts {
		accounts[i].Token = maskKey(accounts[i].Token)
	}
	return accounts
}

// saveChannelAccounts writes the multi-account JSON file for a channel type.
func saveChannelAccounts(configDir, channelType string, accounts []ChannelAccount) error {
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, channelType+"_accounts.json"), data, 0600)
}

func (s *Server) handleSaveChannels(w http.ResponseWriter, r *http.Request) {
	configDir := "./data/config"
	_ = os.MkdirAll(configDir, 0755)

	var req struct {
		// Legacy single-token fields (backward compatible)
		TelegramToken string `json:"telegram_token,omitempty"`
		DiscordToken  string `json:"discord_token,omitempty"`
		WhatsAppToken string `json:"whatsapp_token,omitempty"`
		WhatsAppPhone string `json:"whatsapp_phone_id,omitempty"`
		WebhookSecret string `json:"webhook_secret,omitempty"`
		// Multi-account fields
		TelegramAccounts []ChannelAccount `json:"telegram_accounts,omitempty"`
		DiscordAccounts  []ChannelAccount `json:"discord_accounts,omitempty"`
		WhatsAppAccounts []ChannelAccount `json:"whatsapp_accounts,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Handle multi-account saves
	if len(req.TelegramAccounts) > 0 {
		_ = saveChannelAccounts(configDir, "telegram", req.TelegramAccounts)
		// Also write first enabled token for legacy adapter compatibility
		for _, acc := range req.TelegramAccounts {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				if s.tgAdapter != nil {
					s.tgAdapter.UpdateToken(strings.TrimSpace(acc.Token))
					_ = s.tgAdapter.Start(r.Context())
				}
				break
			}
		}
	}
	if len(req.DiscordAccounts) > 0 {
		_ = saveChannelAccounts(configDir, "discord", req.DiscordAccounts)
		for _, acc := range req.DiscordAccounts {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "discord.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				break
			}
		}
	}
	if len(req.WhatsAppAccounts) > 0 {
		_ = saveChannelAccounts(configDir, "whatsapp", req.WhatsAppAccounts)
		for _, acc := range req.WhatsAppAccounts {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "whatsapp.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				if acc.PhoneID != "" {
					_ = os.WriteFile(filepath.Join(configDir, "whatsapp.phone_id"), []byte(strings.TrimSpace(acc.PhoneID)), 0600)
				}
				break
			}
		}
	}

	// Legacy single-token save (backward compat)
	if req.TelegramToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(req.TelegramToken)), 0600)
		if s.tgAdapter != nil {
			s.tgAdapter.UpdateToken(strings.TrimSpace(req.TelegramToken))
			_ = s.tgAdapter.Start(r.Context())
		}
	}
	if req.DiscordToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "discord.token"), []byte(strings.TrimSpace(req.DiscordToken)), 0600)
	}
	if req.WhatsAppToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "whatsapp.token"), []byte(strings.TrimSpace(req.WhatsAppToken)), 0600)
	}
	if req.WhatsAppPhone != "" {
		_ = os.WriteFile(filepath.Join(configDir, "whatsapp.phone_id"), []byte(strings.TrimSpace(req.WhatsAppPhone)), 0600)
	}
	if req.WebhookSecret != "" {
		_ = os.WriteFile(filepath.Join(configDir, "webhook.secret"), []byte(strings.TrimSpace(req.WebhookSecret)), 0600)
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// Channel Pairing Handlers
func (s *Server) handleGeneratePairingCode(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}

	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil || req.ChannelID == "" {
		req.ChannelID = "telegram"
	}

	code, err := s.pairingMgr.GeneratePairingCode(req.ChannelID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CODE_GEN_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"code":       code,
		"channel_id": req.ChannelID,
		"expires_in": 600, // 10 minutes
	})
}

func (s *Server) handleVerifyPairingCode(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}

	var req struct {
		ChannelID  string `json:"channel_id"`
		Code       string `json:"code"`
		SenderID   string `json:"sender_id"`
		SenderName string `json:"sender_name"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	ok, err := s.pairingMgr.ValidateAndPair(req.ChannelID, req.Code, req.SenderID, req.SenderName)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "PAIRING_FAILED", err.Error())
		return
	}

	if !ok {
		s.respondError(w, http.StatusBadRequest, "INVALID_CODE", "pairing code is invalid or expired")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "paired",
		"sender": req.SenderID,
	})
}

func (s *Server) handleListAuthorizations(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"users": []any{}, "count": 0})
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	users := s.pairingMgr.ListAuthorized(channelID)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"users": users,
		"count": len(users),
	})
}

func (s *Server) handleRevokeAuthorization(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}

	var req struct {
		ChannelID string `json:"channel_id"`
		SenderID  string `json:"sender_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.pairingMgr.RevokeUser(req.ChannelID, req.SenderID); err != nil {
		s.respondError(w, http.StatusBadRequest, "REVOKE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// WhatsApp Webhook Handlers
func (s *Server) handleWhatsAppVerifyWebhook(w http.ResponseWriter, r *http.Request) {
	if s.waAdapter == nil {
		http.Error(w, "whatsapp adapter not configured", http.StatusNotImplemented)
		return
	}

	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if res, ok := s.waAdapter.VerifyWebhook(mode, token, challenge); ok {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(res))
		return
	}

	http.Error(w, "forbidden", http.StatusForbidden)
}

func (s *Server) handleWhatsAppInboundWebhook(w http.ResponseWriter, r *http.Request) {
	if s.waAdapter == nil {
		http.Error(w, "whatsapp adapter not configured", http.StatusNotImplemented)
		return
	}

	rawData, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_BODY_FAILED", err.Error())
		return
	}
	defer r.Body.Close()

	if err := s.waAdapter.HandleInboundPayload(r.Context(), rawData); err != nil {
		s.respondError(w, http.StatusBadRequest, "PAYLOAD_PROCESSING_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "received"})
}


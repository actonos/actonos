package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/actonos/actonos/internal/channels"
)

// GET /api/integrations/channels
func (s *Server) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	secret, _ := os.ReadFile(filepath.Join(configDir, "webhook.secret"))

	tgAccounts := loadChannelAccounts(configDir, "telegram")
	dcAccounts := loadChannelAccounts(configDir, "discord")
	waAccounts := loadChannelAccounts(configDir, "whatsapp")

	s.respondJSON(w, http.StatusOK, map[string]any{
		"telegram":       tgAccounts,
		"discord":        dcAccounts,
		"whatsapp":       waAccounts,
		"webhook_secret": string(secret),
		"webhook_url":    "/api/webhooks/whatsapp",
	})
}

// GET /api/integrations/channels/accounts
func (s *Server) handleListAllChannelAccounts(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	var all []channels.ChannelAccount

	tg := loadChannelAccounts(configDir, "telegram")
	dc := loadChannelAccounts(configDir, "discord")
	wa := loadChannelAccounts(configDir, "whatsapp")

	all = append(all, tg...)
	all = append(all, dc...)
	all = append(all, wa...)

	var statuses map[string]channels.AccountStatus
	if s.channelMgr != nil {
		statuses = s.channelMgr.GetAccountStatuses()
	}
	type accountWithStatus struct {
		channels.ChannelAccount
		Status *channels.AccountStatus `json:"status,omitempty"`
	}
	enriched := make([]accountWithStatus, 0, len(all))
	for _, acc := range all {
		entry := accountWithStatus{ChannelAccount: acc}
		if st, ok := statuses[acc.ID]; ok {
			stCopy := st
			entry.Status = &stCopy
		}
		enriched = append(enriched, entry)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"accounts": enriched,
		"count":    len(enriched),
	})
}

func loadChannelAccounts(configDir, channelType string) []channels.ChannelAccount {
	data, err := os.ReadFile(filepath.Join(configDir, channelType+"_accounts.json"))
	if err != nil {
		return nil
	}
	var accounts []channels.ChannelAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil
	}
	for i := range accounts {
		accounts[i].Channel = channelType
		if accounts[i].Token != "" {
			accounts[i].Token = maskKey(accounts[i].Token)
		}
	}
	return accounts
}

func saveChannelAccounts(configDir, channelType string, accounts []channels.ChannelAccount) error {
	existing := loadRawChannelAccounts(configDir, channelType)
	existingMap := make(map[string]string)
	for _, e := range existing {
		existingMap[e.ID] = e.Token
	}

	for i := range accounts {
		accounts[i].Channel = channelType
		if strings.Contains(accounts[i].Token, "•") || strings.Contains(accounts[i].Token, "...") {
			if realTok, ok := existingMap[accounts[i].ID]; ok && realTok != "" {
				accounts[i].Token = realTok
			}
		}
	}

	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, channelType+"_accounts.json"), data, 0600)
}

func loadRawChannelAccounts(configDir, channelType string) []channels.ChannelAccount {
	data, err := os.ReadFile(filepath.Join(configDir, channelType+"_accounts.json"))
	if err != nil {
		return nil
	}
	var accounts []channels.ChannelAccount
	_ = json.Unmarshal(data, &accounts)
	return accounts
}

// POST /api/integrations/channels
func (s *Server) handleSaveChannels(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)

	var req struct {
		TelegramToken    string                    `json:"telegram_token,omitempty"`
		DiscordToken     string                    `json:"discord_token,omitempty"`
		WhatsAppToken    string                    `json:"whatsapp_token,omitempty"`
		WhatsAppPhone    string                    `json:"whatsapp_phone_id,omitempty"`
		WebhookSecret    string                    `json:"webhook_secret,omitempty"`
		TelegramAccounts []channels.ChannelAccount `json:"telegram_accounts,omitempty"`
		DiscordAccounts  []channels.ChannelAccount `json:"discord_accounts,omitempty"`
		WhatsAppAccounts []channels.ChannelAccount `json:"whatsapp_accounts,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	var allAccounts []channels.ChannelAccount

	if req.TelegramAccounts != nil {
		_ = saveChannelAccounts(configDir, "telegram", req.TelegramAccounts)
		rawTg := loadRawChannelAccounts(configDir, "telegram")
		allAccounts = append(allAccounts, rawTg...)
		foundActive := false
		for _, acc := range rawTg {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				foundActive = true
				break
			}
		}
		if !foundActive {
			_ = os.Remove(filepath.Join(configDir, "telegram.token"))
		}
	} else {
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, "telegram")...)
	}

	if req.DiscordAccounts != nil {
		_ = saveChannelAccounts(configDir, "discord", req.DiscordAccounts)
		rawDc := loadRawChannelAccounts(configDir, "discord")
		allAccounts = append(allAccounts, rawDc...)
		foundActive := false
		for _, acc := range rawDc {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "discord.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				foundActive = true
				break
			}
		}
		if !foundActive {
			_ = os.Remove(filepath.Join(configDir, "discord.token"))
		}
	} else {
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, "discord")...)
	}

	if req.WhatsAppAccounts != nil {
		_ = saveChannelAccounts(configDir, "whatsapp", req.WhatsAppAccounts)
		rawWa := loadRawChannelAccounts(configDir, "whatsapp")
		allAccounts = append(allAccounts, rawWa...)
		foundActive := false
		for _, acc := range rawWa {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "whatsapp.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				_ = os.WriteFile(filepath.Join(configDir, "whatsapp.phone_id"), []byte(strings.TrimSpace(acc.PhoneID)), 0600)
				foundActive = true
				break
			}
		}
		if !foundActive {
			_ = os.Remove(filepath.Join(configDir, "whatsapp.token"))
			_ = os.Remove(filepath.Join(configDir, "whatsapp.phone_id"))
		}
	} else {
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, "whatsapp")...)
	}

	if req.TelegramToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(req.TelegramToken)), 0600)
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

	if s.channelMgr != nil {
		_ = s.channelMgr.SyncAccounts(context.Background(), allAccounts)
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

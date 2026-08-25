package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/plugin"
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
	all := loadAllChannelAccounts(configDir)
	if s.pluginMgr != nil {
		all = plugin.MergeChannelAccounts(plugin.AccountsFromPlugins(s.pluginMgr.ListPlugins()), all)
	}
	if s.pairingMgr != nil {
		policies := s.pairingMgr.ListPolicies()
		for i := range all {
			if required, ok := policies[strings.ToLower(all[i].Channel)]; ok {
				all[i].RequiresPairing = required
			}
		}
	}

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

func listStoredChannelTypes(configDir string) []string {
	matches, err := filepath.Glob(filepath.Join(configDir, "*_accounts.json"))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		ch := strings.TrimSuffix(base, "_accounts.json")
		if ch == "" || ch == base {
			continue
		}
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
}

func loadAllChannelAccounts(configDir string) []channels.ChannelAccount {
	var all []channels.ChannelAccount
	for _, ch := range listStoredChannelTypes(configDir) {
		all = append(all, loadChannelAccounts(configDir, ch)...)
	}
	return all
}

// POST /api/integrations/channels
func (s *Server) handleSaveChannels(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)

	var raw map[string]json.RawMessage
	if err := s.decodeJSON(r, &raw); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	saved := map[string]bool{}
	var allAccounts []channels.ChannelAccount

	for key, val := range raw {
		if key == "webhook_secret" {
			var secret string
			if err := json.Unmarshal(val, &secret); err == nil && strings.TrimSpace(secret) != "" {
				_ = os.WriteFile(filepath.Join(configDir, "webhook.secret"), []byte(strings.TrimSpace(secret)), 0600)
			}
			continue
		}
		channelType, ok := strings.CutSuffix(key, "_accounts")
		if !ok || channelType == "" {
			continue
		}
		var accounts []channels.ChannelAccount
		if err := json.Unmarshal(val, &accounts); err != nil {
			s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		_ = saveChannelAccounts(configDir, channelType, accounts)
		loaded := loadRawChannelAccounts(configDir, channelType)
		allAccounts = append(allAccounts, loaded...)
		saved[channelType] = true
	}

	for _, ch := range listStoredChannelTypes(configDir) {
		if saved[ch] {
			continue
		}
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, ch)...)
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
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	req.ChannelID = strings.ToLower(strings.TrimSpace(req.ChannelID))
	if req.ChannelID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "channel_id is required")
		return
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
		"expires_at": time.Now().UTC().Add(10 * time.Minute),
	})
}

func (s *Server) handleListPairingCodes(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"codes": []any{}, "count": 0})
		return
	}
	codes := s.pairingMgr.ListActiveCodes()
	s.respondJSON(w, http.StatusOK, map[string]any{"codes": codes, "count": len(codes)})
}

func (s *Server) handleListPendingPairing(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"pending": []any{}, "count": 0})
		return
	}
	pending := s.pairingMgr.ListPending()
	s.respondJSON(w, http.StatusOK, map[string]any{"pending": pending, "count": len(pending)})
}

func (s *Server) handleGetPairingPolicies(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"policies": map[string]bool{}})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"policies": s.pairingMgr.ListPolicies()})
}

func (s *Server) handleSetPairingPolicy(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}
	var req struct {
		ChannelID string `json:"channel_id"`
		Required  bool   `json:"required"`
	}
	if err := s.decodeJSON(r, &req); err != nil || strings.TrimSpace(req.ChannelID) == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "channel_id is required")
		return
	}
	if err := s.pairingMgr.SetChannelRequiresPairing(req.ChannelID, req.Required); err != nil {
		s.respondError(w, http.StatusInternalServerError, "POLICY_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":     "saved",
		"channel_id": req.ChannelID,
		"required":   req.Required,
	})
}

func (s *Server) handleAllowPairingSender(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}
	var req struct {
		ChannelID  string `json:"channel_id"`
		SenderID   string `json:"sender_id"`
		SenderName string `json:"sender_name"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := s.pairingMgr.AuthorizeSender(req.ChannelID, req.SenderID, req.SenderName); err != nil {
		s.respondError(w, http.StatusBadRequest, "ALLOW_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "paired", "sender": req.SenderID})
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

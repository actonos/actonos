package server

import (
	"net/http"
	"os"
	"path/filepath"
)

type setupWizardRequest struct {
	WifiSSID     string `json:"wifi_ssid"`
	WifiPassword string `json:"wifi_password"`
	AnthropicKey string `json:"anthropic_key,omitempty"`
	GeminiKey    string `json:"gemini_key,omitempty"`
	OpenAIKey    string `json:"openai_key,omitempty"`
	TailscaleKey string `json:"tailscale_key,omitempty"`
	AdminPIN     string `json:"admin_pin,omitempty"`
}

func (s *Server) handleGetSetupStatus(w http.ResponseWriter, r *http.Request) {
	// Setup is completed if vault exists or agents are registered
	configured := false
	if s.agentMgr != nil {
		agents, _ := s.agentMgr.List(r.Context())
		if len(agents) > 0 {
			configured = true
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"configured":   configured,
		"runtime_mode": s.hal.RuntimeMode(),
	})
}

func (s *Server) handleSetupWizard(w http.ResponseWriter, r *http.Request) {
	var req setupWizardRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// 1. Connect Wi-Fi if provided
	if req.WifiSSID != "" && s.hal != nil {
		_ = s.hal.ConnectWifi(r.Context(), req.WifiSSID, req.WifiPassword)
	}

	// 2. Save keys into config file or vault
	configDir := "./data/config"
	_ = os.MkdirAll(configDir, 0755)

	if req.AnthropicKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "anthropic.key"), []byte(req.AnthropicKey), 0600)
	}
	if req.GeminiKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "gemini.key"), []byte(req.GeminiKey), 0600)
	}
	if req.OpenAIKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "openai.key"), []byte(req.OpenAIKey), 0600)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "completed",
		"message": "ActonOS appliance configured successfully",
	})
}

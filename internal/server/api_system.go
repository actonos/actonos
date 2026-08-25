package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/system"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	metrics, err := s.hal.GetMetrics(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "METRICS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleGetTailscale(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"enabled":   false,
		})
		return
	}

	status, err := s.tailscale.GetStatus(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TAILSCALE_STATUS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleWifiScan(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	networks, err := s.hal.ScanWifi(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "WIFI_SCAN_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"networks": networks,
		"count":    len(networks),
	})
}

func (s *Server) handleWifiConnect(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.SSID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "ssid is required")
		return
	}

	if err := s.hal.ConnectWifi(r.Context(), req.SSID, req.Password); err != nil {
		s.respondError(w, http.StatusBadRequest, "WIFI_CONNECT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "connected",
		"ssid":   req.SSID,
	})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	approval, err := s.requestAdminAction(r.Context(), "system_restart", map[string]any{})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

// ==============================================================================
// Production LLM Provider Management & Real Latency Testing
// ==============================================================================

// LLMProviderRecord holds configuration and metadata for an LLM provider.
type LLMProviderRecord struct {
	ID           string `json:"id"`            // "anthropic", "openai", "gemini", "deepseek", "groq", "openrouter", "mistral", "ollama", "custom_openai"
	Name         string `json:"name"`          // e.g. "Anthropic Claude"
	APIKey       string `json:"api_key"`       // Runtime-only Vault secret; stripped from persisted metadata
	BaseURL      string `json:"base_url"`      // Custom API endpoint URL
	DefaultModel string `json:"default_model"` // e.g. "claude-3-7-sonnet"
	Enabled      bool   `json:"enabled"`       // Whether active in cascade
	LastLatency  int64  `json:"last_latency"`  // Ping latency in milliseconds
	LastTested   string `json:"last_tested"`   // RFC3339 timestamp
	Status       string `json:"status"`        // "connected", "error", "not_configured"
}

// ProviderDefaults specifies factory settings for all known providers.
var providerDefaults = map[string]LLMProviderRecord{
	"anthropic": {
		ID:           "anthropic",
		Name:         "Anthropic Claude",
		DefaultModel: "claude-sonnet-4-6",
		BaseURL:      "https://api.anthropic.com/v1",
		Enabled:      true,
	},
	"openai": {
		ID:           "openai",
		Name:         "OpenAI",
		DefaultModel: "gpt-5",
		BaseURL:      "https://api.openai.com/v1",
		Enabled:      true,
	},
	"deepseek": {
		ID:           "deepseek",
		Name:         "DeepSeek",
		DefaultModel: "deepseek-v4-flash",
		BaseURL:      "https://api.deepseek.com/v1",
		Enabled:      true,
	},
	"grok": {
		ID:           "grok",
		Name:         "xAI (Grok)",
		DefaultModel: "grok-4.5",
		BaseURL:      "https://api.x.ai/v1",
		Enabled:      true,
	},
	"openrouter": {
		ID:           "openrouter",
		Name:         "OpenRouter",
		DefaultModel: "anthropic/claude-sonnet-5",
		BaseURL:      "https://openrouter.ai/api/v1",
		Enabled:      true,
	},
	"custom_openai": {
		ID:           "custom_openai",
		Name:         "Custom OpenAI-Compatible",
		DefaultModel: "default-model",
		BaseURL:      "http://localhost:8000/v1",
		Enabled:      false,
	},
}

var providersMu sync.RWMutex

// Helper: load stored LLM providers from disk
func loadStoredProviders(configDir string) map[string]LLMProviderRecord {
	filePath := filepath.Join(configDir, "llm_providers.json")

	result := make(map[string]LLMProviderRecord)
	for k, v := range providerDefaults {
		result[k] = v
	}

	data, err := os.ReadFile(filePath)
	if err == nil {
		var stored map[string]LLMProviderRecord
		if err := json.Unmarshal(data, &stored); err == nil {
			for k, v := range stored {
				base := result[k]
				if base.ID == "" {
					base = v
				}
				base.APIKey = v.APIKey
				if v.BaseURL != "" {
					base.BaseURL = v.BaseURL
				}
				if v.DefaultModel != "" {
					base.DefaultModel = v.DefaultModel
				}
				base.Enabled = v.Enabled
				base.LastLatency = v.LastLatency
				base.LastTested = v.LastTested
				base.Status = v.Status
				result[k] = base
			}
		}
	}

	// Legacy file fallback migration (.key files)
	readKeyFile := func(filename string) string {
		b, err := os.ReadFile(filepath.Join(configDir, filename))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}

	if ant := readKeyFile("anthropic.key"); ant != "" && result["anthropic"].APIKey == "" {
		p := result["anthropic"]
		p.APIKey = ant
		result["anthropic"] = p
	}
	if gem := readKeyFile("gemini.key"); gem != "" && result["gemini"].APIKey == "" {
		p := result["gemini"]
		p.APIKey = gem
		result["gemini"] = p
	}
	if oai := readKeyFile("openai.key"); oai != "" && result["openai"].APIKey == "" {
		p := result["openai"]
		p.APIKey = oai
		result["openai"] = p
	}
	if ds := readKeyFile("deepseek.key"); ds != "" && result["deepseek"].APIKey == "" {
		p := result["deepseek"]
		p.APIKey = ds
		result["deepseek"] = p
	}
	if ollama := readKeyFile("ollama.url"); ollama != "" {
		p := result["ollama"]
		p.BaseURL = ollama
		result["ollama"] = p
	}

	return result
}

func providerSecretName(providerID string) string {
	return "llm.provider." + providerID + ".api_key"
}

// loadStoredProvidersWithVault overlays encrypted secrets and migrates legacy
// plaintext JSON/key files into Vault.
func loadStoredProvidersWithVault(
	ctx context.Context,
	configDir string,
	vault *memory.Vault,
) map[string]LLMProviderRecord {
	stored := loadStoredProviders(configDir)
	if vault == nil {
		for id, record := range stored {
			record.APIKey = ""
			stored[id] = record
		}
		return stored
	}

	metadataChanged := false
	for id, record := range stored {
		plaintextOnDisk := record.APIKey != ""
		secret, err := vault.GetSecret(ctx, providerSecretName(id))
		if err == nil {
			record.APIKey = secret
			stored[id] = record
			_ = os.Remove(filepath.Join(configDir, id+".key"))
			metadataChanged = metadataChanged || plaintextOnDisk
			continue
		}
		if record.APIKey == "" {
			continue
		}
		if err := vault.SetSecret(ctx, providerSecretName(id), record.APIKey); err == nil {
			_ = os.Remove(filepath.Join(configDir, id+".key"))
			metadataChanged = true
		} else {
			// Never activate a provider from plaintext when migration cannot
			// establish encrypted storage. Leave the legacy file intact for a
			// later retry, but fail closed for this process.
			record.APIKey = ""
			stored[id] = record
		}
	}
	if metadataChanged {
		_ = saveStoredProviders(configDir, stored)
	}
	return stored
}

// Helper: save non-secret LLM provider metadata to disk.
func saveStoredProviders(configDir string, providers map[string]LLMProviderRecord) error {
	_ = os.MkdirAll(configDir, 0755)
	filePath := filepath.Join(configDir, "llm_providers.json")

	sanitized := make(map[string]LLMProviderRecord, len(providers))
	for id, record := range providers {
		record.APIKey = ""
		sanitized[id] = record
	}
	data, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0600)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		if len(key) == 0 {
			return ""
		}
		return "••••••••"
	}
	return key[:4] + "••••••••" + key[len(key)-4:]
}

type ProviderDetailItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Configured   bool   `json:"configured"`
	MaskedKey    string `json:"masked_key"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	Enabled      bool   `json:"enabled"`
	LastLatency  int64  `json:"last_latency"`
	LastTested   string `json:"last_tested"`
	Status       string `json:"status"`
}

type ComprehensiveKeysResponse struct {
	Providers []ProviderDetailItem `json:"providers"`

	// Legacy backward compatibility fields
	AnthropicConfigured bool   `json:"anthropic_configured"`
	AnthropicMasked     string `json:"anthropic_masked"`
	GeminiConfigured    bool   `json:"gemini_configured"`
	GeminiMasked        string `json:"gemini_masked"`
	OpenAIConfigured    bool   `json:"openai_configured"`
	OpenAIMasked        string `json:"openai_masked"`
	DeepSeekConfigured  bool   `json:"deepseek_configured"`
	DeepSeekMasked      string `json:"deepseek_masked"`
	OllamaURL           string `json:"ollama_url"`
}

func (s *Server) handleGetAPIKeys(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	providersMu.Lock()
	stored := loadStoredProvidersWithVault(r.Context(), configDir, s.vault)
	providersMu.Unlock()

	// Provider ordering
	order := []string{"anthropic", "openai", "gemini", "deepseek", "groq", "openrouter", "mistral", "ollama", "custom_openai"}
	var items []ProviderDetailItem

	for _, id := range order {
		rec, ok := stored[id]
		if !ok {
			rec = providerDefaults[id]
		}
		configured := rec.APIKey != "" || id == "ollama"
		status := rec.Status
		if status == "" {
			if configured {
				status = "configured"
			} else {
				status = "not_configured"
			}
		}

		items = append(items, ProviderDetailItem{
			ID:           rec.ID,
			Name:         rec.Name,
			Configured:   configured,
			MaskedKey:    maskKey(rec.APIKey),
			BaseURL:      rec.BaseURL,
			DefaultModel: rec.DefaultModel,
			Enabled:      rec.Enabled,
			LastLatency:  rec.LastLatency,
			LastTested:   rec.LastTested,
			Status:       status,
		})
	}

	ant := stored["anthropic"]
	gem := stored["gemini"]
	oai := stored["openai"]
	ds := stored["deepseek"]
	oll := stored["ollama"]

	resp := ComprehensiveKeysResponse{
		Providers:           items,
		AnthropicConfigured: ant.APIKey != "",
		AnthropicMasked:     maskKey(ant.APIKey),
		GeminiConfigured:    gem.APIKey != "",
		GeminiMasked:        maskKey(gem.APIKey),
		OpenAIConfigured:    oai.APIKey != "",
		OpenAIMasked:        maskKey(oai.APIKey),
		DeepSeekConfigured:  ds.APIKey != "",
		DeepSeekMasked:      maskKey(ds.APIKey),
		OllamaURL:           oll.BaseURL,
	}

	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSaveAPIKeys(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")

	var req struct {
		// Single provider direct save
		Provider     string `json:"provider,omitempty"`
		APIKey       string `json:"api_key,omitempty"`
		BaseURL      string `json:"base_url,omitempty"`
		DefaultModel string `json:"default_model,omitempty"`
		Enabled      *bool  `json:"enabled,omitempty"`

		// Batch save legacy fields
		AnthropicKey string `json:"anthropic_key,omitempty"`
		GeminiKey    string `json:"gemini_key,omitempty"`
		OpenAIKey    string `json:"openai_key,omitempty"`
		DeepSeekKey  string `json:"deepseek_key,omitempty"`
		OllamaURL    string `json:"ollama_url,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	providersMu.Lock()
	defer providersMu.Unlock()

	stored := loadStoredProvidersWithVault(r.Context(), configDir, s.vault)

	// Direct single provider save
	if req.Provider != "" {
		p := stored[req.Provider]
		if p.ID == "" {
			p = providerDefaults[req.Provider]
			if p.ID == "" {
				p.ID = req.Provider
				p.Name = req.Provider
			}
		}

		if req.APIKey != "" {
			if s.vault == nil {
				s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is required to store provider keys")
				return
			}
			p.APIKey = strings.TrimSpace(req.APIKey)
			if err := s.vault.SetSecret(r.Context(), providerSecretName(req.Provider), p.APIKey); err != nil {
				s.respondError(w, http.StatusInternalServerError, "VAULT_WRITE_FAILED", err.Error())
				return
			}
		}
		if req.BaseURL != "" {
			p.BaseURL = strings.TrimSpace(req.BaseURL)
		}
		if req.DefaultModel != "" {
			p.DefaultModel = strings.TrimSpace(req.DefaultModel)
		}
		if req.Enabled != nil {
			p.Enabled = *req.Enabled
		}
		stored[req.Provider] = p

		// Dynamic router registration
		s.registerProviderInRouter(p)
	}

	// Legacy batch save
	if req.AnthropicKey != "" {
		p := stored["anthropic"]
		p.APIKey = strings.TrimSpace(req.AnthropicKey)
		if s.vault == nil {
			s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is required to store provider keys")
			return
		}
		if err := s.vault.SetSecret(r.Context(), providerSecretName("anthropic"), p.APIKey); err != nil {
			s.respondError(w, http.StatusInternalServerError, "VAULT_WRITE_FAILED", err.Error())
			return
		}
		stored["anthropic"] = p
		s.registerProviderInRouter(p)
		_ = os.Remove(filepath.Join(configDir, "anthropic.key"))
	}
	if req.GeminiKey != "" {
		p := stored["gemini"]
		p.APIKey = strings.TrimSpace(req.GeminiKey)
		if s.vault == nil {
			s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is required to store provider keys")
			return
		}
		if err := s.vault.SetSecret(r.Context(), providerSecretName("gemini"), p.APIKey); err != nil {
			s.respondError(w, http.StatusInternalServerError, "VAULT_WRITE_FAILED", err.Error())
			return
		}
		stored["gemini"] = p
		s.registerProviderInRouter(p)
		_ = os.Remove(filepath.Join(configDir, "gemini.key"))
	}
	if req.OpenAIKey != "" {
		p := stored["openai"]
		p.APIKey = strings.TrimSpace(req.OpenAIKey)
		if s.vault == nil {
			s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is required to store provider keys")
			return
		}
		if err := s.vault.SetSecret(r.Context(), providerSecretName("openai"), p.APIKey); err != nil {
			s.respondError(w, http.StatusInternalServerError, "VAULT_WRITE_FAILED", err.Error())
			return
		}
		stored["openai"] = p
		s.registerProviderInRouter(p)
		_ = os.Remove(filepath.Join(configDir, "openai.key"))
	}
	if req.DeepSeekKey != "" {
		p := stored["deepseek"]
		p.APIKey = strings.TrimSpace(req.DeepSeekKey)
		if s.vault == nil {
			s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is required to store provider keys")
			return
		}
		if err := s.vault.SetSecret(r.Context(), providerSecretName("deepseek"), p.APIKey); err != nil {
			s.respondError(w, http.StatusInternalServerError, "VAULT_WRITE_FAILED", err.Error())
			return
		}
		stored["deepseek"] = p
		s.registerProviderInRouter(p)
		_ = os.Remove(filepath.Join(configDir, "deepseek.key"))
	}
	if req.OllamaURL != "" {
		p := stored["ollama"]
		p.BaseURL = strings.TrimSpace(req.OllamaURL)
		stored["ollama"] = p
		s.registerProviderInRouter(p)
		_ = os.WriteFile(filepath.Join(configDir, "ollama.url"), []byte(p.BaseURL), 0600)
	}

	if err := saveStoredProviders(configDir, stored); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	providersMu.Lock()
	defer providersMu.Unlock()

	configDir := filepath.Join(s.dataDir, "config")
	stored := loadStoredProvidersWithVault(r.Context(), configDir, s.vault)

	if p, ok := stored[provider]; ok {
		p.APIKey = ""
		p.Status = "not_configured"
		stored[provider] = p
		_ = saveStoredProviders(configDir, stored)
	}
	if s.vault != nil {
		_ = s.vault.DeleteSecret(r.Context(), providerSecretName(provider))
	}

	// Remove legacy key file if exists
	_ = os.Remove(filepath.Join(configDir, provider+".key"))

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "provider": provider})
}

// RegisterAllStoredProviders loads all configured providers from disk and registers them into router.
func RegisterAllStoredProviders(router *llm.ModelCascadeRouter, configDir string) {
	RegisterAllStoredProvidersWithVault(context.Background(), router, configDir, nil)
}

// RegisterAllStoredProvidersWithVault registers providers using encrypted keys.
func RegisterAllStoredProvidersWithVault(
	ctx context.Context,
	router *llm.ModelCascadeRouter,
	configDir string,
	vault *memory.Vault,
) {
	if router == nil {
		return
	}
	stored := loadStoredProvidersWithVault(ctx, configDir, vault)
	for _, rec := range stored {
		if rec.Enabled && (rec.APIKey != "" || rec.ID == "ollama" || rec.ID == "custom_openai") {
			RegisterProviderInRouter(router, rec)
		}
	}
}

// RegisterProviderInRouter registers a specific provider and all its supported model aliases into llmRouter.
func RegisterProviderInRouter(router *llm.ModelCascadeRouter, rec LLMProviderRecord) {
	if router == nil || !rec.Enabled || (rec.APIKey == "" && rec.ID != "custom_openai" && rec.ID != "ollama") {
		return
	}

	defaultModel := rec.DefaultModel
	baseURL := rec.BaseURL

	// Match provider from canonical catalog
	for _, provSpec := range llm.GetCanonicalProviders() {
		if provSpec.ID == rec.ID {
			if baseURL == "" {
				baseURL = provSpec.DefaultBaseURL
			}
			if defaultModel == "" && len(provSpec.ModelPresets) > 0 {
				defaultModel = provSpec.ModelPresets[0].ID
			}

			var baseProv llm.LLMProvider
			switch rec.ID {
			case "anthropic":
				baseProv = llm.NewAnthropicProvider(rec.APIKey, defaultModel)
			case "deepseek":
				baseProv = llm.NewDeepSeekProvider(rec.APIKey, defaultModel)
			default:
				baseProv = llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
				// Responses is native to the OpenAI provider only. A custom provider
				// may point at api.openai.com while still exposing Chat Completions.
				if rec.ID != "openai" {
					if compatible, ok := baseProv.(*llm.OpenAIProvider); ok {
						compatible.UseResponsesAPI = false
					}
				}
			}

			router.RegisterProvider(rec.ID, baseProv)
			router.RegisterProvider(rec.ID+"/"+defaultModel, baseProv)

			// Register each canonical model preset
			for _, m := range provSpec.ModelPresets {
				var p llm.LLMProvider
				switch rec.ID {
				case "anthropic":
					p = llm.NewAnthropicProvider(rec.APIKey, m.ID)
				case "deepseek":
					p = llm.NewDeepSeekProvider(rec.APIKey, m.ID)
				default:
					p = llm.NewOpenAIProvider(rec.APIKey, m.ID, baseURL)
					if rec.ID != "openai" {
						if compatible, ok := p.(*llm.OpenAIProvider); ok {
							compatible.UseResponsesAPI = false
						}
					}
				}
				router.RegisterProvider(m.ID, p)
			}
			return
		}
	}

	// Custom OpenAI-compatible fallback
	if defaultModel == "" {
		defaultModel = "default-model"
	}
	if baseURL == "" {
		baseURL = "http://localhost:8000/v1"
	}
	prov := llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
	if rec.ID != "openai" {
		prov.UseResponsesAPI = false
	}
	router.RegisterProvider(rec.ID, prov)
	router.RegisterProvider(rec.ID+"/"+defaultModel, prov)
}

// Helper: dynamically registers/updates provider in s.llmRouter
func (s *Server) registerProviderInRouter(rec LLMProviderRecord) {
	RegisterProviderInRouter(s.llmRouter, rec)
}

// Live provider test with roundtrip latency measurement and clear diagnostics
func (s *Server) handleTestAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"` // "anthropic", "openai", "gemini", "deepseek", "groq", "openrouter", "mistral", "ollama", "custom_openai"
		Key      string `json:"key"`
		URL      string `json:"url,omitempty"`
		Model    string `json:"model,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	configDir := filepath.Join(s.dataDir, "config")
	providersMu.Lock()
	stored := loadStoredProvidersWithVault(r.Context(), configDir, s.vault)
	providersMu.Unlock()

	rec := stored[req.Provider]
	key := req.Key
	if key == "" {
		key = rec.APIKey
	}
	baseURL := req.URL
	if baseURL == "" {
		baseURL = rec.BaseURL
	}
	if baseURL == "" {
		baseURL = providerDefaults[req.Provider].BaseURL
	}
	model := req.Model
	if model == "" {
		model = rec.DefaultModel
	}
	if model == "" {
		model = providerDefaults[req.Provider].DefaultModel
	}

	if key == "" && req.Provider != "ollama" {
		s.respondError(w, http.StatusBadRequest, "MISSING_KEY", "API key is required for connection testing")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	var provider llm.LLMProvider
	switch req.Provider {
	case "anthropic":
		provider = llm.NewAnthropicProvider(key, model)
	case "deepseek":
		provider = llm.NewDeepSeekProvider(key, model)
	default:
		// OpenAI-compatible (OpenAI, Grok, OpenRouter, Custom)
		provider = llm.NewOpenAIProvider(key, model, baseURL)
		if req.Provider != "openai" {
			if compatible, ok := provider.(*llm.OpenAIProvider); ok {
				compatible.UseResponsesAPI = false
			}
		}
	}

	testMsg := []llm.Message{{Role: llm.RoleUser, Content: "Say 'OK' in one word."}}

	start := time.Now()
	resp, err := provider.Complete(ctx, testMsg, llm.CompletionOptions{})
	latency := time.Since(start).Milliseconds()

	// Update stored status and latency
	providersMu.Lock()
	if r, ok := stored[req.Provider]; ok {
		r.LastTested = time.Now().UTC().Format(time.RFC3339)
		if err == nil {
			r.LastLatency = latency
			r.Status = "connected"
		} else {
			r.Status = "error"
		}
		stored[req.Provider] = r
		_ = saveStoredProviders(configDir, stored)
	}
	providersMu.Unlock()

	if err != nil {
		s.respondError(w, http.StatusBadRequest, "TEST_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":     "connected",
		"provider":   req.Provider,
		"latency_ms": latency,
		"model":      resp.Model,
		"response":   resp.Content,
	})
}

// ==============================================================================
// Audit Log & Storage Breakdown Handlers
// ==============================================================================

func (s *Server) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.auditLogger == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0})
		return
	}
	entries, err := s.auditLogger.ReadRecentEntries(100)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

func (s *Server) handleVerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	if s.auditLogger == nil {
		s.respondError(w, http.StatusNotImplemented, "AUDIT_NOT_CONFIGURED", "audit logger is not configured")
		return
	}
	if err := s.auditLogger.VerifyChain(); err != nil {
		s.respondError(w, http.StatusConflict, "AUDIT_CHAIN_INVALID", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":      "valid",
		"verified_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func getDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info fs.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func (s *Server) handleGetStorageInfo(w http.ResponseWriter, r *http.Request) {
	dataDir := s.dataDir
	storageSize := getDirSize(filepath.Join(dataDir, "storage"))
	vectorsSize := getDirSize(filepath.Join(dataDir, "vectors"))
	workspaceSize := int64(0)
	if s.workspaceStore != nil {
		if stats, err := s.workspaceStore.Stats(r.Context()); err == nil {
			workspaceSize = stats.TotalSize
		}
	}
	agentWorkspaceSize := getDirSize(filepath.Join(dataDir, "agents"))
	logsSize := getDirSize(filepath.Join(dataDir, "logs"))

	totalSize := storageSize + vectorsSize + agentWorkspaceSize + logsSize

	s.respondJSON(w, http.StatusOK, map[string]any{
		"storage_bytes":         storageSize,
		"vectors_bytes":         vectorsSize,
		"workspace_bytes":       workspaceSize,
		"agent_workspace_bytes": agentWorkspaceSize,
		"logs_bytes":            logsSize,
		"total_bytes":           totalSize,
	})
}

func (s *Server) handleCheckOTA(w http.ResponseWriter, r *http.Request) {
	eng := system.NewOTAEngine(s.dataDir)
	active, previous := eng.State()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"current_version":  s.version,
		"update_available": false,
		"latest_version":   s.version,
		"git_commit":       s.gitCommit,
		"build_time":       s.buildTime,
		"last_checked":     time.Now().UTC().Format(time.RFC3339),
		"active_binary":    active,
		"previous_binary":  previous,
	})
}

func (s *Server) handleGetBackup(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil || s.memory.DB() == nil || s.memory.DB().SQLDB() == nil {
		s.respondError(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "database is not configured")
		return
	}
	tempDir, err := os.MkdirTemp("", "actonos-backup-*")
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}
	defer os.RemoveAll(tempDir)
	tempBackupFile := filepath.Join(tempDir, "actonos-backup.db")

	// VACUUM INTO creates a transactionally consistent standalone database,
	// including committed WAL content, without blocking normal readers.
	if _, err := s.memory.DB().SQLDB().ExecContext(r.Context(), `VACUUM INTO ?`, tempBackupFile); err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}
	data, err := os.ReadFile(tempBackupFile)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", "attachment; filename=\"actonos-backup.db\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleGetTokenUsage(w http.ResponseWriter, r *http.Request) {
	if s.tokenTracker == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"total_tokens":   0,
			"total_cost_usd": 0.0,
			"today_tokens":   0,
			"today_cost_usd": 0.0,
			"month_tokens":   0,
			"month_cost_usd": 0.0,
			"by_model":       []any{},
			"by_agent":       []any{},
			"daily_trend":    []any{},
		})
		return
	}

	summary, err := s.tokenTracker.GetSummary(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TOKEN_QUERY_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, summary)
}

func (s *Server) handleGetTokenHistory(w http.ResponseWriter, r *http.Request) {
	if s.tokenTracker == nil {
		s.respondJSON(w, http.StatusOK, []any{})
		return
	}

	limit := 50
	agentID := r.URL.Query().Get("agent_id")
	source := r.URL.Query().Get("source")

	records, err := s.tokenTracker.GetHistory(r.Context(), limit, agentID, source)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TOKEN_HISTORY_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, records)
}

func (s *Server) handleGetHeartbeatHistory(w http.ResponseWriter, r *http.Request) {
	if s.heartbeat == nil {
		s.respondJSON(w, http.StatusOK, []any{})
		return
	}

	runs, err := s.heartbeat.GetRecentRuns(r.Context(), 30)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "HEARTBEAT_QUERY_FAILED", err.Error())
		return
	}
	if runs == nil {
		runs = []agent.HeartbeatRun{}
	}
	s.respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(s.startTime).Seconds()
	activeAgents := 0
	if s.agentMgr != nil {
		agents, _ := s.agentMgr.List(r.Context())
		for _, a := range agents {
			if a.Status == agent.StatusActive {
				activeAgents++
			}
		}
	}

	var totalTokens int64
	var totalCost float64
	if s.tokenTracker != nil {
		if sum, err := s.tokenTracker.GetSummary(r.Context()); err == nil {
			totalTokens = sum.TotalTokens
			totalCost = sum.TotalCostUSD
		}
	}

	var out strings.Builder
	out.WriteString("# HELP actonos_uptime_seconds Total runtime uptime in seconds\n")
	out.WriteString("# TYPE actonos_uptime_seconds counter\n")
	out.WriteString(fmt.Sprintf("actonos_uptime_seconds %f\n", uptime))

	out.WriteString("# HELP actonos_goroutines Current active goroutines\n")
	out.WriteString("# TYPE actonos_goroutines gauge\n")
	out.WriteString(fmt.Sprintf("actonos_goroutines %d\n", runtime.NumGoroutine()))

	out.WriteString("# HELP actonos_memory_alloc_bytes Memory currently allocated\n")
	out.WriteString("# TYPE actonos_memory_alloc_bytes gauge\n")
	out.WriteString(fmt.Sprintf("actonos_memory_alloc_bytes %d\n", m.Alloc))

	out.WriteString("# HELP actonos_agents_active Number of active running agents\n")
	out.WriteString("# TYPE actonos_agents_active gauge\n")
	out.WriteString(fmt.Sprintf("actonos_agents_active %d\n", activeAgents))

	out.WriteString("# HELP actonos_tokens_total Total LLM tokens consumed\n")
	out.WriteString("# TYPE actonos_tokens_total counter\n")
	out.WriteString(fmt.Sprintf("actonos_tokens_total %d\n", totalTokens))

	out.WriteString("# HELP actonos_cost_usd_total Total estimated LLM cost in USD\n")
	out.WriteString("# TYPE actonos_cost_usd_total counter\n")
	out.WriteString(fmt.Sprintf("actonos_cost_usd_total %f\n", totalCost))

	var droppedEvents uint64
	if s.bus != nil {
		droppedEvents = s.bus.DroppedEvents()
	}
	out.WriteString("# HELP actonos_eventbus_dropped_total Subscriber event deliveries dropped due to backpressure\n")
	out.WriteString("# TYPE actonos_eventbus_dropped_total counter\n")
	out.WriteString(fmt.Sprintf("actonos_eventbus_dropped_total %d\n", droppedEvents))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out.String()))
}

// ==============================================================================
// Identity & User Profile Handlers
// ==============================================================================

func (s *Server) handleGetIdentity(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"user_name":           "Operator",
			"user_role":           "System Administrator & Architect",
			"language":            "en",
			"timezone":            "Asia/Ho_Chi_Minh",
			"communication_style": "adaptive, natural, empathetic & sharp",
			"bio":                 "Owner of the ActonOS local intelligence kernel.",
			"custom_instructions": "Provide intelligent, natural, and empathetic responses. Act as a trusted senior engineering partner. Proactively solve problems and avoid robotic or stiff clichés.",
			"soul":                "",
		})
		return
	}

	profile := s.profileMgr.GetProfile()
	soul := s.profileMgr.GetSoul()

	s.respondJSON(w, http.StatusOK, map[string]any{
		"user_name":           profile.UserName,
		"user_role":           profile.UserRole,
		"language":            profile.Language,
		"timezone":            profile.Timezone,
		"communication_style": profile.CommunicationStyle,
		"bio":                 profile.Bio,
		"custom_instructions": profile.CustomInstructions,
		"soul":                soul,
		"updated_at":          profile.UpdatedAt,
	})
}

func (s *Server) handleSaveIdentity(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "PROFILE_MANAGER_UNAVAILABLE", "profile manager not initialized")
		return
	}

	var req struct {
		UserName           string `json:"user_name"`
		UserRole           string `json:"user_role"`
		Language           string `json:"language"`
		Timezone           string `json:"timezone"`
		CommunicationStyle string `json:"communication_style"`
		Bio                string `json:"bio"`
		CustomInstructions string `json:"custom_instructions"`
		Soul               string `json:"soul"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	current := s.profileMgr.GetProfile()
	if req.UserName != "" {
		current.UserName = req.UserName
	}
	if req.UserRole != "" {
		current.UserRole = req.UserRole
	}
	if req.Language != "" {
		current.Language = req.Language
	}
	if req.Timezone != "" {
		current.Timezone = req.Timezone
	}
	if req.CommunicationStyle != "" {
		current.CommunicationStyle = req.CommunicationStyle
	}
	if req.Bio != "" {
		current.Bio = req.Bio
	}
	if req.CustomInstructions != "" {
		current.CustomInstructions = req.CustomInstructions
	}

	if err := s.profileMgr.UpdateProfile(r.Context(), current); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_PROFILE_FAILED", err.Error())
		return
	}

	if req.Soul != "" {
		_ = s.profileMgr.SaveSoul(r.Context(), req.Soul)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":     "saved",
		"profile":    current,
		"updated_at": current.UpdatedAt,
	})
}

func (s *Server) handleGetModelsCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := llm.GetCatalogResponse()
	s.respondJSON(w, http.StatusOK, catalog)
}

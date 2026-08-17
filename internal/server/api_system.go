package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/llm"
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

	go func() {
		time.Sleep(1 * time.Second)
		_ = s.hal.RestartDaemon(context.Background())
	}()

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
}

// ==============================================================================
// Production LLM Provider Management & Real Latency Testing
// ==============================================================================

// LLMProviderRecord holds configuration and metadata for an LLM provider.
type LLMProviderRecord struct {
	ID           string `json:"id"`            // "anthropic", "openai", "gemini", "deepseek", "groq", "openrouter", "mistral", "ollama", "custom_openai"
	Name         string `json:"name"`          // e.g. "Anthropic Claude"
	APIKey       string `json:"api_key"`       // Stored raw, masked on output
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
		DefaultModel: "claude-3-7-sonnet",
		BaseURL:      "https://api.anthropic.com",
		Enabled:      true,
	},
	"openai": {
		ID:           "openai",
		Name:         "OpenAI",
		DefaultModel: "gpt-4o",
		BaseURL:      "https://api.openai.com/v1",
		Enabled:      true,
	},
	"gemini": {
		ID:           "gemini",
		Name:         "Google Gemini",
		DefaultModel: "gemini-2.5-flash",
		BaseURL:      "https://generativelanguage.googleapis.com",
		Enabled:      true,
	},
	"deepseek": {
		ID:           "deepseek",
		Name:         "DeepSeek",
		DefaultModel: "deepseek-chat",
		BaseURL:      "https://api.deepseek.com/v1",
		Enabled:      true,
	},
	"groq": {
		ID:           "groq",
		Name:         "Groq",
		DefaultModel: "llama-3.3-70b-versatile",
		BaseURL:      "https://api.groq.com/openai/v1",
		Enabled:      true,
	},
	"openrouter": {
		ID:           "openrouter",
		Name:         "OpenRouter",
		DefaultModel: "anthropic/claude-3.7-sonnet",
		BaseURL:      "https://openrouter.ai/api/v1",
		Enabled:      true,
	},
	"mistral": {
		ID:           "mistral",
		Name:         "Mistral AI",
		DefaultModel: "mistral-large-latest",
		BaseURL:      "https://api.mistral.ai/v1",
		Enabled:      true,
	},
	"ollama": {
		ID:           "ollama",
		Name:         "Local Ollama / vLLM",
		DefaultModel: "llama3",
		BaseURL:      "http://localhost:11434",
		Enabled:      true,
	},
	"custom_openai": {
		ID:           "custom_openai",
		Name:         "Custom OpenAI Compatible",
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

// Helper: save stored LLM providers to disk
func saveStoredProviders(configDir string, providers map[string]LLMProviderRecord) error {
	_ = os.MkdirAll(configDir, 0755)
	filePath := filepath.Join(configDir, "llm_providers.json")

	data, err := json.MarshalIndent(providers, "", "  ")
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
	configDir := "./data/config"
	providersMu.RLock()
	stored := loadStoredProviders(configDir)
	providersMu.RUnlock()

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
	configDir := "./data/config"

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

	stored := loadStoredProviders(configDir)

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
			p.APIKey = strings.TrimSpace(req.APIKey)
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
		stored["anthropic"] = p
		s.registerProviderInRouter(p)
		_ = os.WriteFile(filepath.Join(configDir, "anthropic.key"), []byte(p.APIKey), 0600)
	}
	if req.GeminiKey != "" {
		p := stored["gemini"]
		p.APIKey = strings.TrimSpace(req.GeminiKey)
		stored["gemini"] = p
		s.registerProviderInRouter(p)
		_ = os.WriteFile(filepath.Join(configDir, "gemini.key"), []byte(p.APIKey), 0600)
	}
	if req.OpenAIKey != "" {
		p := stored["openai"]
		p.APIKey = strings.TrimSpace(req.OpenAIKey)
		stored["openai"] = p
		s.registerProviderInRouter(p)
		_ = os.WriteFile(filepath.Join(configDir, "openai.key"), []byte(p.APIKey), 0600)
	}
	if req.DeepSeekKey != "" {
		p := stored["deepseek"]
		p.APIKey = strings.TrimSpace(req.DeepSeekKey)
		stored["deepseek"] = p
		s.registerProviderInRouter(p)
		_ = os.WriteFile(filepath.Join(configDir, "deepseek.key"), []byte(p.APIKey), 0600)
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

	configDir := "./data/config"
	stored := loadStoredProviders(configDir)

	if p, ok := stored[provider]; ok {
		p.APIKey = ""
		p.Status = "not_configured"
		stored[provider] = p
		_ = saveStoredProviders(configDir, stored)
	}

	// Remove legacy key file if exists
	_ = os.Remove(filepath.Join(configDir, provider+".key"))

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "provider": provider})
}

// RegisterAllStoredProviders loads all configured providers from disk and registers them into router.
func RegisterAllStoredProviders(router *llm.ModelCascadeRouter, configDir string) {
	if router == nil {
		return
	}
	stored := loadStoredProviders(configDir)
	for _, rec := range stored {
		if rec.Enabled && (rec.APIKey != "" || rec.ID == "ollama" || rec.ID == "custom_openai") {
			RegisterProviderInRouter(router, rec)
		}
	}
}

// RegisterProviderInRouter registers a specific provider and all its supported model aliases into llmRouter.
func RegisterProviderInRouter(router *llm.ModelCascadeRouter, rec LLMProviderRecord) {
	if router == nil || !rec.Enabled {
		return
	}

	switch rec.ID {
	case "anthropic":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "claude-3-7-sonnet"
			}
			prov := llm.NewAnthropicProvider(rec.APIKey, defaultModel)
			router.RegisterProvider("anthropic", prov)
			router.RegisterProvider("claude", prov)
			router.RegisterProvider("anthropic/"+defaultModel, prov)
			for _, m := range []string{"claude-3-7-sonnet", "claude-3-5-sonnet", "claude-3-5-haiku", "claude-3-opus"} {
				router.RegisterProvider("anthropic/"+m, llm.NewAnthropicProvider(rec.APIKey, m))
			}
		}
	case "gemini":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "gemini-2.5-flash"
			}
			prov := llm.NewGeminiProvider(rec.APIKey, defaultModel)
			router.RegisterProvider("gemini", prov)
			router.RegisterProvider("google", prov)
			router.RegisterProvider("google/"+defaultModel, prov)
			for _, m := range []string{"gemini-2.5-flash", "gemini-2.0-flash", "gemini-2.0-flash-thinking-exp", "gemini-2.0-pro-exp-02-05", "gemini-1.5-pro", "gemini-1.5-flash"} {
				p := llm.NewGeminiProvider(rec.APIKey, m)
				router.RegisterProvider("google/"+m, p)
				router.RegisterProvider("gemini/"+m, p)
			}
		}
	case "ollama":
		defaultModel := rec.DefaultModel
		if defaultModel == "" {
			defaultModel = "llama3.3"
		}
		url := rec.BaseURL
		if url == "" {
			url = "http://localhost:11434"
		}
		prov := llm.NewOllamaProvider(defaultModel, url)
		router.RegisterProvider("ollama", prov)
		router.RegisterProvider("ollama/"+defaultModel, prov)
		for _, m := range []string{"llama3.3", "deepseek-r1:70b", "deepseek-r1:14b", "deepseek-r1:8b", "qwen2.5-coder:32b", "phi4", "mistral-nemo"} {
			router.RegisterProvider("ollama/"+m, llm.NewOllamaProvider(m, url))
		}
	case "openai":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "gpt-4o"
			}
			baseURL := rec.BaseURL
			if baseURL == "" {
				baseURL = "https://api.openai.com/v1"
			}
			prov := llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
			router.RegisterProvider("openai", prov)
			router.RegisterProvider("openai/"+defaultModel, prov)
			for _, m := range []string{"gpt-4.5-preview", "gpt-4o", "gpt-4o-mini", "o3-mini", "o1", "o1-mini"} {
				router.RegisterProvider("openai/"+m, llm.NewOpenAIProvider(rec.APIKey, m, baseURL))
			}
		}
	case "deepseek":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "deepseek-chat"
			}
			baseURL := rec.BaseURL
			if baseURL == "" {
				baseURL = "https://api.deepseek.com/v1"
			}
			prov := llm.NewDeepSeekProvider(rec.APIKey, defaultModel)
			router.RegisterProvider("deepseek", prov)
			router.RegisterProvider("deepseek/"+defaultModel, prov)
			for _, m := range []string{"deepseek-chat", "deepseek-reasoner"} {
				router.RegisterProvider("deepseek/"+m, llm.NewDeepSeekProvider(rec.APIKey, m))
			}
		}
	case "groq":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "llama-3.3-70b-versatile"
			}
			baseURL := rec.BaseURL
			if baseURL == "" {
				baseURL = "https://api.groq.com/openai/v1"
			}
			prov := llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
			router.RegisterProvider("groq", prov)
			router.RegisterProvider("groq/"+defaultModel, prov)
			for _, m := range []string{"llama-3.3-70b-versatile", "deepseek-r1-distill-llama-70b", "qwen-2.5-32b", "llama-3.1-8b-instant", "mixtral-8x7b-32768"} {
				router.RegisterProvider("groq/"+m, llm.NewOpenAIProvider(rec.APIKey, m, baseURL))
			}
		}
	case "openrouter":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "anthropic/claude-3.7-sonnet"
			}
			baseURL := rec.BaseURL
			if baseURL == "" {
				baseURL = "https://openrouter.ai/api/v1"
			}
			prov := llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
			router.RegisterProvider("openrouter", prov)
			router.RegisterProvider("openrouter/"+defaultModel, prov)
			for _, m := range []string{"anthropic/claude-3.7-sonnet", "openai/gpt-4o", "openai/o3-mini", "google/gemini-2.5-flash", "deepseek/deepseek-r1", "meta-llama/llama-3.3-70b-instruct"} {
				router.RegisterProvider("openrouter/"+m, llm.NewOpenAIProvider(rec.APIKey, m, baseURL))
			}
		}
	case "mistral":
		if rec.APIKey != "" {
			defaultModel := rec.DefaultModel
			if defaultModel == "" {
				defaultModel = "mistral-large-latest"
			}
			baseURL := rec.BaseURL
			if baseURL == "" {
				baseURL = "https://api.mistral.ai/v1"
			}
			prov := llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
			router.RegisterProvider("mistral", prov)
			router.RegisterProvider("mistral/"+defaultModel, prov)
			for _, m := range []string{"mistral-large-latest", "codestral-latest", "mistral-small-latest", "pixtral-large-latest"} {
				router.RegisterProvider("mistral/"+m, llm.NewOpenAIProvider(rec.APIKey, m, baseURL))
			}
		}
	case "custom_openai":
		defaultModel := rec.DefaultModel
		if defaultModel == "" {
			defaultModel = "default-model"
		}
		baseURL := rec.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:8000/v1"
		}
		prov := llm.NewOpenAIProvider(rec.APIKey, defaultModel, baseURL)
		router.RegisterProvider("custom_openai", prov)
		router.RegisterProvider("custom_openai/"+defaultModel, prov)
	}
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

	configDir := "./data/config"
	providersMu.RLock()
	stored := loadStoredProviders(configDir)
	providersMu.RUnlock()

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
	case "gemini":
		provider = llm.NewGeminiProvider(key, model)
	case "ollama":
		provider = llm.NewOllamaProvider(model, baseURL)
	case "deepseek":
		provider = llm.NewDeepSeekProvider(key, model)
	default:
		// OpenAI compatible
		provider = llm.NewOpenAIProvider(key, model, baseURL)
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
	auditLogger, err := system.NewAuditLogger("./data")
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0})
		return
	}
	defer auditLogger.Close()

	entries, err := auditLogger.ReadRecentEntries(100)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
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
	dataDir := "./data"
	storageSize := getDirSize(filepath.Join(dataDir, "storage"))
	vectorsSize := getDirSize(filepath.Join(dataDir, "vectors"))
	workspaceSize := getDirSize(filepath.Join(dataDir, "workspace"))
	logsSize := getDirSize(filepath.Join(dataDir, "logs"))

	totalSize := storageSize + vectorsSize + workspaceSize + logsSize

	s.respondJSON(w, http.StatusOK, map[string]any{
		"storage_bytes":   storageSize,
		"vectors_bytes":   vectorsSize,
		"workspace_bytes": workspaceSize,
		"logs_bytes":      logsSize,
		"total_bytes":     totalSize,
	})
}

func (s *Server) handleCheckOTA(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]any{
		"current_version":  "v0.1.0",
		"update_available": false,
		"latest_version":   "v0.1.0",
		"last_checked":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleGetBackup(w http.ResponseWriter, r *http.Request) {
	dbPath := filepath.Join("./data/storage", "acton.db")
	data, err := os.ReadFile(dbPath)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", "attachment; filename=\"actonos-backup.db\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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

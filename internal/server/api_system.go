package server

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/system"
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
// LLM Provider Keys & Settings Handlers
// ==============================================================================

type ProviderKeysResponse struct {
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

func maskKey(key string) string {
	if len(key) <= 8 {
		if len(key) == 0 {
			return ""
		}
		return "••••••••"
	}
	return key[:4] + "••••••••" + key[len(key)-4:]
}

func (s *Server) handleGetAPIKeys(w http.ResponseWriter, r *http.Request) {
	configDir := "./data/config"
	_ = os.MkdirAll(configDir, 0755)

	readKey := func(filename string) string {
		data, err := os.ReadFile(filepath.Join(configDir, filename))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	antKey := readKey("anthropic.key")
	gemKey := readKey("gemini.key")
	oaiKey := readKey("openai.key")
	dsKey := readKey("deepseek.key")
	ollamaURL := readKey("ollama.url")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	resp := ProviderKeysResponse{
		AnthropicConfigured: antKey != "",
		AnthropicMasked:     maskKey(antKey),
		GeminiConfigured:    gemKey != "",
		GeminiMasked:        maskKey(gemKey),
		OpenAIConfigured:    oaiKey != "",
		OpenAIMasked:        maskKey(oaiKey),
		DeepSeekConfigured:  dsKey != "",
		DeepSeekMasked:      maskKey(dsKey),
		OllamaURL:           ollamaURL,
	}

	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSaveAPIKeys(w http.ResponseWriter, r *http.Request) {
	configDir := "./data/config"
	_ = os.MkdirAll(configDir, 0755)

	var req struct {
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

	if req.AnthropicKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "anthropic.key"), []byte(strings.TrimSpace(req.AnthropicKey)), 0600)
		s.llmRouter.RegisterProvider("anthropic/claude-3-7-sonnet", llm.NewAnthropicProvider(req.AnthropicKey, "claude-3-7-sonnet"))
	}
	if req.GeminiKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "gemini.key"), []byte(strings.TrimSpace(req.GeminiKey)), 0600)
		s.llmRouter.RegisterProvider("google/gemini-2.5-flash", llm.NewGeminiProvider(req.GeminiKey, "gemini-2.5-flash"))
	}
	if req.OpenAIKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "openai.key"), []byte(strings.TrimSpace(req.OpenAIKey)), 0600)
		s.llmRouter.RegisterProvider("openai/gpt-4o", llm.NewOpenAIProvider(req.OpenAIKey, "gpt-4o", ""))
	}
	if req.DeepSeekKey != "" {
		_ = os.WriteFile(filepath.Join(configDir, "deepseek.key"), []byte(strings.TrimSpace(req.DeepSeekKey)), 0600)
		s.llmRouter.RegisterProvider("deepseek/deepseek-chat", llm.NewDeepSeekProvider(req.DeepSeekKey, "deepseek-chat"))
	}
	if req.OllamaURL != "" {
		_ = os.WriteFile(filepath.Join(configDir, "ollama.url"), []byte(strings.TrimSpace(req.OllamaURL)), 0600)
		s.llmRouter.RegisterProvider("ollama/llama3", llm.NewOllamaProvider("llama3", req.OllamaURL))
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleTestAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"` // "anthropic", "gemini", "openai", "deepseek", "ollama"
		Key      string `json:"key"`
		URL      string `json:"url,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var provider llm.LLMProvider
	switch req.Provider {
	case "anthropic":
		provider = llm.NewAnthropicProvider(req.Key, "claude-3-7-sonnet")
	case "gemini":
		provider = llm.NewGeminiProvider(req.Key, "gemini-2.5-flash")
	case "openai":
		provider = llm.NewOpenAIProvider(req.Key, "gpt-4o", "")
	case "deepseek":
		provider = llm.NewDeepSeekProvider(req.Key, "deepseek-chat")
	case "ollama":
		provider = llm.NewOllamaProvider("llama3", req.URL)
	default:
		s.respondError(w, http.StatusBadRequest, "UNKNOWN_PROVIDER", "unsupported provider: "+req.Provider)
		return
	}

	testMsg := []llm.Message{{Role: llm.RoleUser, Content: "Say 'OK' for connection test."}}
	resp, err := provider.Complete(ctx, testMsg, llm.CompletionOptions{})
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "TEST_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":   "connected",
		"response": resp.Content,
		"model":    resp.Model,
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
	// Sample simulated / live OTA release payload
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

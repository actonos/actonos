package llm

import (
	"strings"
	"sync"
)

// ModelSpec defines the complete canonical specification for an AI model.
type ModelSpec struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	ProviderID      string  `json:"provider_id"`
	ProviderName    string  `json:"provider_name"`
	Badge           string  `json:"badge,omitempty"`
	ContextWindow   string  `json:"context_window,omitempty"`
	Category        string  `json:"category"` // "Cloud Frontier", "Reasoning", "Ultra Fast", "Aggregator", "Custom"
	PromptPer1M     float64 `json:"prompt_per_1m"`
	CompletionPer1M float64 `json:"completion_per_1m"`
	IsDefault       bool    `json:"is_default,omitempty"`
	SupportsTools   bool    `json:"supports_tools"`
	SupportsVision  bool    `json:"supports_vision"`
}

// ProviderSpec defines the canonical metadata for an AI provider.
type ProviderSpec struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Category       string      `json:"category"`
	Description    string      `json:"description"`
	DefaultBaseURL string      `json:"default_base_url"`
	AccentColor    string      `json:"accent_color"`
	ModelPresets   []ModelSpec `json:"model_presets"`
}

// CatalogResponse represents the full payload returned to API clients.
type CatalogResponse struct {
	Models    []ModelSpec    `json:"models"`
	Providers []ProviderSpec `json:"providers"`
}

var (
	catalogMu          sync.RWMutex
	canonicalProviders []ProviderSpec
	canonicalModels    []ModelSpec
	modelMap           map[string]ModelSpec
)

func init() {
	initCanonicalCatalog()
}

func initCanonicalCatalog() {
	catalogMu.Lock()
	defer catalogMu.Unlock()

	canonicalProviders = []ProviderSpec{
		{
			ID:             "anthropic",
			Name:           "Anthropic Claude",
			Category:       "Cloud Frontier",
			Description:    "Frontier coding, hybrid reasoning, and autonomous multi-agent intelligence.",
			DefaultBaseURL: "https://api.anthropic.com/v1",
			AccentColor:    "#D97706",
			ModelPresets: []ModelSpec{
				{
					ID:              "anthropic/claude-haiku-4-5",
					Name:            "Claude Haiku 4.5",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Ultra Fast & High Efficiency Sub-Agent",
					ContextWindow:   "256k",
					Category:        "Ultra Fast",
					PromptPer1M:     0.80,
					CompletionPer1M: 4.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-sonnet-4-5",
					Name:            "Claude Sonnet 4.5",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Autonomous Engineering Specialist",
					ContextWindow:   "256k",
					Category:        "Cloud Frontier",
					PromptPer1M:     3.00,
					CompletionPer1M: 15.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-sonnet-4-6",
					Name:            "Claude Sonnet 4.6",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Frontier Coding & Multi-Agent Swarm",
					ContextWindow:   "512k",
					Category:        "Cloud Frontier",
					PromptPer1M:     3.00,
					CompletionPer1M: 15.00,
					IsDefault:       true,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-opus-4-5",
					Name:            "Claude Opus 4.5",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Deep Cognitive Reasoning Flagship",
					ContextWindow:   "256k",
					Category:        "Reasoning",
					PromptPer1M:     10.00,
					CompletionPer1M: 40.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-sonnet-5",
					Name:            "Claude Sonnet 5",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Next-Gen Cognitive Architecture",
					ContextWindow:   "1M+",
					Category:        "Cloud Frontier",
					PromptPer1M:     3.50,
					CompletionPer1M: 17.50,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-opus-4-6",
					Name:            "Claude Opus 4.6",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Supreme STEM, Math & System Architecture",
					ContextWindow:   "512k",
					Category:        "Reasoning",
					PromptPer1M:     12.00,
					CompletionPer1M: 50.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-opus-4-7",
					Name:            "Claude Opus 4.7",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Advanced Deliberate Reasoning",
					ContextWindow:   "512k",
					Category:        "Reasoning",
					PromptPer1M:     14.00,
					CompletionPer1M: 55.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-opus-4-8",
					Name:            "Claude Opus 4.8",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Supreme Autonomous Superintelligence",
					ContextWindow:   "1M+",
					Category:        "Reasoning",
					PromptPer1M:     15.00,
					CompletionPer1M: 60.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "anthropic/claude-opus-5",
					Name:            "Claude Opus 5",
					ProviderID:      "anthropic",
					ProviderName:    "Anthropic Claude",
					Badge:           "Peak Frontier Superintelligence Flagship",
					ContextWindow:   "2M+",
					Category:        "Cloud Frontier",
					PromptPer1M:     20.00,
					CompletionPer1M: 80.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
			},
		},
		{
			ID:             "openai",
			Name:           "OpenAI",
			Category:       "Cloud Frontier",
			Description:    "Industry standard GPT-5 generation reasoning and agentic execution.",
			DefaultBaseURL: "https://api.openai.com/v1",
			AccentColor:    "#10B981",
			ModelPresets: []ModelSpec{
				{
					ID:              "openai/gpt-5.4-mini",
					Name:            "GPT-5.4 Mini",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Lightweight Ultra-Fast Multimodal",
					ContextWindow:   "256k",
					Category:        "Ultra Fast",
					PromptPer1M:     0.20,
					CompletionPer1M: 0.80,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5",
					Name:            "GPT-5",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "GPT-5 Flagship Foundation Model",
					ContextWindow:   "256k",
					Category:        "Cloud Frontier",
					PromptPer1M:     2.00,
					CompletionPer1M: 8.00,
					IsDefault:       true,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.1",
					Name:            "GPT-5.1",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Enhanced Code & Tool Calling",
					ContextWindow:   "256k",
					Category:        "Cloud Frontier",
					PromptPer1M:     2.20,
					CompletionPer1M: 8.80,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.2",
					Name:            "GPT-5.2",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Adaptive Multi-Step Reasoning",
					ContextWindow:   "512k",
					Category:        "Cloud Frontier",
					PromptPer1M:     2.50,
					CompletionPer1M: 10.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.4-mini",
					Name:            "GPT-5.4 Mini",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "High-Throughput Sub-Agent Brain",
					ContextWindow:   "256k",
					Category:        "Ultra Fast",
					PromptPer1M:     0.30,
					CompletionPer1M: 1.20,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.4",
					Name:            "GPT-5.4",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Enterprise Cognitive Flagship",
					ContextWindow:   "512k",
					Category:        "Cloud Frontier",
					PromptPer1M:     3.00,
					CompletionPer1M: 12.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.5",
					Name:            "GPT-5.5",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Advanced Multimodal Deep Understanding",
					ContextWindow:   "1M+",
					Category:        "Cloud Frontier",
					PromptPer1M:     3.50,
					CompletionPer1M: 14.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.6-terra",
					Name:            "GPT-5.6 Terra",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Grounding & Large-Scale Data Systems",
					ContextWindow:   "1M+",
					Category:        "Cloud Frontier",
					PromptPer1M:     4.00,
					CompletionPer1M: 16.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openai/gpt-5.6-sol",
					Name:            "GPT-5.6 Sol",
					ProviderID:      "openai",
					ProviderName:    "OpenAI",
					Badge:           "Peak Autonomous Agent Flagship",
					ContextWindow:   "2M+",
					Category:        "Reasoning",
					PromptPer1M:     5.00,
					CompletionPer1M: 20.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
			},
		},
		{
			ID:             "deepseek",
			Name:           "DeepSeek",
			Category:       "Open Weights & Cloud",
			Description:    "DeepSeek-V4 generation high-performance architecture.",
			DefaultBaseURL: "https://api.deepseek.com/v1",
			AccentColor:    "#6366F1",
			ModelPresets: []ModelSpec{
				{
					ID:              "deepseek/deepseek-v4-flash",
					Name:            "DeepSeek-V4 Flash",
					ProviderID:      "deepseek",
					ProviderName:    "DeepSeek",
					Badge:           "Ultra-High Throughput MoE Architecture",
					ContextWindow:   "256k",
					Category:        "Ultra Fast",
					PromptPer1M:     0.10,
					CompletionPer1M: 0.25,
					IsDefault:       true,
					SupportsTools:   true,
					SupportsVision:  false,
				},
				{
					ID:              "deepseek/deepseek-v4-pro",
					Name:            "DeepSeek-V4 Pro",
					ProviderID:      "deepseek",
					ProviderName:    "DeepSeek",
					Badge:           "1M Context MoE Reasoning Leader",
					ContextWindow:   "1M+",
					Category:        "Reasoning",
					PromptPer1M:     0.45,
					CompletionPer1M: 1.80,
					SupportsTools:   true,
					SupportsVision:  false,
				},
			},
		},
		{
			ID:             "grok",
			Name:           "xAI (Grok)",
			Category:       "Real-Time Intelligence",
			Description:    "xAI Grok generation real-time knowledge and frontier reasoning.",
			DefaultBaseURL: "https://api.x.ai/v1",
			AccentColor:    "#EC4899",
			ModelPresets: []ModelSpec{
				{
					ID:              "grok/grok-4.3",
					Name:            "Grok 4.3",
					ProviderID:      "grok",
					ProviderName:    "xAI (Grok)",
					Badge:           "Real-Time Knowledge & Rapid Tool Use",
					ContextWindow:   "256k",
					Category:        "Ultra Fast",
					PromptPer1M:     1.50,
					CompletionPer1M: 6.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "grok/grok-4.5",
					Name:            "Grok 4.5",
					ProviderID:      "grok",
					ProviderName:    "xAI (Grok)",
					Badge:           "Deep Cognitive Reasoning & Coding",
					ContextWindow:   "512k",
					Category:        "Reasoning",
					PromptPer1M:     3.00,
					CompletionPer1M: 12.00,
					IsDefault:       true,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "grok/grok-4.6",
					Name:            "Grok 4.6",
					ProviderID:      "grok",
					ProviderName:    "xAI (Grok)",
					Badge:           "Peak Frontier Realtime Intelligence",
					ContextWindow:   "1M+",
					Category:        "Cloud Frontier",
					PromptPer1M:     4.50,
					CompletionPer1M: 18.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
			},
		},
		{
			ID:             "openrouter",
			Name:           "OpenRouter",
			Category:       "Unified Aggregator",
			Description:    "Universal gateway aggregating Claude, GPT-5, DeepSeek-V4, and Grok.",
			DefaultBaseURL: "https://openrouter.ai/api/v1",
			AccentColor:    "#8B5CF6",
			ModelPresets: []ModelSpec{
				{
					ID:              "openrouter/anthropic/claude-sonnet-5",
					Name:            "Claude Sonnet 5 (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "Frontier Claude via OpenRouter",
					ContextWindow:   "1M+",
					Category:        "Aggregator",
					PromptPer1M:     3.50,
					CompletionPer1M: 17.50,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openrouter/anthropic/claude-opus-5",
					Name:            "Claude Opus 5 (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "Superintelligence via OpenRouter",
					ContextWindow:   "2M+",
					Category:        "Aggregator",
					PromptPer1M:     20.00,
					CompletionPer1M: 80.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openrouter/anthropic/claude-sonnet-4-6",
					Name:            "Claude Sonnet 4.6 (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "Frontier Coding via OpenRouter",
					ContextWindow:   "512k",
					Category:        "Aggregator",
					PromptPer1M:     3.00,
					CompletionPer1M: 15.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openrouter/openai/gpt-5.6-sol",
					Name:            "GPT-5.6 Sol (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "GPT-5.6 Flagship via OpenRouter",
					ContextWindow:   "2M+",
					Category:        "Aggregator",
					PromptPer1M:     5.00,
					CompletionPer1M: 20.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openrouter/openai/gpt-5.5",
					Name:            "GPT-5.5 (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "GPT-5.5 via OpenRouter",
					ContextWindow:   "1M+",
					Category:        "Aggregator",
					PromptPer1M:     3.50,
					CompletionPer1M: 14.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openrouter/openai/gpt-5",
					Name:            "GPT-5 (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "GPT-5 Standard via OpenRouter",
					ContextWindow:   "256k",
					Category:        "Aggregator",
					PromptPer1M:     2.00,
					CompletionPer1M: 8.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
				{
					ID:              "openrouter/deepseek/deepseek-v4-pro",
					Name:            "DeepSeek-V4 Pro (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "DeepSeek-V4 via OpenRouter",
					ContextWindow:   "1M+",
					Category:        "Aggregator",
					PromptPer1M:     0.45,
					CompletionPer1M: 1.80,
					SupportsTools:   true,
					SupportsVision:  false,
				},
				{
					ID:              "openrouter/x-ai/grok-4.6",
					Name:            "Grok 4.6 (OpenRouter)",
					ProviderID:      "openrouter",
					ProviderName:    "OpenRouter",
					Badge:           "xAI Grok via OpenRouter",
					ContextWindow:   "1M+",
					Category:        "Aggregator",
					PromptPer1M:     4.50,
					CompletionPer1M: 18.00,
					SupportsTools:   true,
					SupportsVision:  true,
				},
			},
		},
		{
			ID:             "custom_openai",
			Name:           "Custom OpenAI-Compatible",
			Category:       "Self-Hosted / Gateway",
			Description:    "Connect LM Studio, vLLM, LocalAI, Azure OpenAI, or enterprise gateway.",
			DefaultBaseURL: "http://localhost:8000/v1",
			AccentColor:    "#0EA5E9",
			ModelPresets: []ModelSpec{
				{
					ID:              "custom_openai/default-model",
					Name:            "Default Model",
					ProviderID:      "custom_openai",
					ProviderName:    "Custom Gateway",
					Badge:           "Self-Hosted / Private",
					ContextWindow:   "128k",
					Category:        "Custom",
					PromptPer1M:     0.00,
					CompletionPer1M: 0.00,
					IsDefault:       true,
					SupportsTools:   true,
					SupportsVision:  true,
				},
			},
		},
	}

	// Flatten all models into lookup slice and map
	canonicalModels = make([]ModelSpec, 0, 32)
	modelMap = make(map[string]ModelSpec, 64)

	for _, prov := range canonicalProviders {
		for _, m := range prov.ModelPresets {
			canonicalModels = append(canonicalModels, m)
			modelMap[m.ID] = m
			// Also index clean ID without prefix (e.g. "gpt-5", "claude-sonnet-4-6")
			if idx := strings.Index(m.ID, "/"); idx != -1 {
				cleanID := m.ID[idx+1:]
				modelMap[cleanID] = m
			}
		}
	}
}

// GetCanonicalCatalog returns the full list of canonical models.
func GetCanonicalCatalog() []ModelSpec {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	out := make([]ModelSpec, len(canonicalModels))
	copy(out, canonicalModels)
	return out
}

// GetCanonicalProviders returns all configured provider metadata.
func GetCanonicalProviders() []ProviderSpec {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	out := make([]ProviderSpec, len(canonicalProviders))
	copy(out, canonicalProviders)
	return out
}

// GetCatalogResponse returns the unified catalog response for REST API.
func GetCatalogResponse() CatalogResponse {
	return CatalogResponse{
		Models:    GetCanonicalCatalog(),
		Providers: GetCanonicalProviders(),
	}
}

// GetModelPricing returns the pricing per 1M tokens (prompt, completion) for a model.
func GetModelPricing(modelID string) (prompt1M, compl1M float64) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	cleanID := strings.TrimSpace(modelID)
	if spec, ok := modelMap[cleanID]; ok {
		return spec.PromptPer1M, spec.CompletionPer1M
	}

	// Fuzzy match
	lower := strings.ToLower(cleanID)
	for k, spec := range modelMap {
		if strings.Contains(lower, strings.ToLower(k)) || strings.Contains(strings.ToLower(k), lower) {
			return spec.PromptPer1M, spec.CompletionPer1M
		}
	}

	// Fallback default
	if strings.Contains(lower, "custom") || strings.Contains(lower, "local") {
		return 0.0, 0.0
	}
	return 2.0, 8.0
}

// GetModelSpec retrieves a model specification by ID.
func GetModelSpec(modelID string) *ModelSpec {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	cleanID := strings.TrimSpace(modelID)
	if spec, ok := modelMap[cleanID]; ok {
		cp := spec
		return &cp
	}
	return nil
}

package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

var (
	ErrAllProvidersFailed = errors.New("all LLM providers in cascade failed")
	ErrProviderNotFound   = errors.New("provider not found")
)

// ModelCascadeRouter routes completion and embedding requests through an ordered cascade of providers.
type ModelCascadeRouter struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider
	defaultID string
}

// NewModelCascadeRouter creates a new router instance.
func NewModelCascadeRouter() *ModelCascadeRouter {
	return &ModelCascadeRouter{
		providers: make(map[string]LLMProvider),
	}
}

// Count returns the number of registered providers.
func (r *ModelCascadeRouter) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

// SetDefaultProvider sets the default provider ID.
func (r *ModelCascadeRouter) SetDefaultProvider(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultID = id
}

// RegisterProvider adds an LLMProvider to the router registry.
func (r *ModelCascadeRouter) RegisterProvider(id string, provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[id] = provider
	if r.defaultID == "" || (r.defaultID == "local-stub" && id != "local-stub") {
		r.defaultID = id
	}
}

// GetProvider retrieves a provider by identifier or falls back gracefully.
func (r *ModelCascadeRouter) GetProvider(id string) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Direct key match
	if p, ok := r.providers[id]; ok {
		return p, nil
	}

	// 2. Extract prefix before slash (e.g. "anthropic/claude-sonnet-4.5" -> "anthropic")
	prefix := id
	if idx := strings.Index(id, "/"); idx != -1 {
		prefix = id[:idx]
		if p, ok := r.providers[prefix]; ok {
			return p, nil
		}
	}

	// 3. Fuzzy alias matching
	cleanID := strings.ToLower(prefix)
	for k, p := range r.providers {
		kLower := strings.ToLower(k)
		if kLower == cleanID {
			return p, nil
		}
		if strings.HasPrefix(cleanID, kLower) || strings.HasPrefix(kLower, cleanID) {
			return p, nil
		}
		if (cleanID == "google" && kLower == "gemini") || (cleanID == "gemini" && kLower == "google") {
			return p, nil
		}
	}

	// 4. If an explicit provider ID was requested (e.g. "anthropic/claude" or "openai") but not found,
	// check if a default non-mock provider is available before resorting to anything else
	if r.defaultID != "" && r.defaultID != "local-stub" {
		if p, ok := r.providers[r.defaultID]; ok {
			return p, nil
		}
	}

	// 5. Fallback to mock provider only if no other provider exists
	if p, ok := r.providers["local-stub"]; ok {
		return p, nil
	}

	for _, p := range r.providers {
		return p, nil
	}

	return nil, fmt.Errorf("%w: %s (provider not configured in Settings)", ErrProviderNotFound, id)
}

// CompleteWithCascade attempts execution with primary provider, falling back on error.
func (r *ModelCascadeRouter) CompleteWithCascade(
	ctx context.Context,
	cascadeOrder []string,
	messages []Message,
	opts CompletionOptions,
) (*Response, error) {
	messages = SanitizeMessages(messages)
	if len(cascadeOrder) == 0 {
		r.mu.RLock()
		if r.defaultID != "" {
			cascadeOrder = []string{r.defaultID}
		}
		r.mu.RUnlock()
	}

	var lastErr error
	for _, target := range cascadeOrder {
		provider, err := r.GetProvider(target)
		if err != nil {
			slog.Warn("provider not found in router cascade", "target", target, "error", err)
			lastErr = err
			continue
		}

		// Dynamically assign target model if specified in the target string
		callOpts := opts
		if callOpts.Model == "" && strings.Contains(target, "/") {
			callOpts.Model = target[strings.Index(target, "/")+1:]
		}

		resp, err := provider.Complete(ctx, messages, callOpts)
		if err == nil {
			if target != "" {
				resp.Model = target
			}
			return resp, nil
		}

		slog.Warn("provider failed in cascade, attempting fallback",
			"target", target,
			"error", err,
		)
		lastErr = err
	}

	return nil, fmt.Errorf("%w: last error: %v", ErrAllProvidersFailed, lastErr)
}

// StreamCompleteWithCascade attempts to stream using cascade.
func (r *ModelCascadeRouter) StreamCompleteWithCascade(
	ctx context.Context,
	cascadeOrder []string,
	messages []Message,
	opts CompletionOptions,
) (<-chan StreamChunk, error) {
	messages = SanitizeMessages(messages)
	if len(cascadeOrder) == 0 {
		r.mu.RLock()
		if r.defaultID != "" {
			cascadeOrder = []string{r.defaultID}
		}
		r.mu.RUnlock()
	}

	var lastErr error
	for _, target := range cascadeOrder {
		provider, err := r.GetProvider(target)
		if err != nil {
			lastErr = err
			continue
		}

		callOpts := opts
		if callOpts.Model == "" && strings.Contains(target, "/") {
			callOpts.Model = target[strings.Index(target, "/")+1:]
		}

		ch, err := provider.StreamComplete(ctx, messages, callOpts)
		if err == nil {
			return ch, nil
		}

		slog.Warn("provider stream failed, attempting fallback",
			"target", target,
			"error", err,
		)
		lastErr = err
	}

	return nil, fmt.Errorf("%w: last error: %v", ErrAllProvidersFailed, lastErr)
}

// Embed generates embeddings using the designated or default provider.
func (r *ModelCascadeRouter) Embed(ctx context.Context, providerID string, texts []string) ([][]float32, error) {
	target := providerID
	if target == "" {
		r.mu.RLock()
		target = r.defaultID
		r.mu.RUnlock()
	}

	provider, err := r.GetProvider(target)
	if err != nil {
		return nil, err
	}

	return provider.Embed(ctx, texts)
}

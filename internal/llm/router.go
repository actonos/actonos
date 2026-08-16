package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// RegisterProvider adds an LLMProvider to the router registry.
func (r *ModelCascadeRouter) RegisterProvider(id string, provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[id] = provider
	if r.defaultID == "" {
		r.defaultID = id
	}
}

// GetProvider retrieves a provider by identifier.
func (r *ModelCascadeRouter) GetProvider(id string) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, id)
	}
	return p, nil
}

// CompleteWithCascade attempts execution with primary provider, falling back on error.
func (r *ModelCascadeRouter) CompleteWithCascade(
	ctx context.Context,
	cascadeOrder []string,
	messages []Message,
	opts CompletionOptions,
) (*Response, error) {
	if len(cascadeOrder) == 0 {
		r.mu.RLock()
		if r.defaultID != "" {
			cascadeOrder = []string{r.defaultID}
		}
		r.mu.RUnlock()
	}

	var lastErr error
	for _, providerID := range cascadeOrder {
		provider, err := r.GetProvider(providerID)
		if err != nil {
			slog.Warn("provider not found in router cascade", "provider_id", providerID, "error", err)
			lastErr = err
			continue
		}

		resp, err := provider.Complete(ctx, messages, opts)
		if err == nil {
			return resp, nil
		}

		slog.Warn("provider failed in cascade, attempting fallback",
			"provider_id", providerID,
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
	if len(cascadeOrder) == 0 {
		r.mu.RLock()
		if r.defaultID != "" {
			cascadeOrder = []string{r.defaultID}
		}
		r.mu.RUnlock()
	}

	var lastErr error
	for _, providerID := range cascadeOrder {
		provider, err := r.GetProvider(providerID)
		if err != nil {
			lastErr = err
			continue
		}

		ch, err := provider.StreamComplete(ctx, messages, opts)
		if err == nil {
			return ch, nil
		}

		slog.Warn("provider stream failed, attempting fallback",
			"provider_id", providerID,
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

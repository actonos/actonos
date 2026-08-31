package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrAllProvidersFailed = errors.New("all LLM providers in cascade failed")
	ErrProviderNotFound   = errors.New("provider not found")
)

// IsContextWindowError reports provider errors caused by a prompt that does not
// fit the model context window. Callers should compact observations and retry.
func IsContextWindowError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "context window") ||
		strings.Contains(text, "context_length") ||
		strings.Contains(text, "maximum context length") ||
		strings.Contains(text, "prompt is too long") ||
		strings.Contains(text, "input is too long") ||
		strings.Contains(text, "too many tokens")
}

// TaskKindStats aggregates performance metrics per task type.
type TaskKindStats struct {
	TotalCalls int64 `json:"total_calls"`
	Successes  int64 `json:"successes"`
	Failures   int64 `json:"failures"`
}

// ProviderHealthReport summarizes operational health, latency, and reliability for a provider.
type ProviderHealthReport struct {
	ProviderID       string                   `json:"provider_id"`
	Status           string                   `json:"status"` // "healthy", "degraded", "circuit_tripped"
	TrippedUntil     *time.Time               `json:"tripped_until,omitempty"`
	TotalCalls       int64                    `json:"total_calls"`
	TotalFailures    int64                    `json:"total_failures"`
	P50LatencyMs     int64                    `json:"p50_latency_ms"`
	P95LatencyMs     int64                    `json:"p95_latency_ms"`
	ConsecutiveFails int                      `json:"consecutive_fails"`
	TaskStats        map[TaskKind]TaskKindStats `json:"task_stats,omitempty"`
}

// providerMetricsInternal holds volatile running metrics for a provider.
type providerMetricsInternal struct {
	totalCalls     int64
	totalFailures  int64
	latencySamples []int64 // Latencies in milliseconds (capped at last 50 samples)
	taskStats      map[TaskKind]*TaskKindStats
}

// ModelCascadeRouter routes completion and embedding requests through an ordered, cost/latency-aware cascade.
type ModelCascadeRouter struct {
	mu           sync.RWMutex
	providers    map[string]LLMProvider
	defaultID    string
	failures     map[string]int
	trippedUntil map[string]time.Time
	metrics      map[string]*providerMetricsInternal
}

// NewModelCascadeRouter creates a new router instance.
func NewModelCascadeRouter() *ModelCascadeRouter {
	return &ModelCascadeRouter{
		providers:    make(map[string]LLMProvider),
		failures:     make(map[string]int),
		trippedUntil: make(map[string]time.Time),
		metrics:      make(map[string]*providerMetricsInternal),
	}
}

// Count returns the number of registered providers.
func (r *ModelCascadeRouter) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

// HasRealProvider reports whether any non-stub LLM provider is registered in the cascade router.
func (r *ModelCascadeRouter) HasRealProvider() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id := range r.providers {
		if id != "local-stub" {
			return true
		}
	}
	return false
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
	if _, ok := r.metrics[id]; !ok {
		r.metrics[id] = &providerMetricsInternal{
			taskStats: make(map[TaskKind]*TaskKindStats),
		}
	}
	if r.defaultID == "" || (r.defaultID == "local-stub" && id != "local-stub") {
		r.defaultID = id
	}
}

// GetProvider retrieves a provider by identifier or falls back gracefully.
func (r *ModelCascadeRouter) GetProvider(id string) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Direct key match (explicit local-stub lookups are allowed for tests)
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

	// 4. Default non-mock fallback
	if r.defaultID != "" && r.defaultID != "local-stub" {
		if p, ok := r.providers[r.defaultID]; ok {
			return p, nil
		}
	}

	for k, p := range r.providers {
		if k == "local-stub" || strings.HasPrefix(k, "local-stub/") {
			continue
		}
		return p, nil
	}

	return nil, fmt.Errorf("%w: %s (provider not configured in Settings)", ErrProviderNotFound, id)
}

const (
	cascadeRetriesPerProvider = 2
	circuitBreakerThreshold   = 3
	circuitBreakerHold        = 60 * time.Second
)

func (r *ModelCascadeRouter) providerTripped(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	until, ok := r.trippedUntil[id]
	return ok && time.Now().Before(until)
}

func (r *ModelCascadeRouter) recordProviderResult(id string, kind TaskKind, duration time.Duration, failed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.failures == nil {
		r.failures = make(map[string]int)
	}
	if r.trippedUntil == nil {
		r.trippedUntil = make(map[string]time.Time)
	}
	if r.metrics == nil {
		r.metrics = make(map[string]*providerMetricsInternal)
	}

	met, ok := r.metrics[id]
	if !ok {
		met = &providerMetricsInternal{taskStats: make(map[TaskKind]*TaskKindStats)}
		r.metrics[id] = met
	}

	met.totalCalls++
	if kind == "" {
		kind = TaskKindGeneral
	}
	ts, ok := met.taskStats[kind]
	if !ok {
		ts = &TaskKindStats{}
		met.taskStats[kind] = ts
	}
	ts.TotalCalls++

	// Record latency sample
	latMs := duration.Milliseconds()
	met.latencySamples = append(met.latencySamples, latMs)
	if len(met.latencySamples) > 50 {
		met.latencySamples = met.latencySamples[len(met.latencySamples)-50:]
	}

	if !failed {
		ts.Successes++
		r.failures[id] = 0
		delete(r.trippedUntil, id)
		return
	}

	ts.Failures++
	met.totalFailures++
	r.failures[id]++
	if r.failures[id] >= circuitBreakerThreshold {
		r.trippedUntil[id] = time.Now().Add(circuitBreakerHold)
		slog.Warn("llm circuit breaker tripped", "provider", id, "hold", circuitBreakerHold.String())
	}
}

func (r *ModelCascadeRouter) lastResortIDs(cascadeOrder []string) []string {
	seen := make(map[string]bool)
	for _, id := range cascadeOrder {
		seen[id] = true
		if idx := strings.Index(id, "/"); idx != -1 {
			seen[id[:idx]] = true
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var extra []string
	for _, candidate := range []string{"ollama"} {
		if !seen[candidate] {
			if _, ok := r.providers[candidate]; ok {
				extra = append(extra, candidate)
			}
		}
	}
	return extra
}

// CompleteWithCascade attempts execution with primary provider, retries, circuit-breaker, then last-resort.
func (r *ModelCascadeRouter) CompleteWithCascade(
	ctx context.Context,
	cascadeOrder []string,
	messages []Message,
	opts CompletionOptions,
) (*Response, error) {
	opts = opts.WithDefaults()
	messages = SanitizeMessages(messages)
	if len(cascadeOrder) == 0 {
		r.mu.RLock()
		if r.defaultID != "" && r.defaultID != "local-stub" {
			cascadeOrder = []string{r.defaultID}
		}
		r.mu.RUnlock()
	}

	targets := append([]string{}, cascadeOrder...)
	targets = append(targets, r.lastResortIDs(cascadeOrder)...)

	var lastErr error
	for i, target := range targets {
		if target == "local-stub" {
			continue
		}
		if r.providerTripped(target) {
			lastErr = fmt.Errorf("provider %s circuit-open", target)
			continue
		}
		provider, err := r.GetProvider(target)
		if err != nil {
			slog.Warn("provider not found in router cascade", "target", target, "error", err)
			lastErr = err
			continue
		}

		callOpts := opts
		if callOpts.Model == "" && strings.Contains(target, "/") {
			callOpts.Model = target[strings.Index(target, "/")+1:]
		}

		attempts := 1
		if i == len(targets)-1 {
			attempts = cascadeRetriesPerProvider
		}
		var resp *Response
		for attempt := 0; attempt < attempts; attempt++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			start := time.Now()
			resp, err = provider.Complete(ctx, messages, callOpts)
			dur := time.Since(start)

			if err == nil {
				r.recordProviderResult(target, opts.TaskKind, dur, false)
				if target != "" {
					resp.Model = target
				}
				return resp, nil
			}
			lastErr = err
			r.recordProviderResult(target, opts.TaskKind, dur, true)

			if attempt+1 < cascadeRetriesPerProvider {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
				}
			}
		}
		slog.Warn("provider failed in cascade, attempting fallback",
			"target", target,
			"error", lastErr,
		)
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
	opts = opts.WithDefaults()
	messages = SanitizeMessages(messages)
	if len(cascadeOrder) == 0 {
		r.mu.RLock()
		if r.defaultID != "" && r.defaultID != "local-stub" {
			cascadeOrder = []string{r.defaultID}
		}
		r.mu.RUnlock()
	}

	var lastErr error
	for _, target := range cascadeOrder {
		if target == "local-stub" {
			continue
		}
		provider, err := r.GetProvider(target)
		if err != nil {
			lastErr = err
			continue
		}

		callOpts := opts
		if callOpts.Model == "" && strings.Contains(target, "/") {
			callOpts.Model = target[strings.Index(target, "/")+1:]
		}

		start := time.Now()
		ch, err := provider.StreamComplete(ctx, messages, callOpts)
		if err == nil {
			r.recordProviderResult(target, opts.TaskKind, time.Since(start), false)
			return ch, nil
		}

		r.recordProviderResult(target, opts.TaskKind, time.Since(start), true)
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

// GetHealthReport calculates p50/p95 latency and health state for all providers.
func (r *ModelCascadeRouter) GetHealthReport() []ProviderHealthReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reports := make([]ProviderHealthReport, 0, len(r.providers))
	now := time.Now()

	for id := range r.providers {
		rep := ProviderHealthReport{
			ProviderID: id,
			Status:     "healthy",
			TaskStats:  make(map[TaskKind]TaskKindStats),
		}

		if until, tripped := r.trippedUntil[id]; tripped && now.Before(until) {
			rep.Status = "circuit_tripped"
			tCopy := until
			rep.TrippedUntil = &tCopy
		} else if fails := r.failures[id]; fails > 0 {
			rep.Status = "degraded"
			rep.ConsecutiveFails = fails
		}

		if met, ok := r.metrics[id]; ok {
			rep.TotalCalls = met.totalCalls
			rep.TotalFailures = met.totalFailures

			for k, v := range met.taskStats {
				rep.TaskStats[k] = *v
			}

			if len(met.latencySamples) > 0 {
				sorted := make([]int64, len(met.latencySamples))
				copy(sorted, met.latencySamples)
				sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

				p50Idx := len(sorted) / 2
				p95Idx := int(float64(len(sorted)) * 0.95)
				if p95Idx >= len(sorted) {
					p95Idx = len(sorted) - 1
				}
				rep.P50LatencyMs = sorted[p50Idx]
				rep.P95LatencyMs = sorted[p95Idx]
			}
		}

		reports = append(reports, rep)
	}

	return reports
}

// RunSelfHealthProbe performs a lightweight diagnostic probe across registered providers.
func (r *ModelCascadeRouter) RunSelfHealthProbe(ctx context.Context) map[string]error {
	r.mu.RLock()
	providerIDs := make([]string, 0, len(r.providers))
	for id := range r.providers {
		if id != "local-stub" {
			providerIDs = append(providerIDs, id)
		}
	}
	r.mu.RUnlock()

	results := make(map[string]error)
	testMsg := []Message{{Role: RoleUser, Content: "ping"}}

	for _, id := range providerIDs {
		p, err := r.GetProvider(id)
		if err != nil {
			results[id] = err
			continue
		}
		pCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		start := time.Now()
		_, probeErr := p.Complete(pCtx, testMsg, CompletionOptions{})
		cancel()

		dur := time.Since(start)
		if probeErr != nil {
			r.recordProviderResult(id, TaskKindGeneral, dur, true)
			results[id] = probeErr
		} else {
			r.recordProviderResult(id, TaskKindGeneral, dur, false)
			results[id] = nil
		}
	}

	return results
}

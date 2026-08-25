package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTurnTimeout bounds a single ReAct turn (heartbeat, cron, chat, resume).
	DefaultTurnTimeout = 90 * time.Second
	// HeartbeatTurnTimeout bounds one autonomous pulse including tool calls.
	HeartbeatTurnTimeout = 180 * time.Second
	// cannedSuccessPhrase is the legacy empty-output rewrite; never treat it as completion.
	cannedSuccessPhrase = "Completed requested operations successfully."
	// DefaultMaxConcurrentRuns is the per-agent in-flight cap when the manifest omits one.
	DefaultMaxConcurrentRuns = 2
	// DefaultMaxTokensPerHour is the per-agent hourly token quota when unset.
	DefaultMaxTokensPerHour = 250000
	// DefaultCronJobsPerAgent is the maximum persisted cron jobs an agent may own.
	DefaultCronJobsPerAgent = 8
	maxFailCyclesBeforeBlock = 3
)

var (
	// ErrRunCancelled is returned when an in-flight run is cancelled by an operator signal.
	ErrRunCancelled = errors.New("agent run cancelled")
	// ErrConcurrentRunQuota is returned when an agent already has too many running turns.
	ErrConcurrentRunQuota = errors.New("agent concurrent run quota exhausted")
	// ErrHourlyTokenQuota is returned when the agent exceeded its tokens-per-hour cap.
	ErrHourlyTokenQuota = errors.New("agent hourly token quota exhausted")
)

type executionSourceKey struct{}

// WithExecutionSource tags ctx so approval policy and run records use a real origin
// instead of inferring it from prompt text.
func WithExecutionSource(ctx context.Context, source string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, executionSourceKey{}, source)
}

// ExecutionSource returns the tagged origin or "".
func ExecutionSource(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	source, _ := ctx.Value(executionSourceKey{}).(string)
	return source
}

// IsCannedOrEmptyCompletion reports whether content is empty or the legacy canned
// success phrase and must not complete a mission.
func IsCannedOrEmptyCompletion(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return true
	}
	return strings.EqualFold(trimmed, cannedSuccessPhrase)
}

func resolveTurnTimeout(ctx context.Context, fallback time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	if fallback <= 0 {
		fallback = DefaultTurnTimeout
	}
	return context.WithTimeout(ctx, fallback)
}

type inFlightRegistry struct {
	mu     sync.Mutex
	byRun  map[string]context.CancelFunc
	byAgent map[string]map[string]struct{}
}

func newInFlightRegistry() *inFlightRegistry {
	return &inFlightRegistry{
		byRun:   make(map[string]context.CancelFunc),
		byAgent: make(map[string]map[string]struct{}),
	}
}

func (r *inFlightRegistry) track(agentID, runID string, cancel context.CancelFunc) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byAgent[agentID] == nil {
		r.byAgent[agentID] = make(map[string]struct{})
	}
	r.byAgent[agentID][runID] = struct{}{}
	r.byRun[runID] = cancel
	return nil
}

func (r *inFlightRegistry) untrack(agentID, runID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byRun, runID)
	if set, ok := r.byAgent[agentID]; ok {
		delete(set, runID)
		if len(set) == 0 {
			delete(r.byAgent, agentID)
		}
	}
}

func (r *inFlightRegistry) count(agentID string) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byAgent[agentID])
}

func (r *inFlightRegistry) cancelRun(runID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	cancel, ok := r.byRun[runID]
	r.mu.Unlock()
	if !ok || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (r *inFlightRegistry) cancelAgent(agentID string) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	ids := make([]string, 0, len(r.byAgent[agentID]))
	for id := range r.byAgent[agentID] {
		ids = append(ids, id)
	}
	cancels := make([]context.CancelFunc, 0, len(ids))
	for _, id := range ids {
		if c := r.byRun[id]; c != nil {
			cancels = append(cancels, c)
		}
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	return len(cancels)
}

func recoverAsError(agentID string, rec any) error {
	return fmt.Errorf("agent %s panic: %v", agentID, rec)
}

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

// ReflectionEngine runs asynchronous reflection routines to extract insights and preferences.
type ReflectionEngine struct {
	profileMgr *UserProfileManager
	hybridMem  *memory.HybridEngine
	llmRouter  *llm.ModelCascadeRouter
	bus        *bus.EventBus
	stopCh     chan struct{}
}

// NewReflectionEngine creates a new ReflectionEngine.
func NewReflectionEngine(
	profileMgr *UserProfileManager,
	hybridMem *memory.HybridEngine,
	llmRouter *llm.ModelCascadeRouter,
	bus *bus.EventBus,
) *ReflectionEngine {
	return &ReflectionEngine{
		profileMgr: profileMgr,
		hybridMem:  hybridMem,
		llmRouter:  llmRouter,
		bus:        bus,
		stopCh:     make(chan struct{}),
	}
}

// Start launches the background reflection worker.
func (r *ReflectionEngine) Start(ctx context.Context) {
	go r.reflectionLoop(ctx)
}

func (r *ReflectionEngine) reflectionLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.RunReflectionCycle(ctx)
		}
	}
}

// RunReflectionCycle processes recent conversation episodes and consolidates memory.
func (r *ReflectionEngine) RunReflectionCycle(ctx context.Context) {
	slog.Debug("starting memory reflection cycle...")
}

// ReflectOnConversation extracts user preferences immediately after an agent session.
func (r *ReflectionEngine) ReflectOnConversation(ctx context.Context, agentID, userMessage, assistantResponse string) {
	go func() {
		reflectCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		prompt := fmt.Sprintf(`Analyze the following user-assistant exchange.
Extract any stated user preferences, coding conventions, or workflow habits.
If found, return JSON: {"key": "preference_name", "value": "preference_value"}. If none, return {}.

User: %s
Assistant: %s`, userMessage, assistantResponse)

		messages := []llm.Message{
			{Role: "user", Content: prompt},
		}

		temp := 0.1
		opts := llm.CompletionOptions{
			Temperature: &temp,
		}

		resp, err := r.llmRouter.CompleteWithCascade(reflectCtx, []string{"anthropic/claude-3-7-sonnet", "google/gemini-2.5-flash"}, messages, opts)
		if err != nil {
			return
		}

		var result struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &result); err == nil && result.Key != "" {
			profile := r.profileMgr.GetProfile()
			if profile.Preferences == nil {
				profile.Preferences = make(map[string]string)
			}
			profile.Preferences[result.Key] = result.Value
			_ = r.profileMgr.UpdateProfile(reflectCtx, profile)
			slog.Info("async reflection saved user preference", "key", result.Key, "value", result.Value)
		}
	}()
}

// Stop terminates the reflection engine.
func (r *ReflectionEngine) Stop() {
	close(r.stopCh)
}

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
	slog.Debug("running periodic memory reflection cycle...")
}

// ReflectOnConversation extracts user preferences and episodic memory reflections after an agent session.
func (r *ReflectionEngine) ReflectOnConversation(ctx context.Context, agentID, userMessage, assistantResponse string) {
	userMsg := strings.TrimSpace(userMessage)
	asstResp := strings.TrimSpace(assistantResponse)
	if userMsg == "" || asstResp == "" {
		return
	}
	if agentID == "" {
		agentID = DefaultSystemAgentID
	}

	// Skip trivial zero-noise pulses
	if strings.Contains(asstResp, "HEARTBEAT_OK") || strings.HasPrefix(asstResp, "⏰ [") {
		return
	}

	go func() {
		reflectCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		prompt := fmt.Sprintf(`You are the ActonOS Memory & Reflection Daemon.
Analyze the following user-assistant exchange.
Synthesize:
1. "preference_key" & "preference_value": Any user habit, coding style preference, communication preference, or workflow directive (if none, leave empty "").
2. "episodic_memory": A concise 1-2 sentence summary of key decisions, facts, solutions, or project context learned in this exchange that should be remembered for future interactions (if trivial greeting/chit-chat with zero lasting value, leave empty "").

Exchange:
User: %s
Assistant: %s

Respond STRICTLY with valid JSON:
{
  "preference_key": "",
  "preference_value": "",
  "episodic_memory": ""
}`, userMsg, asstResp)

		messages := []llm.Message{
			{Role: "user", Content: prompt},
		}

		temp := 0.1
		opts := llm.CompletionOptions{
			Temperature: &temp,
		}

		if r.llmRouter == nil {
			return
		}

		resp, err := r.llmRouter.CompleteWithCascade(reflectCtx, nil, messages, opts)
		if err != nil {
			slog.Debug("async reflection completion error", "agent_id", agentID, "error", err)
			return
		}

		cleanJSON := strings.TrimSpace(resp.Content)
		if idx := strings.Index(cleanJSON, "{"); idx != -1 {
			if lastIdx := strings.LastIndex(cleanJSON, "}"); lastIdx > idx {
				cleanJSON = cleanJSON[idx : lastIdx+1]
			}
		}

		var result struct {
			PrefKey        string `json:"preference_key"`
			PrefVal        string `json:"preference_value"`
			EpisodicMemory string `json:"episodic_memory"`
		}
		if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
			return
		}

		// 1. Update user preferences if found
		if result.PrefKey != "" && result.PrefVal != "" && r.profileMgr != nil {
			profile := r.profileMgr.GetProfile()
			if profile.Preferences == nil {
				profile.Preferences = make(map[string]string)
			}
			profile.Preferences[result.PrefKey] = result.PrefVal
			_ = r.profileMgr.UpdateProfile(reflectCtx, profile)
			slog.Info("async reflection saved user preference", "key", result.PrefKey, "value", result.PrefVal)
		}

		// 2. Append to MEMORY.md and index into Hybrid Episodic Memory
		if result.EpisodicMemory != "" {
			if r.profileMgr != nil {
				_ = r.profileMgr.AppendAgentMemoryMD(reflectCtx, agentID, result.EpisodicMemory)
				slog.Info("async reflection saved episodic memory diary", "agent_id", agentID, "memory", result.EpisodicMemory)
			}
			if r.hybridMem != nil {
				_, _ = r.hybridMem.StoreMemory(reflectCtx, agentID, memory.LayerEpisodic, result.EpisodicMemory, nil, map[string]any{
					"source":     "reflection",
					"agent_id":   agentID,
					"created_at": time.Now().UTC().Format(time.RFC3339),
				}, 1.0)
			}
		}
	}()
}

// Stop terminates the reflection engine.
func (r *ReflectionEngine) Stop() {
	close(r.stopCh)
}

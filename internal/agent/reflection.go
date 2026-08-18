package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
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
	if r.hybridMem == nil || r.hybridMem.DB() == nil {
		return
	}
	db := r.hybridMem.DB().SQLDB()
	// Keep the newest copy of identical episodic memories per agent.
	if _, err := db.ExecContext(ctx, `
		DELETE FROM memories
		WHERE rowid NOT IN (
			SELECT MAX(rowid) FROM memories
			WHERE layer = ? GROUP BY agent_id, content
		) AND layer = ?
	`, memory.LayerEpisodic, memory.LayerEpisodic); err != nil {
		slog.Warn("memory reflection deduplication failed", "error", err)
	}
	// Remove stale, low-value episodes while preserving frequently accessed facts.
	cutoff := time.Now().UTC().AddDate(0, -6, 0)
	if _, err := db.ExecContext(ctx, `
		DELETE FROM memories
		WHERE layer = ? AND importance_weight < 0.35
		  AND access_count = 0 AND created_at < ?
	`, memory.LayerEpisodic, cutoff); err != nil {
		slog.Warn("memory reflection retention cleanup failed", "error", err)
	}
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
	userMsg = redactReflectionSecrets(userMsg)
	asstResp = redactReflectionSecrets(asstResp)
	if len(userMsg) > 12000 {
		userMsg = userMsg[:12000]
	}
	if len(asstResp) > 12000 {
		asstResp = asstResp[:12000]
	}

	// Skip automated heartbeat, cron, or directive runs
	if strings.Contains(userMsg, "[AUTONOMOUS") ||
		strings.Contains(userMsg, "[Heartbeat") ||
		strings.Contains(userMsg, "Standing Directives") ||
		strings.Contains(asstResp, "HEARTBEAT_OK") ||
		strings.HasPrefix(asstResp, "⏰ [") {
		return
	}

	if r.llmRouter == nil || !r.llmRouter.HasRealProvider() {
		return
	}

	go func() {
		reflectCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		prompt := fmt.Sprintf(`You are the ActonOS Memory & Reflection Daemon.
Analyze the following user-assistant exchange.
Synthesize:
1. "preference_key" & "preference_value": Genuine user preferences, coding styles, language habits (NEVER system directives, heartbeat rules, or bot standing tasks). If none, leave empty "".
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

		// 1. Update user preferences if found and not a system directive
		isSystemDirectiveKey := strings.Contains(result.PrefKey, "heartbeat") ||
			strings.Contains(result.PrefKey, "autonomous") ||
			strings.Contains(result.PrefKey, "standing") ||
			strings.Contains(result.PrefKey, "directive") ||
			strings.Contains(result.PrefKey, "task") ||
			strings.Contains(result.PrefKey, "mythology")

		if result.PrefKey != "" && result.PrefVal != "" && !isSystemDirectiveKey && r.profileMgr != nil {
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

var reflectionSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|secret|password)\s*[:=]\s*["']?[^\s"',;]+`),
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
}

func redactReflectionSecrets(value string) string {
	redacted := value
	for _, pattern := range reflectionSecretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED_SECRET]")
	}
	return redacted
}

// Stop terminates the reflection engine.
func (r *ReflectionEngine) Stop() {
	close(r.stopCh)
}

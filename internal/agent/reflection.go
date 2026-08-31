package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

// SelfImprovementProposal represents an actionable insight or proposal derived from self-review.
type SelfImprovementProposal struct {
	ID          string     `json:"id"`
	AgentID     string     `json:"agent_id"`
	Category    string     `json:"category"` // "tool_reliability", "prompt_clarity", "task_failure", "performance"
	Title       string     `json:"title"`
	Observation string     `json:"observation"`
	Suggestion  string     `json:"suggestion"`
	Status      string     `json:"status"` // "pending", "applied", "dismissed"
	CreatedAt   time.Time  `json:"created_at"`
	AppliedAt   *time.Time `json:"applied_at,omitempty"`
}

// ReflectionEngine runs asynchronous reflection routines to extract insights, preferences, and self-review proposals.
type ReflectionEngine struct {
	mu         sync.RWMutex
	profileMgr *UserProfileManager
	hybridMem  *memory.HybridEngine
	llmRouter  *llm.ModelCascadeRouter
	bus        *bus.EventBus
	db         *sql.DB
	dataDir    string
	stopCh     chan struct{}
}

// NewReflectionEngine creates a new ReflectionEngine.
func NewReflectionEngine(
	profileMgr *UserProfileManager,
	hybridMem *memory.HybridEngine,
	llmRouter *llm.ModelCascadeRouter,
	bus *bus.EventBus,
) *ReflectionEngine {
	r := &ReflectionEngine{
		profileMgr: profileMgr,
		hybridMem:  hybridMem,
		llmRouter:  llmRouter,
		bus:        bus,
		dataDir:    "./data",
		stopCh:     make(chan struct{}),
	}
	if hybridMem != nil && hybridMem.DB() != nil {
		r.db = hybridMem.DB().SQLDB()
		r.ensureSchema()
	}
	return r
}

// SetDB attaches the SQLite database handle directly.
func (r *ReflectionEngine) SetDB(db *sql.DB) {
	r.mu.Lock()
	r.db = db
	r.mu.Unlock()
	r.ensureSchema()
}

// SetDataDir sets the persistent data root.
func (r *ReflectionEngine) SetDataDir(dataDir string) {
	r.mu.Lock()
	r.dataDir = dataDir
	r.mu.Unlock()
}

func (r *ReflectionEngine) ensureSchema() {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()

	if db == nil {
		return
	}

	schema := `
	CREATE TABLE IF NOT EXISTS self_improvement_proposals (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		observation TEXT NOT NULL DEFAULT '',
		suggestion TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TIMESTAMP NOT NULL,
		applied_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_proposals_agent ON self_improvement_proposals(agent_id, status);
	`
	_, _ = db.Exec(schema)
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
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()

	if db == nil && r.hybridMem != nil && r.hybridMem.DB() != nil {
		db = r.hybridMem.DB().SQLDB()
	}
	if db == nil {
		return
	}

	// Keep the newest copy of identical episodic memories per agent unless pinned.
	if _, err := db.ExecContext(ctx, `
		DELETE FROM memories
		WHERE rowid NOT IN (
			SELECT MAX(rowid) FROM memories
			WHERE layer = ? GROUP BY agent_id, content
		) AND layer = ? AND pinned = 0 AND user_pinned = 0
	`, memory.LayerEpisodic, memory.LayerEpisodic); err != nil {
		slog.Warn("memory reflection deduplication failed", "error", err)
	}

	// 1. Demote low-importance items older than 14 days without access
	demoteCutoff := time.Now().UTC().AddDate(0, 0, -14)
	if _, err := db.ExecContext(ctx, `
		UPDATE memories
		SET demoted_at = CURRENT_TIMESTAMP
		WHERE importance = 'low'
		  AND pinned = 0 AND user_pinned = 0
		  AND access_count = 0 AND created_at < ?
		  AND demoted_at IS NULL
	`, demoteCutoff); err != nil {
		slog.Warn("memory reflection demotion failed", "error", err)
	}

	// 2. Remove stale, unprotected episodes (never delete pinned or critical/user_preference memories)
	cutoff := time.Now().UTC().AddDate(0, -6, 0)
	if _, err := db.ExecContext(ctx, `
		DELETE FROM memories
		WHERE layer = ?
		  AND pinned = 0 AND user_pinned = 0
		  AND importance NOT IN ('critical', 'user_preference')
		  AND importance_weight < 0.35
		  AND access_count = 0 AND created_at < ?
	`, memory.LayerEpisodic, cutoff); err != nil {
		slog.Warn("memory reflection retention cleanup failed", "error", err)
	}
}

// RunSelfReviewCycle analyzes runs from the past 24 hours, identifies bottlenecks, and generates self-improvement proposals.
func (r *ReflectionEngine) RunSelfReviewCycle(ctx context.Context, targetAgentID string) ([]SelfImprovementProposal, error) {
	r.mu.RLock()
	db := r.db
	dataDir := r.dataDir
	r.mu.RUnlock()

	if db == nil && r.hybridMem != nil && r.hybridMem.DB() != nil {
		db = r.hybridMem.DB().SQLDB()
	}
	if db == nil {
		return nil, errors.New("database is not initialized for self review")
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	agentFilter := ""
	var args []any
	args = append(args, cutoff)

	if targetAgentID != "" && targetAgentID != "all" {
		agentFilter = " AND agent_id = ?"
		args = append(args, targetAgentID)
	}

	// 1. Query run failure metrics
	queryRuns := fmt.Sprintf(`
		SELECT agent_id, COUNT(*) as total_runs,
		       SUM(CASE WHEN status IN ('failed', 'error') THEN 1 ELSE 0 END) as failed_runs
		FROM agent_runs
		WHERE started_at > ? %s
		GROUP BY agent_id
	`, agentFilter)

	rows, err := db.QueryContext(ctx, queryRuns, args...)
	if err != nil {
		return nil, fmt.Errorf("querying agent runs for self review: %w", err)
	}
	defer rows.Close()

	var proposals []SelfImprovementProposal

	for rows.Next() {
		var agentID string
		var totalRuns, failedRuns int
		if err := rows.Scan(&agentID, &totalRuns, &failedRuns); err != nil {
			continue
		}

		if totalRuns > 0 && failedRuns > 0 {
			failRate := float64(failedRuns) / float64(totalRuns)
			if failRate >= 0.20 || failedRuns >= 2 {
				p := SelfImprovementProposal{
					ID:          newProposalID(),
					AgentID:     agentID,
					Category:    "task_failure",
					Title:       fmt.Sprintf("High Mission Failure Rate (%.0f%%)", failRate*100),
					Observation: fmt.Sprintf("Agent encountered %d failures out of %d total execution runs in the last 24h.", failedRuns, totalRuns),
					Suggestion:  "Add pre-execution prerequisite checks and tighten input acceptance criteria.",
					Status:      "pending",
					CreatedAt:   time.Now().UTC(),
				}
				proposals = append(proposals, p)
			}
		}
	}

	// 2. Query tool error patterns from run_events
	queryEvents := fmt.Sprintf(`
		SELECT agent_id, COUNT(*) as tool_errors
		FROM run_events
		WHERE event_type = 'tool_failed' AND timestamp > ? %s
		GROUP BY agent_id
	`, agentFilter)

	evRows, err := db.QueryContext(ctx, queryEvents, args...)
	if err == nil {
		defer evRows.Close()
		for evRows.Next() {
			var agentID string
			var toolErrors int
			if err := evRows.Scan(&agentID, &toolErrors); err == nil && toolErrors >= 3 {
				p := SelfImprovementProposal{
					ID:          newProposalID(),
					AgentID:     agentID,
					Category:    "tool_reliability",
					Title:       fmt.Sprintf("Frequent Tool Failures (%d failures)", toolErrors),
					Observation: fmt.Sprintf("Agent experienced %d failed tool calls in the last 24 hours.", toolErrors),
					Suggestion:  "Verify tool parameter formatting and check network connectivity or authentication tokens for external tools.",
					Status:      "pending",
					CreatedAt:   time.Now().UTC(),
				}
				proposals = append(proposals, p)
			}
		}
	}

	// Persist proposals and append to INSIGHTS.md
	for _, p := range proposals {
		_, _ = db.ExecContext(ctx, `
			INSERT INTO self_improvement_proposals (
				id, agent_id, category, title, observation, suggestion, status, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, p.ID, p.AgentID, p.Category, p.Title, p.Observation, p.Suggestion, p.Status, p.CreatedAt)

		r.appendInsightsMD(dataDir, p)
		if r.bus != nil {
			r.bus.Publish(bus.NewEvent("agent:insight_proposed", p.AgentID, map[string]any{
				"proposal": p,
			}))
		}
	}

	return proposals, nil
}

func (r *ReflectionEngine) appendInsightsMD(dataDir string, p SelfImprovementProposal) {
	if dataDir == "" {
		dataDir = "./data"
	}
	agentDir := filepath.Join(dataDir, "agents", p.AgentID)
	_ = os.MkdirAll(agentDir, 0755)
	insightsFile := filepath.Join(agentDir, "INSIGHTS.md")

	header := ""
	if _, err := os.Stat(insightsFile); os.IsNotExist(err) {
		header = "# Agent Self-Improvement Insights\n\n> Auto-generated by ActonOS ReflectionEngine.\n\n"
	}

	entry := fmt.Sprintf("### [%s] %s — %s\n- **Category**: `%s`\n- **Observation**: %s\n- **Suggestion**: %s\n- **Status**: `%s`\n\n",
		p.CreatedAt.Format("2006-01-02 15:04:05 UTC"), p.ID, p.Title, p.Category, p.Observation, p.Suggestion, p.Status)

	f, err := os.OpenFile(insightsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		if header != "" {
			_, _ = f.WriteString(header)
		}
		_, _ = f.WriteString(entry)
	}
}

// ListProposals queries self-improvement proposals for an agent.
func (r *ReflectionEngine) ListProposals(ctx context.Context, agentID, status string) ([]SelfImprovementProposal, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()

	if db == nil {
		return []SelfImprovementProposal{}, nil
	}

	query := `
	SELECT id, agent_id, category, title, observation, suggestion, status, created_at, applied_at
	FROM self_improvement_proposals
	`
	var conditions []string
	var args []any

	if agentID != "" && agentID != "all" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, agentID)
	}
	if status != "" && status != "all" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying proposals: %w", err)
	}
	defer rows.Close()

	var list []SelfImprovementProposal
	for rows.Next() {
		var p SelfImprovementProposal
		var appliedAt sql.NullTime
		if err := rows.Scan(
			&p.ID, &p.AgentID, &p.Category, &p.Title, &p.Observation,
			&p.Suggestion, &p.Status, &p.CreatedAt, &appliedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning proposal: %w", err)
		}
		if appliedAt.Valid {
			p.AppliedAt = &appliedAt.Time
		}
		list = append(list, p)
	}
	if list == nil {
		list = []SelfImprovementProposal{}
	}
	return list, rows.Err()
}

// ApplyProposal marks a proposal applied and saves the learned rule to the agent's memory.
func (r *ReflectionEngine) ApplyProposal(ctx context.Context, proposalID string) error {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()

	if db == nil {
		return errors.New("database unavailable")
	}

	var p SelfImprovementProposal
	err := db.QueryRowContext(ctx, `
		SELECT id, agent_id, category, title, observation, suggestion, status, created_at
		FROM self_improvement_proposals WHERE id = ?
	`, proposalID).Scan(&p.ID, &p.AgentID, &p.Category, &p.Title, &p.Observation, &p.Suggestion, &p.Status, &p.CreatedAt)
	if err != nil {
		return fmt.Errorf("proposal not found: %w", err)
	}

	now := time.Now().UTC()
	_, err = db.ExecContext(ctx, "UPDATE self_improvement_proposals SET status = 'applied', applied_at = ? WHERE id = ?", now, proposalID)
	if err != nil {
		return fmt.Errorf("updating proposal status: %w", err)
	}

	// Append rule to agent episodic memory as durable reflection learning
	if r.profileMgr != nil && p.Suggestion != "" {
		_ = r.profileMgr.AppendAgentMemoryMD(ctx, p.AgentID, fmt.Sprintf("Applied Self-Improvement (%s): %s", p.Title, p.Suggestion))
	}

	return nil
}

// DismissProposal marks a proposal dismissed.
func (r *ReflectionEngine) DismissProposal(ctx context.Context, proposalID string) error {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()

	if db == nil {
		return errors.New("database unavailable")
	}

	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, "UPDATE self_improvement_proposals SET status = 'dismissed', applied_at = ? WHERE id = ?", now, proposalID)
	if err != nil {
		return fmt.Errorf("dismissing proposal: %w", err)
	}
	return nil
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

	// Skip silent acknowledgements; mission transcripts are reflected after redaction.
	if strings.Contains(asstResp, "HEARTBEAT_OK") || strings.HasPrefix(asstResp, "⏰ [") {
		return
	}

	if r.llmRouter == nil || !r.llmRouter.HasRealProvider() {
		return
	}

	go func() {
		reflectCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		prompt := BuildReflectionPrompt(userMsg, asstResp)

		messages := []llm.Message{
			{Role: "user", Content: prompt},
		}

		opts := llm.CompletionOptions{
			ReasoningEffort: llm.DefaultReasoningEffort,
		}

		if r.llmRouter == nil {
			return
		}

		resp, err := r.llmRouter.CompleteWithCascade(reflectCtx, nil, messages, opts)
		if err != nil {
			slog.Debug("async reflection completion error", "agent_id", agentID, "error", err)
			return
		}

		var result struct {
			PrefKey        string `json:"preference_key"`
			PrefVal        string `json:"preference_value"`
			EpisodicMemory string `json:"episodic_memory"`
		}
		if err := ExtractAndUnmarshalJSON(resp.Content, &result); err != nil {
			slog.Debug("failed to parse reflection JSON", "error", err, "raw", resp.Content)
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

func newProposalID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "prop_" + hex.EncodeToString(b)
}

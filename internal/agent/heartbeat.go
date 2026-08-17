package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

// HeartbeatRun represents an execution record of a proactive heartbeat cycle.
type HeartbeatRun struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id"`
	ExecutedAt time.Time `json:"executed_at"`
	Status     string    `json:"status"` // "ok", "action_taken", "error"
	Summary    string    `json:"summary"`
	TokensUsed int       `json:"tokens_used"`
}

// HeartbeatDaemon monitors proactive trigger rules, executes cognitive self-driving checks, and manages autonomous agent pulse.
type HeartbeatDaemon struct {
	mu           sync.RWMutex
	agentMgr     *AgentManager
	engine       *Engine
	eventBus     *bus.EventBus
	db           *sql.DB
	workspaceDir string
	interval     time.Duration
	stopCh       chan struct{}
	lastRun      time.Time
	running      bool
}

// NewHeartbeatDaemon creates a new HeartbeatDaemon.
func NewHeartbeatDaemon(
	agentMgr *AgentManager,
	engine *Engine,
	eventBus *bus.EventBus,
	db *sql.DB,
	workspaceDir string,
	interval time.Duration,
) *HeartbeatDaemon {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if workspaceDir == "" {
		workspaceDir = "./data/workspace"
	}
	return &HeartbeatDaemon{
		agentMgr:     agentMgr,
		engine:       engine,
		eventBus:     eventBus,
		db:           db,
		workspaceDir: workspaceDir,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// Start launches the autonomous heartbeat evaluation loop.
func (h *HeartbeatDaemon) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	slog.Info("autonomous heartbeat daemon started", "interval", h.interval.String())
	go h.loop(ctx)
}

func (h *HeartbeatDaemon) loop(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkCycle(ctx)
		}
	}
}

// checkCycle runs the autonomous cognitive heartbeat iteration.
func (h *HeartbeatDaemon) checkCycle(ctx context.Context) {
	h.mu.Lock()
	h.lastRun = time.Now().UTC()
	h.mu.Unlock()

	agents, err := h.agentMgr.List(ctx)
	if err != nil || len(agents) == 0 {
		return
	}

	// Check workspace tasks
	heartbeatMDPath := filepath.Join(h.workspaceDir, "HEARTBEAT.md")
	tasksMDPath := filepath.Join(h.workspaceDir, "TASKS.md")

	workspaceTaskContext := ""
	if data, err := os.ReadFile(heartbeatMDPath); err == nil && len(data) > 0 {
		workspaceTaskContext += fmt.Sprintf("\n[Workspace HEARTBEAT.md Instructions]:\n%s\n", string(data))
	}
	if data, err := os.ReadFile(tasksMDPath); err == nil && len(data) > 0 {
		workspaceTaskContext += fmt.Sprintf("\n[Workspace TASKS.md Active Backlog]:\n%s\n", string(data))
	}

	for _, ag := range agents {
		if ag.Status != StatusActive {
			continue
		}

		// Only evaluate agents designated for autonomous operation or the primary core agent
		isPrimary := ag.AgentID == "agent_system_core" || ag.IsSystem
		hasRules := len(ag.TriggerRules) > 0
		hasTasks := workspaceTaskContext != ""

		if !isPrimary && !hasRules && !hasTasks {
			continue
		}

		prompt := fmt.Sprintf(
			"[AUTONOMOUS HEARTBEAT BRAIN CYCLE]\nCurrent UTC Time: %s\n%s\n"+
				"Evaluate system health, pending background tasks, or reminders. "+
				"If everything is nominal and no proactive action or user notification is needed, reply exactly 'HEARTBEAT_OK'. "+
				"Otherwise, perform necessary actions using authorized tools and provide a clear summary of what was performed.",
			time.Now().UTC().Format(time.RFC3339),
			workspaceTaskContext,
		)

		resp, execErr := h.engine.ExecuteStepWithHistory(ctx, ag.AgentID, prompt, nil)

		run := HeartbeatRun{
			AgentID:    ag.AgentID,
			ExecutedAt: time.Now().UTC(),
		}

		b := make([]byte, 8)
		_, _ = rand.Read(b)
		run.ID = "hb_" + hex.EncodeToString(b)

		if execErr != nil {
			run.Status = "error"
			run.Summary = execErr.Error()
			slog.Warn("heartbeat execution failed", "agent_id", ag.AgentID, "error", execErr)
		} else if resp != nil {
			run.TokensUsed = resp.Usage.TotalTokens
			trimmed := strings.TrimSpace(resp.Content)

			if strings.Contains(trimmed, "HEARTBEAT_OK") || trimmed == "" {
				run.Status = "ok"
				run.Summary = "System nominal. No proactive user notification required."
				slog.Debug("heartbeat nominal (zero noise)", "agent_id", ag.AgentID)
			} else {
				run.Status = "action_taken"
				run.Summary = trimmed
				slog.Info("heartbeat performed proactive action", "agent_id", ag.AgentID, "summary_len", len(trimmed))

				// Proactively notify user through event bus
				if h.eventBus != nil {
					h.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, ag.AgentID, map[string]any{
						"type":              "proactive_cron_notification",
						"job_name":          "Proactive Heartbeat Pulse",
						"content":           trimmed,
						"target_channel":    "all",
						"target_account_id": "all",
						"target_recipient":  "",
					}))
				}
			}
		}

		h.recordRun(run)
	}
}

func (h *HeartbeatDaemon) recordRun(run HeartbeatRun) {
	if h.db == nil {
		return
	}

	query := `
	INSERT INTO heartbeat_runs (id, agent_id, executed_at, status, summary, tokens_used)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := h.db.Exec(query, run.ID, run.AgentID, run.ExecutedAt, run.Status, run.Summary, run.TokensUsed)
	if err != nil {
		slog.Warn("failed to record heartbeat run in sqlite", "error", err)
	}
}

// GetRecentRuns returns the most recent heartbeat runs from SQLite.
func (h *HeartbeatDaemon) GetRecentRuns(ctx context.Context, limit int) ([]HeartbeatRun, error) {
	if h.db == nil {
		return []HeartbeatRun{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT id, agent_id, executed_at, status, summary, tokens_used
		FROM heartbeat_runs
		ORDER BY executed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []HeartbeatRun
	for rows.Next() {
		var r HeartbeatRun
		if err := rows.Scan(&r.ID, &r.AgentID, &r.ExecutedAt, &r.Status, &r.Summary, &r.TokensUsed); err == nil {
			runs = append(runs, r)
		}
	}
	return runs, nil
}

// Stop gracefully terminates the heartbeat daemon.
func (h *HeartbeatDaemon) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	h.running = false
	close(h.stopCh)
	slog.Info("autonomous heartbeat daemon stopped")
}

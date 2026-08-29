package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/system"
)

// SystemAnomaly represents a proactively detected system anomaly or health warning.
type SystemAnomaly struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`     // "disk_usage", "cert_expiry", "embedding_queue", "mcp_error", "task_stalled", "token_budget", "inbound_queue"
	Severity        string          `json:"severity"` // "info", "warning", "critical"
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	SuggestedAction string          `json:"suggested_action"`
	AutoTaskPayload *AutonomousTask `json:"auto_task_payload,omitempty"`
	DetectedAt      time.Time       `json:"detected_at"`
	Status          string          `json:"status"` // "active", "resolved", "ignored"
	ResolvedAt      *time.Time      `json:"resolved_at,omitempty"`
}

// ProactiveConfig defines configuration parameters for proactive anomaly detection.
type ProactiveConfig struct {
	Enabled              bool    `json:"enabled"`
	ScanIntervalMinutes  int     `json:"scan_interval_minutes"`
	AutoCreateTasks      bool    `json:"auto_create_tasks"`
	DiskThresholdPercent float64 `json:"disk_threshold_percent"` // e.g. 80.0
	GlobalKillSwitch     bool    `json:"global_kill_switch"`
}

// ProactiveEngine coordinates proactive health probes and autonomous mission suggestions.
type ProactiveEngine struct {
	mu                  sync.RWMutex
	db                  *sql.DB
	dataDir             string
	taskMgr             *TaskManager
	eventBus            *bus.EventBus
	cfg                 ProactiveConfig
	mcpChecker          func(ctx context.Context) ([]string, error)
	tokenBudgetChecker  func(ctx context.Context) (used, cap int64, err error)
	inboundQueueChecker func(ctx context.Context) (int, error)
}

// NewProactiveEngine initializes a new ProactiveEngine.
func NewProactiveEngine(db *sql.DB, dataDir string, taskMgr *TaskManager, eventBus *bus.EventBus) *ProactiveEngine {
	pe := &ProactiveEngine{
		db:       db,
		dataDir:  dataDir,
		taskMgr:  taskMgr,
		eventBus: eventBus,
		cfg: ProactiveConfig{
			Enabled:              true,
			ScanIntervalMinutes:  15,
			AutoCreateTasks:      false,
			DiskThresholdPercent: 80.0,
			GlobalKillSwitch:     false,
		},
	}
	pe.ensureSchema()
	_ = pe.loadConfig(context.Background())
	return pe
}

func (pe *ProactiveEngine) ensureSchema() {
	if pe == nil || pe.db == nil {
		return
	}
	schema := `
	CREATE TABLE IF NOT EXISTS system_anomalies (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		severity TEXT NOT NULL DEFAULT 'info',
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		suggested_action TEXT NOT NULL DEFAULT '',
		auto_task_payload TEXT NOT NULL DEFAULT '',
		detected_at TIMESTAMP NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		resolved_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_anomalies_status ON system_anomalies(status, detected_at);
	CREATE INDEX IF NOT EXISTS idx_anomalies_kind ON system_anomalies(kind);

	CREATE TABLE IF NOT EXISTS proactive_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, _ = pe.db.Exec(schema)
}

func (pe *ProactiveEngine) loadConfig(ctx context.Context) error {
	if pe == nil || pe.db == nil {
		return nil
	}
	var raw string
	err := pe.db.QueryRowContext(ctx, "SELECT value FROM proactive_settings WHERE key = 'config'").Scan(&raw)
	if err != nil {
		return err
	}
	var c ProactiveConfig
	if err := json.Unmarshal([]byte(raw), &c); err == nil {
		pe.mu.Lock()
		pe.cfg = c
		pe.mu.Unlock()
	}
	return nil
}

// GetConfig returns the active proactive engine configuration.
func (pe *ProactiveEngine) GetConfig(ctx context.Context) (ProactiveConfig, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.cfg, nil
}

// SaveConfig updates and persists proactive engine configuration.
func (pe *ProactiveEngine) SaveConfig(ctx context.Context, cfg ProactiveConfig) error {
	pe.mu.Lock()
	pe.cfg = cfg
	pe.mu.Unlock()

	if pe.db != nil {
		raw, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling proactive config: %w", err)
		}
		query := `
		INSERT INTO proactive_settings (key, value) VALUES ('config', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`
		_, err = pe.db.ExecContext(ctx, query, string(raw))
		if err != nil {
			return fmt.Errorf("saving proactive config: %w", err)
		}
	}
	return nil
}

// SetMCPChecker registers custom MCP health inspection function.
func (pe *ProactiveEngine) SetMCPChecker(fn func(ctx context.Context) ([]string, error)) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.mcpChecker = fn
}

// SetTokenBudgetChecker registers custom token quota inspection function.
func (pe *ProactiveEngine) SetTokenBudgetChecker(fn func(ctx context.Context) (used, cap int64, err error)) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.tokenBudgetChecker = fn
}

// SetInboundQueueChecker registers inbound queue length inspection function.
func (pe *ProactiveEngine) SetInboundQueueChecker(fn func(ctx context.Context) (int, error)) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.inboundQueueChecker = fn
}

// Scan executes all system health probes, records newly discovered anomalies, and optionally auto-creates tasks.
func (pe *ProactiveEngine) Scan(ctx context.Context) ([]SystemAnomaly, error) {
	pe.mu.RLock()
	cfg := pe.cfg
	pe.mu.RUnlock()

	if !cfg.Enabled || cfg.GlobalKillSwitch {
		return nil, nil
	}

	var discovered []SystemAnomaly

	// Probe 1: Disk Usage
	if an := pe.probeDiskUsage(ctx, cfg.DiskThresholdPercent); an != nil {
		discovered = append(discovered, *an)
	}

	// Probe 2: Stuck Embedding Queue
	if an := pe.probeEmbeddingQueue(ctx); an != nil {
		discovered = append(discovered, *an)
	}

	// Probe 3: Degraded MCP Servers
	if an := pe.probeMCPServers(ctx); an != nil {
		discovered = append(discovered, *an)
	}

	// Probe 4: Stalled Missions
	if an := pe.probeStalledMissions(ctx); an != nil {
		discovered = append(discovered, *an)
	}

	// Probe 5: Token Budget
	if an := pe.probeTokenBudget(ctx); an != nil {
		discovered = append(discovered, *an)
	}

	// Probe 6: Inbound Message Queue
	if an := pe.probeInboundQueue(ctx); an != nil {
		discovered = append(discovered, *an)
	}

	// Persist and publish events
	for i := range discovered {
		item := &discovered[i]
		pe.persistAnomaly(ctx, item)
		if cfg.AutoCreateTasks && item.AutoTaskPayload != nil && pe.taskMgr != nil {
			_, err := pe.taskMgr.CreateTask(ctx, *item.AutoTaskPayload)
			if err == nil {
				slog.Info("proactive engine auto-created mission for anomaly", "anomaly_id", item.ID, "kind", item.Kind)
			}
		}
		if pe.eventBus != nil {
			pe.eventBus.Publish(bus.NewEvent("anomaly:detected", "system", map[string]any{
				"anomaly": *item,
			}))
		}
	}

	return discovered, nil
}

func (pe *ProactiveEngine) probeDiskUsage(_ context.Context, thresholdPercent float64) *SystemAnomaly {
	if thresholdPercent <= 0 {
		thresholdPercent = 80.0
	}
	free, total, err := system.DiskUsage(pe.dataDir)
	if err != nil || total == 0 {
		if system.WritesFrozen(pe.dataDir) {
			return &SystemAnomaly{
				ID:              newAnomalyID("disk"),
				Kind:            "disk_usage",
				Severity:        "critical",
				Title:           "Disk Write Operations Frozen",
				Description:     "Data volume has reached critical low storage threshold. Heavy writes are frozen.",
				SuggestedAction: "Purge old temporary workspaces and prune logs immediately.",
				AutoTaskPayload: &AutonomousTask{
					Title:           "Emergency disk space cleanup",
					Description:     "Clean up temporary files, old caches, and vacuum databases in data partition.",
					Priority:        "p0_critical",
					AssignedAgentID: DefaultSystemAgentID,
				},
				DetectedAt: time.Now().UTC(),
				Status:     "active",
			}
		}
		return nil
	}

	usedRatio := 1.0 - (float64(free) / float64(total))
	usedPercent := usedRatio * 100.0

	if usedPercent >= thresholdPercent || free < 500*1024*1024 {
		severity := "warning"
		if usedPercent >= 95.0 || free < 200*1024*1024 {
			severity = "critical"
		}
		return &SystemAnomaly{
			ID:              newAnomalyID("disk"),
			Kind:            "disk_usage",
			Severity:        severity,
			Title:           fmt.Sprintf("High Disk Utilization (%.1f%%)", usedPercent),
			Description:     fmt.Sprintf("Data partition is using %.1f%% of available space (Free: %d MB / Total: %d MB).", usedPercent, free/(1024*1024), total/(1024*1024)),
			SuggestedAction: "Clean up old workspace backups and temporary cache files.",
			AutoTaskPayload: &AutonomousTask{
				Title:           "Clean up old workspace backups and temporary cache",
				Description:     "Prune files in workspace tmp and old backup archives to reclaim disk space.",
				Priority:        "p1_high",
				AssignedAgentID: DefaultSystemAgentID,
			},
			DetectedAt: time.Now().UTC(),
			Status:     "active",
		}
	}
	return nil
}

func (pe *ProactiveEngine) probeEmbeddingQueue(ctx context.Context) *SystemAnomaly {
	if pe.db == nil {
		return nil
	}
	// Check if embedding_jobs table exists and has stuck jobs
	var count int
	err := pe.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM embedding_jobs
		WHERE status = 'pending' AND due_at < ?
	`, time.Now().UTC().Add(-30*time.Minute)).Scan(&count)
	if err == nil && count > 0 {
		return &SystemAnomaly{
			ID:              newAnomalyID("embed"),
			Kind:            "embedding_queue",
			Severity:        "warning",
			Title:           fmt.Sprintf("Embedding Queue Stalled (%d overdue jobs)", count),
			Description:     fmt.Sprintf("There are %d embedding jobs in queue that have not progressed for over 30 minutes.", count),
			SuggestedAction: "Drain and reconcile the embedding worker queue.",
			AutoTaskPayload: &AutonomousTask{
				Title:           "Drain and recover stalled embedding jobs",
				Description:     "Re-queue overdue embedding indexing tasks and ensure vector search consistency.",
				Priority:        "p2_normal",
				AssignedAgentID: DefaultSystemAgentID,
			},
			DetectedAt: time.Now().UTC(),
			Status:     "active",
		}
	}
	return nil
}

func (pe *ProactiveEngine) probeMCPServers(ctx context.Context) *SystemAnomaly {
	pe.mu.RLock()
	checker := pe.mcpChecker
	pe.mu.RUnlock()

	if checker == nil {
		return nil
	}
	failedServers, err := checker(ctx)
	if err == nil && len(failedServers) > 0 {
		return &SystemAnomaly{
			ID:              newAnomalyID("mcp"),
			Kind:            "mcp_error",
			Severity:        "warning",
			Title:           fmt.Sprintf("MCP Servers Connection Degraded (%d failing)", len(failedServers)),
			Description:     fmt.Sprintf("Disconnected or failing MCP servers detected: %s", strings.Join(failedServers, ", ")),
			SuggestedAction: "Inspect server logs and reconnect MCP transport.",
			AutoTaskPayload: &AutonomousTask{
				Title:           "Diagnose and reconnect degraded MCP servers",
				Description:     fmt.Sprintf("Check health and reconnect failing MCP servers: %s", strings.Join(failedServers, ", ")),
				Priority:        "p2_normal",
				AssignedAgentID: DefaultSystemAgentID,
			},
			DetectedAt: time.Now().UTC(),
			Status:     "active",
		}
	}
	return nil
}

func (pe *ProactiveEngine) probeStalledMissions(ctx context.Context) *SystemAnomaly {
	if pe.db == nil {
		return nil
	}
	var count int
	err := pe.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM autonomous_tasks
		WHERE status = 'in_progress' AND stalled_cycles >= 3
	`).Scan(&count)
	if err == nil && count > 0 {
		return &SystemAnomaly{
			ID:              newAnomalyID("task"),
			Kind:            "task_stalled",
			Severity:        "info",
			Title:           fmt.Sprintf("Autonomous Tasks Stalled (%d missions without progress)", count),
			Description:     fmt.Sprintf("%d missions are marked in_progress but have shown 0 progress across multiple heartbeat cycles.", count),
			SuggestedAction: "Inspect step plans and resolve blocking dependencies.",
			AutoTaskPayload: &AutonomousTask{
				Title:           "Review and unblock stalled mission steps",
				Description:     "Examine dependencies and restart paused steps in stalled autonomous tasks.",
				Priority:        "p2_normal",
				AssignedAgentID: DefaultSystemAgentID,
			},
			DetectedAt: time.Now().UTC(),
			Status:     "active",
		}
	}
	return nil
}

func (pe *ProactiveEngine) probeTokenBudget(ctx context.Context) *SystemAnomaly {
	pe.mu.RLock()
	checker := pe.tokenBudgetChecker
	pe.mu.RUnlock()

	if checker == nil {
		return nil
	}
	used, capTokens, err := checker(ctx)
	if err == nil && capTokens > 0 {
		ratio := float64(used) / float64(capTokens)
		if ratio >= 0.80 {
			severity := "warning"
			if ratio >= 0.95 {
				severity = "critical"
			}
			return &SystemAnomaly{
				ID:              newAnomalyID("token"),
				Kind:            "token_budget",
				Severity:        severity,
				Title:           fmt.Sprintf("Token Budget High Usage (%.1f%% of quota)", ratio*100.0),
				Description:     fmt.Sprintf("Current monthly token consumption is %d / %d tokens (%.1f%%).", used, capTokens, ratio*100.0),
				SuggestedAction: "Tune cascade router to prioritize cost-efficient fallback models.",
				AutoTaskPayload: &AutonomousTask{
					Title:           "Optimize agent model cascades for cost efficiency",
					Description:     "Review agent model configurations and switch non-critical reasoning to lightweight models.",
					Priority:        "p2_normal",
					AssignedAgentID: DefaultSystemAgentID,
				},
				DetectedAt: time.Now().UTC(),
				Status:     "active",
			}
		}
	}
	return nil
}

func (pe *ProactiveEngine) probeInboundQueue(ctx context.Context) *SystemAnomaly {
	pe.mu.RLock()
	checker := pe.inboundQueueChecker
	pe.mu.RUnlock()

	if checker == nil {
		return nil
	}
	queueLen, err := checker(ctx)
	if err == nil && queueLen >= 10 {
		return &SystemAnomaly{
			ID:              newAnomalyID("inbound"),
			Kind:            "inbound_queue",
			Severity:        "info",
			Title:           fmt.Sprintf("Inbound Channel Message Backlog (%d queued)", queueLen),
			Description:     fmt.Sprintf("High volume of incoming messages waiting in queue (%d messages).", queueLen),
			SuggestedAction: "Triage and process incoming queue backlog.",
			AutoTaskPayload: &AutonomousTask{
				Title:           "Triage and process inbound channel messages",
				Description:     "Process pending queue of unhandled messages across connected channels.",
				Priority:        "p2_normal",
				AssignedAgentID: DefaultSystemAgentID,
			},
			DetectedAt: time.Now().UTC(),
			Status:     "active",
		}
	}
	return nil
}

func (pe *ProactiveEngine) persistAnomaly(ctx context.Context, an *SystemAnomaly) {
	if pe.db == nil || an == nil {
		return
	}
	payloadJSON := ""
	if an.AutoTaskPayload != nil {
		if b, err := json.Marshal(an.AutoTaskPayload); err == nil {
			payloadJSON = string(b)
		}
	}

	// Avoid duplicate active anomalies of the same kind
	var existingID string
	err := pe.db.QueryRowContext(ctx, `
		SELECT id FROM system_anomalies
		WHERE kind = ? AND status = 'active'
		ORDER BY detected_at DESC LIMIT 1
	`, an.Kind).Scan(&existingID)
	if err == nil && existingID != "" {
		an.ID = existingID
		_, _ = pe.db.ExecContext(ctx, `
			UPDATE system_anomalies
			SET severity = ?, title = ?, description = ?, suggested_action = ?, auto_task_payload = ?, detected_at = ?
			WHERE id = ?
		`, an.Severity, an.Title, an.Description, an.SuggestedAction, payloadJSON, an.DetectedAt, an.ID)
		return
	}

	query := `
	INSERT INTO system_anomalies (
		id, kind, severity, title, description, suggested_action, auto_task_payload, detected_at, status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, _ = pe.db.ExecContext(ctx, query, an.ID, an.Kind, an.Severity, an.Title, an.Description, an.SuggestedAction, payloadJSON, an.DetectedAt, an.Status)
}

// ListAnomalies queries system anomalies with optional status and severity filtering.
func (pe *ProactiveEngine) ListAnomalies(ctx context.Context, status, severity string, limit int) ([]SystemAnomaly, error) {
	if pe == nil || pe.db == nil {
		return []SystemAnomaly{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := `
	SELECT id, kind, severity, title, description, suggested_action, auto_task_payload, detected_at, status, resolved_at
	FROM system_anomalies
	`
	var conditions []string
	var args []any

	if status != "" && status != "all" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if severity != "" && severity != "all" {
		conditions = append(conditions, "severity = ?")
		args = append(args, severity)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY detected_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := pe.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing anomalies: %w", err)
	}
	defer rows.Close()

	var items []SystemAnomaly
	for rows.Next() {
		var item SystemAnomaly
		var rawPayload string
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Kind, &item.Severity, &item.Title, &item.Description,
			&item.SuggestedAction, &rawPayload, &item.DetectedAt, &item.Status, &resolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning anomaly: %w", err)
		}
		if rawPayload != "" {
			var task AutonomousTask
			if err := json.Unmarshal([]byte(rawPayload), &task); err == nil {
				item.AutoTaskPayload = &task
			}
		}
		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.Time
		}
		items = append(items, item)
	}
	if items == nil {
		items = []SystemAnomaly{}
	}
	return items, rows.Err()
}

// ActOnAnomaly handles operator or automated actions on a detected anomaly ("auto_task", "resolve", "ignore").
func (pe *ProactiveEngine) ActOnAnomaly(ctx context.Context, id, action string) (*AutonomousTask, error) {
	if pe == nil || pe.db == nil {
		return nil, errors.New("proactive engine database unavailable")
	}

	var item SystemAnomaly
	var rawPayload string
	err := pe.db.QueryRowContext(ctx, `
		SELECT id, kind, severity, title, description, suggested_action, auto_task_payload, detected_at, status
		FROM system_anomalies WHERE id = ?
	`, id).Scan(&item.ID, &item.Kind, &item.Severity, &item.Title, &item.Description, &item.SuggestedAction, &rawPayload, &item.DetectedAt, &item.Status)
	if err != nil {
		return nil, fmt.Errorf("anomaly not found: %w", err)
	}

	now := time.Now().UTC()
	switch strings.ToLower(action) {
	case "auto_task":
		var created *AutonomousTask
		if rawPayload != "" && pe.taskMgr != nil {
			var task AutonomousTask
			if err := json.Unmarshal([]byte(rawPayload), &task); err == nil {
				created, err = pe.taskMgr.CreateTask(ctx, task)
				if err != nil {
					return nil, fmt.Errorf("creating mission for anomaly: %w", err)
				}
			}
		}
		_, _ = pe.db.ExecContext(ctx, "UPDATE system_anomalies SET status = 'resolved', resolved_at = ? WHERE id = ?", now, id)
		return created, nil

	case "resolve":
		_, err := pe.db.ExecContext(ctx, "UPDATE system_anomalies SET status = 'resolved', resolved_at = ? WHERE id = ?", now, id)
		if err != nil {
			return nil, fmt.Errorf("resolving anomaly: %w", err)
		}
		return nil, nil

	case "ignore":
		_, err := pe.db.ExecContext(ctx, "UPDATE system_anomalies SET status = 'ignored', resolved_at = ? WHERE id = ?", now, id)
		if err != nil {
			return nil, fmt.Errorf("ignoring anomaly: %w", err)
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("invalid anomaly action: %q (must be auto_task, resolve, or ignore)", action)
	}
}

func newAnomalyID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("anom_%s_%d_%x", prefix, time.Now().Unix(), b)
}

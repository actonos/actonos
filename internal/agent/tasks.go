package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// AutonomousTask represents an operational mission or background goal managed by ActonOS.
type AutonomousTask struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`            // "pending", "in_progress", "completed", "blocked", "cancelled"
	Priority        string     `json:"priority"`          // "p0_critical", "p1_high", "p2_normal", "p3_low"
	AssignedAgentID string     `json:"assigned_agent_id"` // "auto", "agent_system_core", or specific agent ID
	TargetChannel   string     `json:"target_channel,omitempty"`
	TargetAccountID string     `json:"target_account_id,omitempty"`
	Progress        int        `json:"progress"` // 0 to 100%
	ExecutionLog    string     `json:"execution_log,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
	CreatedBy       string     `json:"created_by"` // "user", "heartbeat", "agent"
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// HeartbeatConfig defines standing rules and pulse parameters for the autonomous daemon.
type HeartbeatConfig struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	Directives      string `json:"directives"`
	TargetChannel   string `json:"target_channel"`
	TargetAccountID string `json:"target_account_id"`
	AutoDelegate    bool   `json:"auto_delegate"`
	ZeroNoise       bool   `json:"zero_noise"`

	// AckMaxChars bounds how much extra commentary may accompany HEARTBEAT_OK
	// before a reply is treated as a real alert rather than a silent
	// acknowledgement. Mirrors OpenClaw's heartbeat.ackMaxChars (default 300).
	AckMaxChars int `json:"ack_max_chars,omitempty"`

	// ActiveHoursStart/End restrict routine (non-manual) heartbeat cycles to a
	// daily HH:MM window; outside the window, cycles are skipped until the
	// next tick inside it. Leave both empty to run 24/7 (default). Mirrors
	// OpenClaw's heartbeat.activeHours.
	ActiveHoursStart    string `json:"active_hours_start,omitempty"`
	ActiveHoursEnd      string `json:"active_hours_end,omitempty"`
	ActiveHoursTimezone string `json:"active_hours_timezone,omitempty"`
}

// legacyDefaultHeartbeatDirective was persisted by earlier releases despite
// having no concrete check to perform. It must not activate an idle heartbeat.
const legacyDefaultHeartbeatDirective = "Autonomous standing supervisor. Routinely review pending tasks in TASKS.md and monitor system stability."

// TaskManager coordinates autonomous tasks and heartbeat configuration in
// SQLite. User workspace filenames are never reserved for system state.
type TaskManager struct {
	mu sync.RWMutex
	db *sql.DB
}

// NewTaskManager creates and initializes a TaskManager.
func NewTaskManager(db *sql.DB, _ string) (*TaskManager, error) {
	tm := &TaskManager{db: db}

	if err := tm.initDB(); err != nil {
		return nil, err
	}

	return tm, nil
}

func (tm *TaskManager) initDB() error {
	if tm.db == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS autonomous_tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT DEFAULT 'pending',
		priority TEXT DEFAULT 'p2_normal',
		assigned_agent_id TEXT DEFAULT 'auto',
		target_channel TEXT DEFAULT 'all',
		target_account_id TEXT DEFAULT 'all',
		progress INTEGER DEFAULT 0,
		execution_log TEXT DEFAULT '',
		session_id TEXT DEFAULT '',
		created_by TEXT DEFAULT 'user',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON autonomous_tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON autonomous_tasks(priority);

	CREATE TABLE IF NOT EXISTS heartbeat_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	if _, err := tm.db.Exec(schema); err != nil {
		return err
	}

	// Auto-heal/unblock system health tasks with refined instructions
	_, _ = tm.db.Exec(`
		UPDATE autonomous_tasks
		SET status = 'pending',
		    description = 'Run native_sysinfo to inspect memory allocation, CPU load, SQLite database & WAL status, and vector memory index health. If nominal, report healthy metrics and complete the mission.'
		WHERE status = 'blocked' AND title LIKE '%System Health%'
	`)

	return nil
}

// CreateTask inserts a new task and updates TASKS.md.
func (tm *TaskManager) CreateTask(ctx context.Context, t AutonomousTask) (*AutonomousTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if t.ID == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		t.ID = fmt.Sprintf("task_%d_%x", time.Now().Unix(), b)
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.Priority == "" {
		t.Priority = "p2_normal"
	}
	if t.AssignedAgentID == "" {
		t.AssignedAgentID = "auto"
	}
	if t.TargetChannel == "" {
		t.TargetChannel = "all"
	}
	if t.TargetAccountID == "" {
		t.TargetAccountID = "all"
	}
	if t.SessionID == "" {
		t.SessionID = fmt.Sprintf("conv_task_%s", t.ID)
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now

	if tm.db != nil {
		query := `
		INSERT INTO autonomous_tasks (
			id, title, description, status, priority, assigned_agent_id,
			target_channel, target_account_id, progress, execution_log,
			session_id, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		_, err := tm.db.ExecContext(ctx, query,
			t.ID, t.Title, t.Description, t.Status, t.Priority, t.AssignedAgentID,
			t.TargetChannel, t.TargetAccountID, t.Progress, t.ExecutionLog,
			t.SessionID, t.CreatedBy, t.CreatedAt, t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
	}

	_ = tm.syncToMarkdownLocked()
	return &t, nil
}

// GetTask retrieves a single task by ID.
func (tm *TaskManager) GetTask(ctx context.Context, id string) (*AutonomousTask, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.db == nil {
		return nil, sql.ErrNoRows
	}

	query := `
	SELECT id, title, description, status, priority, assigned_agent_id,
	       target_channel, target_account_id, progress, execution_log,
	       session_id, created_by, created_at, updated_at, completed_at
	FROM autonomous_tasks
	WHERE id = ?
	`
	row := tm.db.QueryRowContext(ctx, query, id)

	var t AutonomousTask
	var compAt sql.NullTime
	err := row.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.AssignedAgentID,
		&t.TargetChannel, &t.TargetAccountID, &t.Progress, &t.ExecutionLog,
		&t.SessionID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &compAt,
	)
	if err != nil {
		return nil, err
	}
	if compAt.Valid {
		t.CompletedAt = &compAt.Time
	}
	return &t, nil
}

// UpdateTask updates task fields, status, or progress and syncs to TASKS.md.
func (tm *TaskManager) UpdateTask(ctx context.Context, t AutonomousTask) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.db == nil {
		return nil
	}

	now := time.Now().UTC()
	t.UpdatedAt = now
	if t.Status == "completed" && t.CompletedAt == nil {
		t.CompletedAt = &now
		t.Progress = 100
	}

	query := `
	UPDATE autonomous_tasks SET
		title = ?, description = ?, status = ?, priority = ?, assigned_agent_id = ?,
		target_channel = ?, target_account_id = ?, progress = ?, execution_log = ?,
		updated_at = ?, completed_at = ?
	WHERE id = ?
	`
	_, err := tm.db.ExecContext(ctx, query,
		t.Title, t.Description, t.Status, t.Priority, t.AssignedAgentID,
		t.TargetChannel, t.TargetAccountID, t.Progress, t.ExecutionLog,
		t.UpdatedAt, t.CompletedAt, t.ID,
	)
	if err != nil {
		return err
	}

	_ = tm.syncToMarkdownLocked()
	return nil
}

// DeleteTask removes a task by ID, purges its conversation session history, and syncs to TASKS.md.
func (tm *TaskManager) DeleteTask(ctx context.Context, id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.db != nil {
		// 1. Look up the task's session ID before deletion so we can clean up conversations.
		var sessionID string
		_ = tm.db.QueryRowContext(ctx, "SELECT session_id FROM autonomous_tasks WHERE id = ?", id).Scan(&sessionID)

		// 2. Delete the task record.
		_, err := tm.db.ExecContext(ctx, "DELETE FROM autonomous_tasks WHERE id = ?", id)
		if err != nil {
			return err
		}

		// 3. Purge conversation session history to prevent ghost task contamination.
		// The session ID is deterministic: "conv_task_{taskID}" (see CreateTask).
		convIDs := []string{fmt.Sprintf("conv_task_%s", id)}
		if sessionID != "" && sessionID != convIDs[0] {
			convIDs = append(convIDs, sessionID)
		}
		for _, cid := range convIDs {
			_, _ = tm.db.ExecContext(ctx, "DELETE FROM messages WHERE conversation_id = ?", cid)
			_, _ = tm.db.ExecContext(ctx, "DELETE FROM conversations WHERE id = ?", cid)
		}

		slog.Info("task deleted with session cleanup", "task_id", id, "purged_sessions", convIDs)
	}

	_ = tm.syncToMarkdownLocked()
	return nil
}

// ListTasks returns all tasks matching optional status and priority filters.
func (tm *TaskManager) ListTasks(ctx context.Context, status, priority string) ([]AutonomousTask, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.db == nil {
		return []AutonomousTask{}, nil
	}

	query := `
	SELECT id, title, description, status, priority, assigned_agent_id,
	       target_channel, target_account_id, progress, execution_log,
	       session_id, created_by, created_at, updated_at, completed_at
	FROM autonomous_tasks
	WHERE 1=1
	`
	var args []any
	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if priority != "" && priority != "all" {
		query += " AND priority = ?"
		args = append(args, priority)
	}

	// Order by priority (p0 > p1 > p2 > p3) and updated_at DESC
	query += `
	ORDER BY 
		CASE priority
			WHEN 'p0_critical' THEN 1
			WHEN 'p1_high' THEN 2
			WHEN 'p2_normal' THEN 3
			WHEN 'p3_low' THEN 4
			ELSE 5
		END ASC,
		updated_at DESC
	`

	rows, err := tm.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AutonomousTask
	for rows.Next() {
		var t AutonomousTask
		var compAt sql.NullTime
		if err := rows.Scan(
			&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.AssignedAgentID,
			&t.TargetChannel, &t.TargetAccountID, &t.Progress, &t.ExecutionLog,
			&t.SessionID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &compAt,
		); err == nil {
			if compAt.Valid {
				t.CompletedAt = &compAt.Time
			}
			list = append(list, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []AutonomousTask{}
	}
	return list, nil
}

// syncToMarkdownLocked remains as an internal compatibility hook for callers
// created before the database workspace migration. SQLite is now the sole
// source of truth, so synchronization intentionally performs no file I/O.
func (tm *TaskManager) syncToMarkdownLocked() error {
	return nil
}

// GetHeartbeatConfig loads heartbeat configuration and directives.
func (tm *TaskManager) GetHeartbeatConfig(ctx context.Context) (*HeartbeatConfig, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	cfg := &HeartbeatConfig{
		Enabled:         true,
		IntervalMinutes: 5,
		Directives:      "",
		TargetChannel:   "all",
		TargetAccountID: "all",
		AutoDelegate:    true,
		ZeroNoise:       true,
		AckMaxChars:     300,
	}

	if tm.db != nil {
		var raw string
		if err := tm.db.QueryRowContext(ctx, "SELECT value FROM heartbeat_settings WHERE key = 'config'").Scan(&raw); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), cfg)
		}
	}

	if strings.TrimSpace(cfg.Directives) == legacyDefaultHeartbeatDirective {
		cfg.Directives = ""
	}

	return cfg, nil
}

// SaveHeartbeatConfig persists heartbeat configuration exclusively in SQLite.
func (tm *TaskManager) SaveHeartbeatConfig(ctx context.Context, cfg HeartbeatConfig) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if strings.TrimSpace(cfg.Directives) == legacyDefaultHeartbeatDirective {
		cfg.Directives = ""
	}

	if tm.db != nil {
		raw, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling heartbeat configuration: %w", err)
		}
		query := `
			INSERT INTO heartbeat_settings (key, value) VALUES ('config', ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
			`
		if _, err := tm.db.ExecContext(ctx, query, string(raw)); err != nil {
			return fmt.Errorf("saving heartbeat configuration: %w", err)
		}
	}

	slog.Info("heartbeat directives and configuration saved", "interval_mins", cfg.IntervalMinutes, "enabled", cfg.Enabled)
	return nil
}

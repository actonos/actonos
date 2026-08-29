package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	Plan            *TaskPlan  `json:"plan,omitempty"`
	StalledCycles   int        `json:"stalled_cycles,omitempty"`
	FailCount       int        `json:"fail_count,omitempty"`
}

// StructuredDirective defines an autonomous standing directive with structured fields and verification.
type StructuredDirective struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	Priority          string `json:"priority"` // "p0_critical", "p1_high", "p2_normal", "p3_low"
	Schedule          string `json:"schedule,omitempty"` // Cron expression e.g. "0 9 * * *"
	ExpectedOutcome   string `json:"expected_outcome,omitempty"`
	Verification      string `json:"verification,omitempty"` // e.g. "file_exists:/data/workspace/reports/seo.md"
	AutoCreateMission bool   `json:"auto_create_mission"`
	MaxRuntimeMin     int    `json:"max_runtime_min,omitempty"`
	Enabled           bool   `json:"enabled"`
}

// HeartbeatConfig defines standing rules and pulse parameters for the autonomous daemon.
type HeartbeatConfig struct {
	Enabled              bool                  `json:"enabled"`
	IntervalMinutes      int                   `json:"interval_minutes"`
	Directives           string                `json:"directives"`
	StructuredDirectives []StructuredDirective `json:"structured_directives,omitempty"`
	TargetChannel        string                `json:"target_channel"`
	TargetAccountID      string                `json:"target_account_id"`

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
// SQLite and synchronizes them to agent markdown files (/data/agents/{AGENT_SLUG}/).
type TaskManager struct {
	mu        sync.RWMutex
	db        *sql.DB
	agentsDir string
}

// NewTaskManager creates and initializes a TaskManager.
func NewTaskManager(db *sql.DB, path string) (*TaskManager, error) {
	if path == "" {
		path = "./data"
	}
	dataDir := path
	if filepath.Base(path) == "workspace" {
		dataDir = filepath.Dir(path)
	}
	agentsDir := filepath.Join(dataDir, "agents")
	tm := &TaskManager{
		db:        db,
		agentsDir: agentsDir,
	}

	if err := tm.initDB(); err != nil {
		return nil, err
	}

	if tm.db != nil {
		_ = tm.syncToMarkdownLocked()
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
	_, _ = tm.db.Exec(`ALTER TABLE autonomous_tasks ADD COLUMN plan_json TEXT NOT NULL DEFAULT ''`)
	_, _ = tm.db.Exec(`ALTER TABLE autonomous_tasks ADD COLUMN stalled_cycles INTEGER NOT NULL DEFAULT 0`)
	_, _ = tm.db.Exec(`ALTER TABLE autonomous_tasks ADD COLUMN fail_count INTEGER NOT NULL DEFAULT 0`)
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
		planJSON, _ := json.Marshal(t.Plan)
		if t.Plan == nil {
			planJSON = []byte("")
		}
		query := `
		INSERT INTO autonomous_tasks (
			id, title, description, status, priority, assigned_agent_id,
			target_channel, target_account_id, progress, execution_log,
			session_id, created_by, created_at, updated_at, plan_json, stalled_cycles, fail_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		_, err := tm.db.ExecContext(ctx, query,
			t.ID, t.Title, t.Description, t.Status, t.Priority, t.AssignedAgentID,
			t.TargetChannel, t.TargetAccountID, t.Progress, t.ExecutionLog,
			t.SessionID, t.CreatedBy, t.CreatedAt, t.UpdatedAt, string(planJSON), t.StalledCycles, t.FailCount,
		)
		if err != nil {
			return nil, err
		}
	}

	_ = tm.syncToMarkdownLocked()
	return &t, nil
}

// EnqueueMission records agent-originated unattended work for the 24/7 loop.
func (tm *TaskManager) EnqueueMission(ctx context.Context, title, description, assignedAgentID string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("mission title is required")
	}
	created, err := tm.CreateTask(ctx, AutonomousTask{
		Title:           title,
		Description:     strings.TrimSpace(description),
		AssignedAgentID: strings.TrimSpace(assignedAgentID),
		CreatedBy:       "agent",
		Status:          "pending",
		Priority:        "p2_normal",
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
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
	       session_id, created_by, created_at, updated_at, completed_at,
	       COALESCE(plan_json, ''), COALESCE(stalled_cycles, 0), COALESCE(fail_count, 0)
	FROM autonomous_tasks
	WHERE id = ?
	`
	row := tm.db.QueryRowContext(ctx, query, id)

	var t AutonomousTask
	var compAt sql.NullTime
	var planJSON string
	err := row.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.AssignedAgentID,
		&t.TargetChannel, &t.TargetAccountID, &t.Progress, &t.ExecutionLog,
		&t.SessionID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &compAt,
		&planJSON, &t.StalledCycles, &t.FailCount,
	)
	if err != nil {
		return nil, err
	}
	if compAt.Valid {
		t.CompletedAt = &compAt.Time
	}
	if strings.TrimSpace(planJSON) != "" {
		var plan TaskPlan
		if json.Unmarshal([]byte(planJSON), &plan) == nil {
			t.Plan = &plan
		}
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

	var oldStatus string
	var oldProgress int
	var oldPlanJSON string
	_ = tm.db.QueryRowContext(ctx, `SELECT status, progress, COALESCE(plan_json, '') FROM autonomous_tasks WHERE id = ?`, t.ID).Scan(&oldStatus, &oldProgress, &oldPlanJSON)

	isOperatorReset := (oldProgress > 0 && t.Progress == 0) ||
		((oldStatus == "blocked" || oldStatus == "failed" || oldStatus == "cancelled") && (t.Status == "pending" || t.Status == "in_progress"))

	// When an operator resets a task (progress rewound to 0 or status set back to pending/in_progress from blocked/failed):
	if isOperatorReset {
		t.FailCount = 0
		t.StalledCycles = 0
	}

	planJSON := ""
	if t.Plan != nil {
		if oldProgress > 0 && t.Progress == 0 {
			t.Plan.ReopenAllSteps()
		} else if isOperatorReset {
			t.Plan.ReopenFailedSteps()
		}
		if encoded, marshalErr := json.Marshal(t.Plan); marshalErr == nil {
			planJSON = string(encoded)
		}
	} else {
		planJSON = oldPlanJSON
		if strings.TrimSpace(planJSON) != "" && isOperatorReset {
			var loadedPlan TaskPlan
			if json.Unmarshal([]byte(planJSON), &loadedPlan) == nil && len(loadedPlan.Steps) > 0 {
				if oldProgress > 0 && t.Progress == 0 {
					loadedPlan.ReopenAllSteps()
					if encoded, marshalErr := json.Marshal(&loadedPlan); marshalErr == nil {
						planJSON = string(encoded)
					}
				} else if isOperatorReset {
					if loadedPlan.ReopenFailedSteps() {
						if encoded, marshalErr := json.Marshal(&loadedPlan); marshalErr == nil {
							planJSON = string(encoded)
						}
					}
				}
			}
		}
	}
	query := `
	UPDATE autonomous_tasks SET
		title = ?, description = ?, status = ?, priority = ?, assigned_agent_id = ?,
		target_channel = ?, target_account_id = ?, progress = ?, execution_log = ?,
		updated_at = ?, completed_at = ?, plan_json = ?, stalled_cycles = ?, fail_count = ?
	WHERE id = ?
	`
	_, err := tm.db.ExecContext(ctx, query,
		t.Title, t.Description, t.Status, t.Priority, t.AssignedAgentID,
		t.TargetChannel, t.TargetAccountID, t.Progress, t.ExecutionLog,
		t.UpdatedAt, t.CompletedAt, planJSON, t.StalledCycles, t.FailCount, t.ID,
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
	       session_id, created_by, created_at, updated_at, completed_at,
	       COALESCE(plan_json, ''), COALESCE(stalled_cycles, 0), COALESCE(fail_count, 0)
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
		var planJSON string
		if err := rows.Scan(
			&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.AssignedAgentID,
			&t.TargetChannel, &t.TargetAccountID, &t.Progress, &t.ExecutionLog,
			&t.SessionID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &compAt,
			&planJSON, &t.StalledCycles, &t.FailCount,
		); err == nil {
			if compAt.Valid {
				t.CompletedAt = &compAt.Time
			}
			if strings.TrimSpace(planJSON) != "" {
				var plan TaskPlan
				if json.Unmarshal([]byte(planJSON), &plan) == nil {
					t.Plan = &plan
				}
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

// syncToMarkdownLocked renders the autonomous task backlog into /data/agents/{AGENT_SLUG}/TASKS.md.
func (tm *TaskManager) syncToMarkdownLocked() error {
	if tm.db == nil || tm.agentsDir == "" {
		return nil
	}

	rows, err := tm.db.Query(`
		SELECT id, title, description, status, priority, assigned_agent_id, progress, updated_at
		FROM autonomous_tasks
		ORDER BY 
			CASE priority
				WHEN 'p0_critical' THEN 1
				WHEN 'p1_high' THEN 2
				WHEN 'p2_normal' THEN 3
				WHEN 'p3_low' THEN 4
				ELSE 5
			END ASC,
			updated_at DESC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var activeTasks []string
	var completedTasks []string

	for rows.Next() {
		var id, title, desc, status, priority, agentID string
		var progress int
		var updatedAt time.Time
		if err := rows.Scan(&id, &title, &desc, &status, &priority, &agentID, &progress, &updatedAt); err != nil {
			continue
		}
		if status == "completed" {
			completedTasks = append(completedTasks, fmt.Sprintf("- [x] **[%s]** %s *(Status: completed)*", priority, title))
		} else {
			activeTasks = append(activeTasks, fmt.Sprintf("- [ ] **[%s]** %s *(Status: %s, Progress: %d%%)*\n  - ID: `%s` | Assigned: `%s`\n  - %s",
				priority, title, status, progress, id, agentID, desc))
		}
	}

	var sb strings.Builder
	sb.WriteString("# ActonOS Autonomous Tasks Backlog\n")
	fmt.Fprintf(&sb, "> Last synchronized: %s UTC\n\n", time.Now().UTC().Format(time.RFC3339))
	sb.WriteString("## Active Backlog\n\n")
	if len(activeTasks) == 0 {
		sb.WriteString("*No active tasks currently pending in backlog.*\n\n")
	} else {
		for _, item := range activeTasks {
			sb.WriteString(item + "\n\n")
		}
	}
	sb.WriteString("## Completed & Archived Missions\n\n")
	if len(completedTasks) == 0 {
		sb.WriteString("*No completed missions recorded.*\n\n")
	} else {
		for _, item := range completedTasks {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\n")
	}

	systemAgentDir := filepath.Join(tm.agentsDir, DefaultSystemAgentID)
	_ = os.MkdirAll(systemAgentDir, 0750)
	targetTasks := filepath.Join(systemAgentDir, "TASKS.md")
	return os.WriteFile(targetTasks, []byte(sb.String()), 0640)
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
		AckMaxChars:     300,
	}

	if tm.db != nil {
		var raw string
		if err := tm.db.QueryRowContext(ctx, "SELECT value FROM heartbeat_settings WHERE key = 'config'").Scan(&raw); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), cfg)
		}
	}

	if cfg.Directives == "" && tm.agentsDir != "" {
		targetHeartbeat := filepath.Join(tm.agentsDir, DefaultSystemAgentID, "HEARTBEAT.md")
		if data, err := os.ReadFile(targetHeartbeat); err == nil && len(data) > 0 {
			cfg.Directives = string(data)
		}
	}

	if strings.TrimSpace(cfg.Directives) == legacyDefaultHeartbeatDirective {
		cfg.Directives = ""
	}

	return cfg, nil
}

// SaveHeartbeatConfig persists heartbeat configuration in SQLite and /data/agents/{AGENT_SLUG}/HEARTBEAT.md.
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

	if tm.agentsDir != "" {
		systemAgentDir := filepath.Join(tm.agentsDir, DefaultSystemAgentID)
		_ = os.MkdirAll(systemAgentDir, 0750)
		targetHeartbeat := filepath.Join(systemAgentDir, "HEARTBEAT.md")
		_ = os.WriteFile(targetHeartbeat, []byte(cfg.Directives), 0640)
	}

	slog.Info("heartbeat directives and configuration saved", "interval_mins", cfg.IntervalMinutes, "enabled", cfg.Enabled)
	return nil
}

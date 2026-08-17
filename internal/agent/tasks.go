package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
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
}

// TaskManager coordinates autonomous tasks, persistence in SQLite, and bi-directional markdown synchronization.
type TaskManager struct {
	mu           sync.RWMutex
	db           *sql.DB
	workspaceDir string
}

// NewTaskManager creates and initializes a TaskManager.
func NewTaskManager(db *sql.DB, workspaceDir string) (*TaskManager, error) {
	if workspaceDir == "" {
		workspaceDir = "./data/workspace"
	}
	_ = os.MkdirAll(workspaceDir, 0755)

	tm := &TaskManager{
		db:           db,
		workspaceDir: workspaceDir,
	}

	if err := tm.initDB(); err != nil {
		return nil, err
	}

	// Initial sync: populate defaults if DB is completely empty
	tasks, _ := tm.ListTasks(context.Background(), "", "")
	if len(tasks) == 0 {
		tm.seedDefaultTasks()
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

func (tm *TaskManager) seedDefaultTasks() {
	ctx := context.Background()
	defaultTasks := []AutonomousTask{
		{
			Title:           "Verify System Health & Storage Integrity",
			Description:     "Run native_sysinfo to inspect memory allocation, CPU load, SQLite database & WAL status, and vector memory index health. If nominal, report healthy metrics and complete the mission.",
			Priority:        "p1_high",
			AssignedAgentID: "auto",
			Status:          "pending",
			CreatedBy:       "system",
		},
		{
			Title:           "Monitor Connected Messaging Channels",
			Description:     "Ensure Telegram and Discord channel bot connections remain responsive with zero session drops.",
			Priority:        "p2_normal",
			AssignedAgentID: "agent_system_core",
			Status:          "pending",
			CreatedBy:       "system",
		},
	}

	for _, dt := range defaultTasks {
		_, _ = tm.CreateTask(ctx, dt)
	}
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

// DeleteTask removes a task by ID and syncs to TASKS.md.
func (tm *TaskManager) DeleteTask(ctx context.Context, id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.db != nil {
		_, err := tm.db.ExecContext(ctx, "DELETE FROM autonomous_tasks WHERE id = ?", id)
		if err != nil {
			return err
		}
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
	if list == nil {
		list = []AutonomousTask{}
	}
	return list, nil
}

// syncToMarkdownLocked writes current tasks to data/workspace/TASKS.md.
func (tm *TaskManager) syncToMarkdownLocked() error {
	if tm.db == nil {
		return nil
	}

	query := `
	SELECT id, title, description, status, priority, assigned_agent_id, progress, execution_log
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
	`
	rows, err := tm.db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("# ActonOS Autonomous Tasks Backlog\n")
	sb.WriteString(fmt.Sprintf("> Last synchronized: %s UTC\n\n", time.Now().UTC().Format(time.RFC3339)))

	sb.WriteString("## Active Backlog\n\n")

	hasActive := false
	var completedTasks []string

	for rows.Next() {
		var id, title, desc, status, priority, agentID, execLog string
		var progress int
		if err := rows.Scan(&id, &title, &desc, &status, &priority, &agentID, &progress, &execLog); err == nil {
			if status == "completed" || status == "cancelled" {
				completedTasks = append(completedTasks, fmt.Sprintf("- [x] **[%s]** %s *(Status: %s)*\n", priority, title, status))
			} else {
				hasActive = true
				statusEmoji := "⏳"
				if status == "in_progress" {
					statusEmoji = "⚡"
				} else if status == "blocked" {
					statusEmoji = "🚫"
				}
				sb.WriteString(fmt.Sprintf("### %s [%s] %s\n", statusEmoji, strings.ToUpper(priority), title))
				sb.WriteString(fmt.Sprintf("- **Task ID**: `%s`\n", id))
				sb.WriteString(fmt.Sprintf("- **Assigned Agent**: `%s`\n", agentID))
				sb.WriteString(fmt.Sprintf("- **Status**: `%s` (%d%% complete)\n", status, progress))
				if desc != "" {
					sb.WriteString(fmt.Sprintf("- **Directive**: %s\n", desc))
				}
				if execLog != "" {
					sb.WriteString(fmt.Sprintf("- **Latest Note**: *%s*\n", execLog))
				}
				sb.WriteString("\n")
			}
		}
	}

	if !hasActive {
		sb.WriteString("*No active tasks currently pending in backlog.*\n\n")
	}

	if len(completedTasks) > 0 {
		sb.WriteString("## Completed & Archived Missions\n\n")
		for _, ct := range completedTasks {
			sb.WriteString(ct)
		}
	}

	filePath := filepath.Join(tm.workspaceDir, "TASKS.md")
	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}

// GetHeartbeatConfig loads heartbeat configuration and directives.
func (tm *TaskManager) GetHeartbeatConfig(ctx context.Context) (*HeartbeatConfig, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	cfg := &HeartbeatConfig{
		Enabled:         true,
		IntervalMinutes: 5,
		Directives:      "Autonomous standing supervisor. Routinely review pending tasks in TASKS.md and monitor system stability.",
		TargetChannel:   "all",
		TargetAccountID: "all",
		AutoDelegate:    true,
		ZeroNoise:       true,
	}

	// Read from HEARTBEAT.md if present
	hbPath := filepath.Join(tm.workspaceDir, "HEARTBEAT.md")
	if data, err := os.ReadFile(hbPath); err == nil && len(data) > 0 {
		cfg.Directives = string(data)
	}

	return cfg, nil
}

// SaveHeartbeatConfig saves directives to HEARTBEAT.md and updates settings.
func (tm *TaskManager) SaveHeartbeatConfig(ctx context.Context, cfg HeartbeatConfig) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if cfg.Directives != "" {
		hbPath := filepath.Join(tm.workspaceDir, "HEARTBEAT.md")
		_ = os.WriteFile(hbPath, []byte(cfg.Directives), 0644)
	}
	slog.Info("heartbeat directives and configuration saved", "interval_mins", cfg.IntervalMinutes)
	return nil
}

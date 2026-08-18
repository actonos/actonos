package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver (CGO_ENABLED=0)
)

// DB wraps the SQLite database connection and provides schema migration and transactional support.
type DB struct {
	db *sql.DB
}

// Open initializes or connects to an SQLite database at dbPath with optimized performance PRAGMAs.
func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1) // Single-writer SQLite safety with WAL mode
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging sqlite db: %w", err)
	}

	instance := &DB{db: db}
	if err := instance.migrate(); err != nil {
		return nil, fmt.Errorf("running sqlite migrations: %w", err)
	}

	return instance, nil
}

// Close closes the underlying SQLite database.
func (d *DB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// SQLDB returns the underlying *sql.DB.
func (d *DB) SQLDB() *sql.DB {
	return d.db
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		manifest_json TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'stopped',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		title TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		tool_calls_json TEXT,
		created_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		layer TEXT NOT NULL,
		content TEXT NOT NULL,
		metadata_json TEXT,
		importance_weight REAL NOT NULL DEFAULT 1.0,
		last_accessed_at TIMESTAMP NOT NULL,
		access_count INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_memories_agent_layer ON memories(agent_id, layer);
	CREATE INDEX IF NOT EXISTS idx_memories_last_accessed ON memories(last_accessed_at);

	CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
		id UNINDEXED,
		agent_id,
		layer,
		content,
		tokenize='porter unicode61'
	);

	CREATE TABLE IF NOT EXISTS oauth_tokens (
		provider TEXT PRIMARY KEY,
		encrypted_data TEXT NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS vault_entries (
		key_name TEXT PRIMARY KEY,
		encrypted_val TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS system_auth (
		id TEXT PRIMARY KEY,
		password_hash TEXT NOT NULL,
		salt TEXT NOT NULL,
		is_initialized BOOLEAN NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS token_usage (
		id TEXT PRIMARY KEY,
		timestamp TIMESTAMP NOT NULL,
		agent_id TEXT NOT NULL,
		model TEXT NOT NULL,
		provider TEXT NOT NULL,
		prompt_tokens INTEGER NOT NULL,
		completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL,
		estimated_cost_usd REAL NOT NULL DEFAULT 0.0,
		source TEXT NOT NULL,
		conversation_id TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_token_usage_timestamp ON token_usage(timestamp);
	CREATE INDEX IF NOT EXISTS idx_token_usage_agent ON token_usage(agent_id);
	CREATE INDEX IF NOT EXISTS idx_token_usage_model ON token_usage(model);

	CREATE TABLE IF NOT EXISTS cron_execution_history (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		status TEXT NOT NULL,
		prompt TEXT NOT NULL,
		output TEXT,
		error TEXT,
		duration_ms INTEGER NOT NULL,
		tokens_used INTEGER NOT NULL DEFAULT 0,
		executed_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_cron_history_job ON cron_execution_history(job_id);
	CREATE INDEX IF NOT EXISTS idx_cron_history_executed ON cron_execution_history(executed_at);

	CREATE TABLE IF NOT EXISTS heartbeat_runs (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		executed_at TIMESTAMP NOT NULL,
		status TEXT NOT NULL,
		summary TEXT,
		tokens_used INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_heartbeat_runs_executed ON heartbeat_runs(executed_at);

	CREATE TABLE IF NOT EXISTS approvals (
		id TEXT PRIMARY KEY,
		trace_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		risk_level TEXT NOT NULL,
		action_hash TEXT NOT NULL,
		input_json TEXT NOT NULL,
		status TEXT NOT NULL,
		reason TEXT,
		requested_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		decided_at TIMESTAMP,
		decided_by TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status, requested_at);
	CREATE INDEX IF NOT EXISTS idx_approvals_action_hash ON approvals(action_hash);

	CREATE TABLE IF NOT EXISTS agent_runs (
		id TEXT PRIMARY KEY,
		trace_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		goal TEXT NOT NULL,
		source TEXT NOT NULL,
		status TEXT NOT NULL,
		termination_reason TEXT NOT NULL DEFAULT '',
		iterations INTEGER NOT NULL DEFAULT 0,
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		completion_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		started_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		checkpoint_json TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_agent_runs_trace ON agent_runs(trace_id);
	CREATE INDEX IF NOT EXISTS idx_agent_runs_started ON agent_runs(started_at);

	CREATE TABLE IF NOT EXISTS run_events (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		trace_id TEXT NOT NULL,
		step INTEGER NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		tool_name TEXT,
		data_json TEXT,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_run_events_run ON run_events(run_id, created_at);

	CREATE TABLE IF NOT EXISTS context_snapshots (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		summary TEXT NOT NULL,
		source_message_count INTEGER NOT NULL,
		retained_message_count INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_context_snapshots_run ON context_snapshots(run_id, created_at);
	`

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := d.db.ExecContext(ctx, schema)
	if err == nil {
		_, _ = d.db.ExecContext(ctx, "ALTER TABLE agent_runs ADD COLUMN checkpoint_json TEXT")
	}
	return err
}

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
	`

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := d.db.ExecContext(ctx, schema)
	return err
}

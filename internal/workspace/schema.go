package workspace

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 1

// Migrate creates the durable, database-backed user workspace schema. The
// migration is deliberately owned by this package so every consumer (daemon,
// tests, and maintenance commands) observes the same storage contract.
func Migrate(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("migrating workspace schema: database is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning workspace migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workspace_schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS workspace_nodes (
			id TEXT PRIMARY KEY,
			parent_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			node_type TEXT NOT NULL CHECK(node_type IN ('file', 'directory')),
			mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
			size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(size_bytes >= 0),
			content_hash TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1 CHECK(version > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_nodes_active_name
			ON workspace_nodes(parent_id, name) WHERE deleted_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_workspace_nodes_parent
			ON workspace_nodes(parent_id, deleted_at, name);
		CREATE INDEX IF NOT EXISTS idx_workspace_nodes_updated
			ON workspace_nodes(updated_at);

		CREATE TABLE IF NOT EXISTS workspace_revisions (
			id TEXT PRIMARY KEY,
			node_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			content BLOB NOT NULL,
			content_hash TEXT NOT NULL,
			size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(node_id) REFERENCES workspace_nodes(id) ON DELETE CASCADE,
			UNIQUE(node_id, version)
		);
		CREATE INDEX IF NOT EXISTS idx_workspace_revisions_node
			ON workspace_revisions(node_id, version DESC);

		CREATE VIRTUAL TABLE IF NOT EXISTS workspace_fts USING fts5(
			node_id UNINDEXED,
			name,
			content,
			tokenize='unicode61'
		);

		CREATE TABLE IF NOT EXISTS workspace_migration_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS workspace_legacy_imports (
			source_path TEXT PRIMARY KEY,
			node_id TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			imported_at TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("applying workspace schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO workspace_schema_migrations(version, applied_at)
		VALUES (?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, schemaVersion); err != nil {
		return fmt.Errorf("recording workspace schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing workspace migration: %w", err)
	}
	return nil
}

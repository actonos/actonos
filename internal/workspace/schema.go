package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

const schemaVersion = 2

// Migrate creates the metadata-only workspace schema. File bytes are stored
// below root and are never persisted in SQLite.
func Migrate(ctx context.Context, db *sql.DB, root string) error {
	if db == nil {
		return fmt.Errorf("migrating workspace schema: database is nil")
	}
	if root == "" {
		return fmt.Errorf("migrating workspace schema: filesystem root is empty")
	}
	if err := os.MkdirAll(root, 0750); err != nil {
		return fmt.Errorf("creating workspace filesystem root: %w", err)
	}
	compatible, err := workspaceSchemaCompatible(ctx, db)
	if err != nil {
		return err
	}
	if !compatible {
		return fmt.Errorf("legacy workspace BLOB schema is not supported; initialize a fresh database")
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
			relative_path TEXT NOT NULL DEFAULT '',
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
		CREATE VIRTUAL TABLE IF NOT EXISTS workspace_fts USING fts5(
			node_id UNINDEXED,
			name,
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
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_schema_migrations(version, applied_at)
		VALUES (?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, schemaVersion); err != nil {
		return fmt.Errorf("recording workspace schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing workspace migration: %w", err)
	}
	return nil
}

func workspaceSchemaCompatible(ctx context.Context, db *sql.DB) (bool, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'workspace_nodes'`).Scan(&exists); err != nil {
		return false, fmt.Errorf("inspecting workspace schema: %w", err)
	}
	if exists == 0 {
		return true, nil
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(workspace_nodes)`)
	if err != nil {
		return false, fmt.Errorf("inspecting workspace node columns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scanning workspace node columns: %w", err)
		}
		if name == "relative_path" {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterating workspace node columns: %w", err)
	}
	return false, nil
}

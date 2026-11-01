package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) Rename(ctx context.Context, id, newParentID, newName string, expectedVersion int64) (Node, error) {
	if err := ValidateName(newName); err != nil {
		return Node{}, err
	}
	if newParentID == id {
		return Node{}, fmt.Errorf("%w: node cannot be its own parent", ErrInvalidNode)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, fmt.Errorf("beginning workspace rename: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureParent(ctx, tx, newParentID); err != nil {
		return Node{}, err
	}
	if newParentID != "" {
		var descendant int
		err := tx.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (
			SELECT id FROM workspace_nodes WHERE parent_id = ? AND deleted_at IS NULL
			UNION ALL SELECT n.id FROM workspace_nodes n JOIN descendants d ON n.parent_id = d.id WHERE n.deleted_at IS NULL
		) SELECT COUNT(*) FROM descendants WHERE id = ?`, id, newParentID).Scan(&descendant)
		if err != nil {
			return Node{}, fmt.Errorf("checking workspace move: %w", err)
		}
		if descendant > 0 {
			return Node{}, fmt.Errorf("%w: cannot move a directory into its descendant", ErrInvalidNode)
		}
	}
	var nodeType string
	var version int64
	var deleted sql.NullString
	var currentRelativePath string
	err = tx.QueryRowContext(ctx, `SELECT node_type, version, deleted_at, relative_path
		FROM workspace_nodes WHERE id = ?`, id).Scan(&nodeType, &version, &deleted, &currentRelativePath)
	if errors.Is(err, sql.ErrNoRows) || deleted.Valid {
		return Node{}, ErrNotFound
	}
	if err != nil {
		return Node{}, fmt.Errorf("loading workspace node for rename: %w", err)
	}
	if expectedVersion > 0 && version != expectedVersion {
		return Node{}, ErrVersion
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	newRelativePath := currentRelativePath
	if nodeType == "file" {
		newRelativePath, err = storageRelativePath(newParentID, id)
		if err != nil {
			return Node{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE workspace_nodes SET parent_id = ?, name = ?, relative_path = ?, version = version + 1,
		updated_at = ? WHERE id = ? AND deleted_at IS NULL`, newParentID, newName, newRelativePath, now, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Node{}, ErrConflict
		}
		return Node{}, fmt.Errorf("renaming workspace node: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Node{}, ErrNotFound
	}
	var rollbackMove = func() {}
	if nodeType == "file" {
		if newRelativePath != currentRelativePath {
			currentTarget, err := s.storagePath(currentRelativePath)
			if err != nil {
				return Node{}, err
			}
			newTarget, err := s.storagePath(newRelativePath)
			if err != nil {
				return Node{}, err
			}
			if err := os.MkdirAll(filepath.Dir(newTarget), 0750); err != nil {
				return Node{}, fmt.Errorf("creating workspace move target: %w", err)
			}
			if err := os.Rename(currentTarget, newTarget); err != nil {
				return Node{}, fmt.Errorf("moving workspace file: %w", err)
			}
			rollbackMove = func() { _ = os.Rename(newTarget, currentTarget) }
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workspace_fts SET name = ? WHERE node_id = ?`, newName, id); err != nil {
			rollbackMove()
			return Node{}, fmt.Errorf("updating workspace search name: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		rollbackMove()
		return Node{}, fmt.Errorf("committing workspace rename: %w", err)
	}
	s.refreshNamed(ctx)
	return s.Get(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id string, expectedVersion int64, recursive bool) error {
	node, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if expectedVersion > 0 && node.Version != expectedVersion {
		return ErrVersion
	}
	if node.Type == "directory" && !recursive {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_nodes WHERE parent_id = ? AND deleted_at IS NULL`, id).Scan(&count); err != nil {
			return fmt.Errorf("checking workspace directory contents: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("%w: directory is not empty", ErrConflict)
		}
	}
	storagePaths, directoryIDs, err := s.deletionStorageTargets(ctx, id, recursive)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning workspace delete: %w", err)
	}
	defer tx.Rollback()
	now := s.now().UTC().Format(time.RFC3339Nano)
	if recursive {
		if _, err := tx.ExecContext(ctx, `WITH RECURSIVE targets(id) AS (
			SELECT id FROM workspace_nodes WHERE id = ? AND deleted_at IS NULL
			UNION ALL SELECT n.id FROM workspace_nodes n JOIN targets t ON n.parent_id = t.id WHERE n.deleted_at IS NULL
		) UPDATE workspace_nodes SET deleted_at = ?, updated_at = ?, version = version + 1 WHERE id IN (SELECT id FROM targets)`, id, now, now); err != nil {
			return fmt.Errorf("deleting workspace tree: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_fts WHERE node_id IN (
			WITH RECURSIVE targets(id) AS (
				SELECT id FROM workspace_nodes WHERE id = ?
				UNION ALL SELECT n.id FROM workspace_nodes n JOIN targets t ON n.parent_id = t.id
			) SELECT id FROM targets
		)`, id); err != nil {
			return fmt.Errorf("removing workspace search tree: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE workspace_nodes SET deleted_at = ?, updated_at = ?,
			version = version + 1 WHERE id = ? AND deleted_at IS NULL`, now, now, id); err != nil {
			return fmt.Errorf("deleting workspace node: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_fts WHERE node_id = ?`, id); err != nil {
			return fmt.Errorf("removing workspace search document: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing workspace delete: %w", err)
	}
	for _, relativePath := range storagePaths {
		target, pathErr := s.storagePath(relativePath)
		if pathErr != nil {
			return pathErr
		}
		if removeErr := os.Remove(target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("removing workspace file: %w", removeErr)
		}
	}
	for _, directoryID := range directoryIDs {
		if removeErr := os.RemoveAll(filepath.Join(s.root, directoryID)); removeErr != nil {
			return fmt.Errorf("removing workspace directory storage: %w", removeErr)
		}
	}
	s.refreshNamed(ctx)
	return nil
}

func (s *Store) deletionStorageTargets(ctx context.Context, id string, recursive bool) ([]string, []string, error) {
	query := `SELECT n.id, n.node_type, n.relative_path
		FROM workspace_nodes n WHERE n.id = ? AND n.deleted_at IS NULL`
	if recursive {
		query = `WITH RECURSIVE targets(id) AS (
			SELECT id FROM workspace_nodes WHERE id = ? AND deleted_at IS NULL
			UNION ALL SELECT n.id FROM workspace_nodes n JOIN targets t ON n.parent_id = t.id WHERE n.deleted_at IS NULL
		) SELECT n.id, n.node_type, n.relative_path
			FROM workspace_nodes n JOIN targets t ON t.id = n.id`
	}
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, nil, fmt.Errorf("loading workspace deletion targets: %w", err)
	}
	defer rows.Close()
	paths := make([]string, 0)
	directories := make([]string, 0)
	for rows.Next() {
		var nodeID, nodeType, relativePath string
		if err := rows.Scan(&nodeID, &nodeType, &relativePath); err != nil {
			return nil, nil, fmt.Errorf("scanning workspace deletion target: %w", err)
		}
		if nodeType == "directory" {
			directories = append(directories, nodeID)
		} else if relativePath != "" {
			paths = append(paths, relativePath)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating workspace deletion targets: %w", err)
	}
	return paths, directories, nil
}

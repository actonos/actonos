package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) Write(ctx context.Context, req WriteRequest) (Node, error) {
	if req.ID == "" && ValidateName(req.Name) != nil {
		return Node{}, ErrInvalidName
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, fmt.Errorf("beginning workspace write: %w", err)
	}
	defer tx.Rollback()

	now := s.now().UTC().Format(time.RFC3339Nano)
	hash := hashContent(req.Content)
	nodeID := req.ID
	version := int64(1)
	name := req.Name
	mimeType := req.MIMEType
	parentID := req.ParentID
	var relativePath string
	if nodeID == "" {
		if err := s.ensureParent(ctx, tx, req.ParentID); err != nil {
			return Node{}, err
		}
		nodeID = uuid.NewString()
		mimeType = detectMIME(name, mimeType, req.Content)
		relativePath, err = storageRelativePath(parentID, nodeID)
		if err != nil {
			return Node{}, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO workspace_nodes
			(id, parent_id, name, node_type, mime_type, size_bytes, content_hash, relative_path, version, created_at, updated_at)
			VALUES (?, ?, ?, 'file', ?, ?, ?, ?, 1, ?, ?)`, nodeID, req.ParentID, name, mimeType, len(req.Content), hash, relativePath, now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return Node{}, ErrConflict
			}
			return Node{}, fmt.Errorf("creating workspace file: %w", err)
		}
	} else {
		var nodeType string
		var deleted sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT name, parent_id, node_type, mime_type, version,
			deleted_at, relative_path FROM workspace_nodes WHERE id = ?`, nodeID).
			Scan(&name, &parentID, &nodeType, &mimeType, &version, &deleted, &relativePath)
		if errors.Is(err, sql.ErrNoRows) || deleted.Valid {
			return Node{}, ErrNotFound
		}
		if err != nil {
			return Node{}, fmt.Errorf("loading workspace file for update: %w", err)
		}
		if nodeType != "file" {
			return Node{}, fmt.Errorf("%w: node is not a file", ErrInvalidNode)
		}
		if req.ExpectedVersion > 0 && req.ExpectedVersion != version {
			return Node{}, ErrVersion
		}
		version++
		mimeType = detectMIME(name, req.MIMEType, req.Content)
		result, updateErr := tx.ExecContext(ctx, `UPDATE workspace_nodes SET mime_type = ?, size_bytes = ?,
			content_hash = ?, version = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
			mimeType, len(req.Content), hash, version, now, nodeID)
		if updateErr != nil {
			return Node{}, fmt.Errorf("updating workspace file: %w", updateErr)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return Node{}, ErrNotFound
		}
	}

	target, err := s.storagePath(relativePath)
	if err != nil {
		return Node{}, err
	}
	staged, err := stageWorkspaceFile(target, req.Content)
	if err != nil {
		return Node{}, fmt.Errorf("staging workspace file: %w", err)
	}
	defer os.Remove(staged)

	if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_fts WHERE node_id = ?`, nodeID); err != nil {
		return Node{}, fmt.Errorf("clearing workspace search document: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_fts(node_id, name) VALUES (?, ?)`, nodeID, name); err != nil {
		return Node{}, fmt.Errorf("indexing workspace file: %w", err)
	}
	rollbackFile, finalizeFile, err := installWorkspaceFile(staged, target)
	if err != nil {
		return Node{}, fmt.Errorf("installing workspace file: %w", err)
	}
	if err := tx.Commit(); err != nil {
		rollbackFile()
		return Node{}, fmt.Errorf("committing workspace write: %w", err)
	}
	finalizeFile()
	s.refreshNamed(ctx)
	return s.Get(ctx, nodeID)
}

func (s *Store) Read(ctx context.Context, id string, offset, limit int64) (Node, []byte, error) {
	if offset < 0 || limit < 0 {
		return Node{}, nil, fmt.Errorf("%w: negative read range", ErrInvalidNode)
	}
	node, file, err := s.Open(ctx, id)
	if err != nil {
		return Node{}, nil, err
	}
	defer file.Close()
	if offset >= node.SizeBytes {
		return node, []byte{}, nil
	}
	if limit == 0 || offset+limit > node.SizeBytes {
		limit = node.SizeBytes - offset
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return Node{}, nil, fmt.Errorf("seeking workspace file: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return Node{}, nil, fmt.Errorf("reading workspace file: %w", err)
	}
	return node, content, nil
}

// Open returns the current immutable file handle for streaming callers.
func (s *Store) Open(ctx context.Context, id string) (Node, *os.File, error) {
	node, err := s.Get(ctx, id)
	if err != nil {
		return Node{}, nil, err
	}
	if node.Type != "file" {
		return Node{}, nil, fmt.Errorf("%w: node is not a file", ErrInvalidNode)
	}
	target, err := s.storagePath(node.RelativePath)
	if err != nil {
		return Node{}, nil, err
	}
	file, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return Node{}, nil, ErrNotFound
	}
	if err != nil {
		return Node{}, nil, fmt.Errorf("opening workspace file: %w", err)
	}
	return node, file, nil
}
func stageWorkspaceFile(target string, content []byte) (string, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".workspace-*")
	if err != nil {
		return "", err
	}
	name := temporary.Name()
	if err := temporary.Chmod(0640); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func installWorkspaceFile(staged, target string) (rollback func(), finalize func(), err error) {
	backup := target + ".backup-" + uuid.NewString()
	hadOriginal := false
	if err := os.Rename(target, backup); err == nil {
		hadOriginal = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if err := os.Rename(staged, target); err != nil {
		if hadOriginal {
			_ = os.Rename(backup, target)
		}
		return nil, nil, err
	}
	rollback = func() {
		_ = os.Remove(target)
		if hadOriginal {
			_ = os.Rename(backup, target)
		}
	}
	finalize = func() {
		if hadOriginal {
			_ = os.Remove(backup)
		}
	}
	return rollback, finalize, nil
}

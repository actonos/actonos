package workspace

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const legacyMigrationKey = "legacy_filesystem_import_v1"

type MigrationReport struct {
	AlreadyCompleted  bool     `json:"already_completed"`
	ImportedFiles     int      `json:"imported_files"`
	ImportedFolders   int      `json:"imported_folders"`
	ImportedBytes     int64    `json:"imported_bytes"`
	CopiedAgentFiles  int      `json:"copied_agent_files"`
	CopiedAgentBytes  int64    `json:"copied_agent_bytes"`
	SkippedSymlinks   []string `json:"skipped_symlinks"`
	Conflicts         []string `json:"conflicts"`
	PreservedLegacyAt string   `json:"preserved_legacy_at"`
}

// ImportLegacy performs an idempotent, non-destructive migration. Known agent
// directories are copied into their private roots; every other legacy entry is
// imported into SQLite. The source tree is intentionally preserved for rollback.
func (s *Store) ImportLegacy(ctx context.Context, legacyRoot, agentsRoot string, agentIDs []string) (MigrationReport, error) {
	report := MigrationReport{PreservedLegacyAt: legacyRoot}
	if legacyRoot == "" {
		return report, nil
	}
	var state string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM workspace_migration_state WHERE key = ?`, legacyMigrationKey).Scan(&state)
	if err == nil && state == "complete" {
		report.AlreadyCompleted = true
		return report, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return report, fmt.Errorf("checking legacy workspace migration: %w", err)
	}
	info, err := os.Stat(legacyRoot)
	if errors.Is(err, os.ErrNotExist) {
		return report, s.markMigrationComplete(ctx)
	}
	if err != nil {
		return report, fmt.Errorf("inspecting legacy workspace: %w", err)
	}
	if !info.IsDir() {
		return report, fmt.Errorf("legacy workspace %q is not a directory", legacyRoot)
	}

	agents := make(map[string]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		if validAgentSlug(id) {
			agents[id] = struct{}{}
		}
	}
	entries, err := os.ReadDir(legacyRoot)
	if err != nil {
		return report, fmt.Errorf("reading legacy workspace: %w", err)
	}
	for _, entry := range entries {
		if _, ok := agents[entry.Name()]; ok && entry.IsDir() {
			source := filepath.Join(legacyRoot, entry.Name())
			destination := filepath.Join(agentsRoot, entry.Name(), "workspace")
			if err := copyAgentTree(ctx, source, destination, &report); err != nil {
				return report, fmt.Errorf("copying legacy agent workspace %s: %w", entry.Name(), err)
			}
			continue
		}
		if _, err := s.importLegacyEntry(ctx, legacyRoot, filepath.Join(legacyRoot, entry.Name()), "", &report); err != nil {
			return report, err
		}
	}
	sort.Strings(report.SkippedSymlinks)
	sort.Strings(report.Conflicts)
	if err := s.markMigrationComplete(ctx); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Store) markMigrationComplete(ctx context.Context) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO workspace_migration_state(key, value, updated_at)
		VALUES (?, 'complete', ?) ON CONFLICT(key) DO UPDATE SET value = 'complete', updated_at = excluded.updated_at`, legacyMigrationKey, now)
	if err != nil {
		return fmt.Errorf("recording legacy workspace migration: %w", err)
	}
	return nil
}

func validAgentSlug(slug string) bool {
	if !strings.HasPrefix(slug, "agent_") || slug == "" {
		return false
	}
	for _, r := range slug {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func copyAgentTree(ctx context.Context, source, destination string, report *MigrationReport) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			report.SkippedSymlinks = append(report.SkippedSymlinks, path)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		if !info.Mode().IsRegular() {
			report.SkippedSymlinks = append(report.SkippedSymlinks, path)
			return nil
		}
		if existing, statErr := os.Stat(target); statErr == nil {
			equal, compareErr := regularFilesEqual(path, target, info.Size(), existing.Size())
			if compareErr != nil {
				return compareErr
			}
			if equal {
				return nil
			}
			report.Conflicts = append(report.Conflicts, target)
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
		if err != nil {
			_ = src.Close()
			return err
		}
		written, copyErr := io.Copy(dst, src)
		sourceCloseErr := src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if sourceCloseErr != nil {
			return sourceCloseErr
		}
		if closeErr != nil {
			return closeErr
		}
		report.CopiedAgentFiles++
		report.CopiedAgentBytes += written
		return nil
	})
}

func regularFilesEqual(source, target string, sourceSize, targetSize int64) (bool, error) {
	if sourceSize != targetSize {
		return false, nil
	}
	sourceData, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return false, err
	}
	return bytes.Equal(sourceData, targetData), nil
}

func (s *Store) importLegacyEntry(ctx context.Context, root, path, parentID string, report *MigrationReport) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspecting legacy workspace entry %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		report.SkippedSymlinks = append(report.SkippedSymlinks, path)
		return "", nil
	}
	name := info.Name()
	if err := ValidateName(name); err != nil {
		report.Conflicts = append(report.Conflicts, path+": invalid UTF-8 or name length")
		return "", nil
	}
	var existingID string
	_ = s.db.QueryRowContext(ctx, `SELECT node_id FROM workspace_legacy_imports WHERE source_path = ? AND status = 'imported'`, path).Scan(&existingID)
	if existingID != "" {
		return existingID, nil
	}
	if info.IsDir() {
		directory, err := s.CreateDirectory(ctx, parentID, name)
		if errors.Is(err, ErrConflict) {
			err = s.db.QueryRowContext(ctx, `SELECT id FROM workspace_nodes WHERE parent_id = ? AND name = ? AND node_type = 'directory' AND deleted_at IS NULL`, parentID, name).Scan(&directory.ID)
		}
		if err != nil {
			return "", fmt.Errorf("importing legacy directory %q: %w", path, err)
		}
		report.ImportedFolders++
		children, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("reading legacy directory %q: %w", path, err)
		}
		for _, child := range children {
			if _, err := s.importLegacyEntry(ctx, root, filepath.Join(path, child.Name()), directory.ID, report); err != nil {
				return "", err
			}
		}
		return directory.ID, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading legacy file %q: %w", path, err)
	}
	node, err := s.Write(ctx, WriteRequest{ParentID: parentID, Name: name, Content: content, ActorID: "migration"})
	if errors.Is(err, ErrConflict) {
		report.Conflicts = append(report.Conflicts, path)
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("importing legacy file %q: %w", path, err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO workspace_legacy_imports
		(source_path, node_id, content_hash, size_bytes, status, detail, imported_at)
		VALUES (?, ?, ?, ?, 'imported', ?, ?)`, path, node.ID, node.ContentHash, node.SizeBytes, filepath.Clean(root), now); err != nil {
		return "", fmt.Errorf("recording legacy file import %q: %w", path, err)
	}
	report.ImportedFiles++
	report.ImportedBytes += int64(len(content))
	return node.ID, nil
}

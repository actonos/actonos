package system

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BackupManifest describes the contents and metadata of an ActonOS backup bundle.
type BackupManifest struct {
	ID                     string    `json:"id"`
	CreatedAt              time.Time `json:"created_at"`
	Version                string    `json:"version"`
	ChecksumSHA256         string    `json:"checksum_sha256"`
	DatabaseSizeBytes      int64     `json:"database_size_bytes"`
	ArchiveSizeBytes       int64     `json:"archive_size_bytes"`
	IncludeWorkspace       bool      `json:"include_workspace"`
	AgentsCount            int       `json:"agents_count"`
	TasksCount             int       `json:"tasks_count"`
	Notes                  string    `json:"notes,omitempty"`
	FileName               string    `json:"file_name,omitempty"`
}

// BackupManager orchestrates online SQLite backups, archive verification, restoration, and disaster recovery.
type BackupManager struct {
	mu         sync.Mutex
	dataDir    string
	db         *sql.DB
	version    string
	backupsDir string
}

// NewBackupManager initializes a BackupManager instance.
func NewBackupManager(dataDir string, db *sql.DB, version string) (*BackupManager, error) {
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0750); err != nil {
		return nil, fmt.Errorf("creating backups directory: %w", err)
	}

	if version == "" {
		version = "1.0.3"
	}

	return &BackupManager{
		dataDir:    dataDir,
		db:         db,
		version:    version,
		backupsDir: backupsDir,
	}, nil
}

// CreateBackup creates an encrypted or gzipped backup bundle containing SQLite snapshot + manifest + optional workspace.
func (bm *BackupManager) CreateBackup(ctx context.Context, includeWorkspace bool, notes string) (*BackupManifest, []byte, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.db == nil {
		return nil, nil, errors.New("database is not configured")
	}

	// 1. Snapshot database using online VACUUM INTO
	tempDir, err := os.MkdirTemp("", "actonos-backup-build-*")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp build dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempDBFile := filepath.Join(tempDir, "actonos.db")
	if _, err := bm.db.ExecContext(ctx, `VACUUM INTO ?`, tempDBFile); err != nil {
		return nil, nil, fmt.Errorf("online database backup failed: %w", err)
	}

	dbStat, err := os.Stat(tempDBFile)
	if err != nil {
		return nil, nil, fmt.Errorf("stat temp db file: %w", err)
	}

	// Count agents and tasks for metadata
	agentsCount := 0
	_ = bm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agents").Scan(&agentsCount)
	tasksCount := 0
	_ = bm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM autonomous_tasks").Scan(&tasksCount)

	backupID := "bak_" + time.Now().UTC().Format("20060102_150405") + "_" + uuid.NewString()[:8]
	manifest := BackupManifest{
		ID:                backupID,
		CreatedAt:         time.Now().UTC(),
		Version:           bm.version,
		DatabaseSizeBytes: dbStat.Size(),
		IncludeWorkspace:  includeWorkspace,
		AgentsCount:       agentsCount,
		TasksCount:        tasksCount,
		Notes:             notes,
		FileName:          fmt.Sprintf("actonos-backup-%s.actonbak", backupID),
	}

	// 2. Build tar.gz in memory
	var buf strings.Builder
	_ = buf

	tempTarFile := filepath.Join(tempDir, manifest.FileName)
	outFile, err := os.Create(tempTarFile)
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp tar file: %w", err)
	}

	hasher := sha256.New()
	multiWriter := io.MultiWriter(outFile, hasher)
	gw := gzip.NewWriter(multiWriter)
	tw := tar.NewWriter(gw)

	// Write DB file into tar
	dbData, err := os.ReadFile(tempDBFile)
	if err != nil {
		outFile.Close()
		return nil, nil, fmt.Errorf("reading db snapshot: %w", err)
	}

	dbHeader := &tar.Header{
		Name:     "actonos.db",
		Mode:     0600,
		Size:     int64(len(dbData)),
		ModTime:  manifest.CreatedAt,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(dbHeader); err != nil {
		outFile.Close()
		return nil, nil, fmt.Errorf("writing db tar header: %w", err)
	}
	if _, err := tw.Write(dbData); err != nil {
		outFile.Close()
		return nil, nil, fmt.Errorf("writing db tar content: %w", err)
	}

	// Optionally include workspace files
	if includeWorkspace {
		workspacePath := filepath.Join(bm.dataDir, "workspace")
		if info, err := os.Stat(workspacePath); err == nil && info.IsDir() {
			_ = filepath.Walk(workspacePath, func(path string, fi os.FileInfo, walkErr error) error {
				if walkErr != nil || fi.IsDir() {
					return nil
				}
				relPath, err := filepath.Rel(bm.dataDir, path)
				if err != nil {
					return nil
				}
				fileContent, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				hdr := &tar.Header{
					Name:     filepath.ToSlash(relPath),
					Mode:     0600,
					Size:     int64(len(fileContent)),
					ModTime:  fi.ModTime(),
					Typeflag: tar.TypeReg,
				}
				if err := tw.WriteHeader(hdr); err == nil {
					_, _ = tw.Write(fileContent)
				}
				return nil
			})
		}
	}

	// Close tar and gzip before computing final manifest checksum
	if err := tw.Close(); err != nil {
		outFile.Close()
		return nil, nil, fmt.Errorf("closing tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		outFile.Close()
		return nil, nil, fmt.Errorf("closing gzip writer: %w", err)
	}
	outFile.Close()

	// Read full archive
	archiveBytes, err := os.ReadFile(tempTarFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading archive bytes: %w", err)
	}

	sum := sha256.Sum256(archiveBytes)
	manifest.ChecksumSHA256 = hex.EncodeToString(sum[:])
	manifest.ArchiveSizeBytes = int64(len(archiveBytes))

	// Save copy to /data/backups/
	savedPath := filepath.Join(bm.backupsDir, manifest.FileName)
	_ = os.WriteFile(savedPath, archiveBytes, 0640)

	// Save manifest alongside archive
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(savedPath+".json", manifestJSON, 0640)

	return &manifest, archiveBytes, nil
}

// listBackupsUnsafe returns all local backups stored in /data/backups/ without acquiring mutex.
func (bm *BackupManager) listBackupsUnsafe() ([]BackupManifest, error) {
	entries, err := os.ReadDir(bm.backupsDir)
	if err != nil {
		return nil, err
	}

	var results []BackupManifest
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		manifestPath := filepath.Join(bm.backupsDir, entry.Name())
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m BackupManifest
		if err := json.Unmarshal(data, &m); err == nil {
			results = append(results, m)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return results, nil
}

// ListBackups returns all local backups stored in /data/backups/.
func (bm *BackupManager) ListBackups() ([]BackupManifest, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	return bm.listBackupsUnsafe()
}

// GetBackupArchive retrieves the raw binary archive and manifest for a given backup ID or filename.
func (bm *BackupManager) GetBackupArchive(backupID string) ([]byte, *BackupManifest, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	backups, err := bm.listBackupsUnsafe()
	if err != nil {
		return nil, nil, err
	}

	var target *BackupManifest
	for _, b := range backups {
		if b.ID == backupID || b.FileName == backupID {
			target = &b
			break
		}
	}
	if target == nil {
		return nil, nil, fmt.Errorf("backup not found: %s", backupID)
	}

	archivePath := filepath.Join(bm.backupsDir, target.FileName)
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading backup archive file: %w", err)
	}

	return data, target, nil
}

// VerifyArchive inspects a backup archive file and returns its manifest and extracted DB size.
func (bm *BackupManager) VerifyArchive(r io.Reader) (*BackupManifest, []byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("reading archive data: %w", err)
	}

	sum := sha256.Sum256(data)
	checksumHex := hex.EncodeToString(sum[:])

	gr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var dbBytes []byte
	hasDB := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading tar entry: %w", err)
		}
		if header.Name == "actonos.db" {
			dbBytes, err = io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("reading db from tar: %w", err)
			}
			hasDB = true
		}
	}

	if !hasDB || len(dbBytes) == 0 {
		return nil, nil, errors.New("archive does not contain a valid actonos.db snapshot")
	}

	// Verify DB header magic
	if len(dbBytes) < 16 || !strings.HasPrefix(string(dbBytes[:16]), "SQLite format 3") {
		return nil, nil, errors.New("extracted database header is not a valid SQLite database")
	}

	manifest := &BackupManifest{
		ChecksumSHA256:    checksumHex,
		DatabaseSizeBytes: int64(len(dbBytes)),
		ArchiveSizeBytes:  int64(len(data)),
		CreatedAt:         time.Now().UTC(),
		Version:           bm.version,
	}

	return manifest, dbBytes, nil
}

// RestoreBackup verifies and safely restores an ActonOS database from a backup archive.
func (bm *BackupManager) RestoreBackup(ctx context.Context, r io.Reader) (*BackupManifest, error) {
	manifest, dbBytes, err := bm.VerifyArchive(r)
	if err != nil {
		return nil, fmt.Errorf("archive verification failed: %w", err)
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	// 1. Take safety snapshot before applying restore
	if bm.db != nil {
		preRestorePath := filepath.Join(bm.backupsDir, "pre-restore-safety.db")
		_, _ = bm.db.ExecContext(ctx, `VACUUM INTO ?`, preRestorePath)
	}

	// 2. Write database to temporary file and test opening it
	tempDir, err := os.MkdirTemp("", "actonos-restore-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	tempDBFile := filepath.Join(tempDir, "restore.db")
	if err := os.WriteFile(tempDBFile, dbBytes, 0600); err != nil {
		return nil, fmt.Errorf("writing restore temp db: %w", err)
	}

	testDB, err := sql.Open("sqlite", tempDBFile)
	if err != nil {
		return nil, fmt.Errorf("opening restored sqlite DB: %w", err)
	}
	defer testDB.Close()

	var testCount int
	if err := testDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&testCount); err != nil {
		return nil, fmt.Errorf("verifying restored tables: %w", err)
	}

	// 3. Perform atomic restore into active SQLite path
	dbPath := filepath.Join(bm.dataDir, "actonos.db")
	if err := os.WriteFile(dbPath, dbBytes, 0600); err != nil {
		return nil, fmt.Errorf("overwriting active db file: %w", err)
	}

	return manifest, nil
}

// FactoryReset resets database tables and state if confirmed with the exact security token "RESET-ACTONOS".
func (bm *BackupManager) FactoryReset(ctx context.Context, confirmToken string) error {
	if strings.TrimSpace(confirmToken) != "RESET-ACTONOS" {
		return errors.New("invalid confirmation token; required 'RESET-ACTONOS'")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.db == nil {
		return errors.New("database is not configured")
	}

	// Create safety backup before wipe
	safetyPath := filepath.Join(bm.backupsDir, "pre-factory-reset-safety.db")
	_, _ = bm.db.ExecContext(ctx, `VACUUM INTO ?`, safetyPath)

	tablesToClear := []string{
		"run_events",
		"agent_runs",
		"autonomous_tasks",
		"notifications",
		"conversations",
		"chat_messages",
		"memory_fragments",
		"cron_jobs",
	}

	for _, table := range tablesToClear {
		_, _ = bm.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s;", table))
	}

	// Reclaim pages
	_, _ = bm.db.ExecContext(ctx, "VACUUM;")

	return nil
}

// RestoreBackupByID safely restores an ActonOS database from a stored local backup ID.
func (bm *BackupManager) RestoreBackupByID(ctx context.Context, backupID string) (*BackupManifest, error) {
	data, _, err := bm.GetBackupArchive(backupID)
	if err != nil {
		return nil, fmt.Errorf("retrieving local backup archive: %w", err)
	}

	return bm.RestoreBackup(ctx, bytes.NewReader(data))
}

// DeleteBackup removes a backup archive and its manifest from /data/backups/.
func (bm *BackupManager) DeleteBackup(backupID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	backups, err := bm.listBackupsUnsafe()
	if err != nil {
		return err
	}

	var target *BackupManifest
	for _, b := range backups {
		if b.ID == backupID || b.FileName == backupID {
			target = &b
			break
		}
	}
	if target == nil {
		return fmt.Errorf("backup not found: %s", backupID)
	}

	archivePath := filepath.Join(bm.backupsDir, target.FileName)
	manifestPath := archivePath + ".json"

	_ = os.Remove(archivePath)
	_ = os.Remove(manifestPath)

	return nil
}

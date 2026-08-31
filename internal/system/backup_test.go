package system

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupManagerLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "actonos.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	// Initialize tables and dummy data
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (agent_id TEXT PRIMARY KEY, name TEXT);
		CREATE TABLE IF NOT EXISTS autonomous_tasks (id TEXT PRIMARY KEY, title TEXT);
		INSERT INTO agents VALUES ('agent_test', 'Test Agent');
		INSERT INTO autonomous_tasks VALUES ('task_1', 'Sample Goal');
	`)
	if err != nil {
		t.Fatalf("failed to setup test tables: %v", err)
	}

	bm, err := NewBackupManager(tempDir, db, "1.0.3")
	if err != nil {
		t.Fatalf("failed to create backup manager: %v", err)
	}

	ctx := context.Background()

	// 1. Test Create Backup
	manifest, archiveBytes, err := bm.CreateBackup(ctx, false, "Initial Test Backup")
	if err != nil {
		t.Fatalf("create backup failed: %v", err)
	}
	if manifest.AgentsCount != 1 || manifest.TasksCount != 1 {
		t.Errorf("unexpected counts: agents=%d tasks=%d", manifest.AgentsCount, manifest.TasksCount)
	}
	if len(archiveBytes) == 0 || manifest.ChecksumSHA256 == "" {
		t.Fatalf("expected non-empty archive and checksum")
	}

	// 2. Test List Backups & Get Backup Archive (no deadlock)
	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("list backups failed: %v", err)
	}
	if len(backups) == 0 {
		t.Fatalf("expected at least 1 backup in list")
	}
	if backups[0].ID != manifest.ID {
		t.Errorf("expected backup ID %s, got %s", manifest.ID, backups[0].ID)
	}

	retrievedBytes, retrievedManifest, err := bm.GetBackupArchive(manifest.ID)
	if err != nil {
		t.Fatalf("get backup archive failed: %v", err)
	}
	if len(retrievedBytes) == 0 || retrievedManifest.ID != manifest.ID {
		t.Fatalf("invalid retrieved backup archive data")
	}

	// 3. Test Verify Archive
	verifiedManifest, dbBytes, err := bm.VerifyArchive(bytes.NewReader(archiveBytes))
	if err != nil {
		t.Fatalf("verify archive failed: %v", err)
	}
	if verifiedManifest.ChecksumSHA256 != manifest.ChecksumSHA256 {
		t.Errorf("checksum mismatch: expected %s, got %s", manifest.ChecksumSHA256, verifiedManifest.ChecksumSHA256)
	}
	if len(dbBytes) == 0 {
		t.Errorf("expected non-empty extracted db bytes")
	}

	// 4. Test Restore Backup
	restoredManifest, err := bm.RestoreBackup(ctx, bytes.NewReader(archiveBytes))
	if err != nil {
		t.Fatalf("restore backup failed: %v", err)
	}
	if restoredManifest == nil {
		t.Fatalf("expected non-nil restored manifest")
	}

	// 5. Test Factory Reset Security Guard
	err = bm.FactoryReset(ctx, "WRONG-TOKEN")
	if err == nil {
		t.Fatalf("expected error on invalid reset token")
	}

	err = bm.FactoryReset(ctx, "RESET-ACTONOS")
	if err != nil {
		t.Fatalf("factory reset failed: %v", err)
	}

	var taskCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM autonomous_tasks").Scan(&taskCount)
	if taskCount != 0 {
		t.Errorf("expected 0 tasks after factory reset, got %d", taskCount)
	}
}

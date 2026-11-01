package system

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

// The reset must clear the tables the application actually writes to
// (messages, memories, memories_fts), not just tables of a similar name.
func TestFactoryResetClearsConversationAndMemoryData(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "actonos.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE agents (agent_id TEXT PRIMARY KEY, name TEXT);
		CREATE TABLE conversations (id TEXT PRIMARY KEY, title TEXT);
		CREATE TABLE messages (id TEXT PRIMARY KEY, conversation_id TEXT, content TEXT);
		CREATE TABLE memories (id TEXT PRIMARY KEY, agent_id TEXT, content TEXT);
		CREATE VIRTUAL TABLE memories_fts USING fts5(id UNINDEXED, agent_id, layer, content);
		CREATE TABLE agent_runs (id TEXT PRIMARY KEY);
		CREATE TABLE run_events (id TEXT PRIMARY KEY);
		CREATE TABLE autonomous_tasks (id TEXT PRIMARY KEY, title TEXT);
		INSERT INTO agents VALUES ('agent_keep', 'Keep Me');
		INSERT INTO conversations VALUES ('c1', 'chat');
		INSERT INTO messages VALUES ('m1', 'c1', 'private message');
		INSERT INTO memories VALUES ('mem1', 'agent_keep', 'private memory');
		INSERT INTO memories_fts VALUES ('mem1', 'agent_keep', 'episodic', 'private memory');
		INSERT INTO agent_runs VALUES ('r1');
		INSERT INTO run_events VALUES ('e1');
		INSERT INTO autonomous_tasks VALUES ('t1', 'goal');
	`); err != nil {
		t.Fatal(err)
	}

	bm, err := NewBackupManager(dir, db, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := bm.FactoryReset(context.Background(), "RESET-ACTONOS"); err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}

	for _, table := range []string{"conversations", "messages", "memories", "memories_fts", "agent_runs", "run_events", "autonomous_tasks"} {
		if n := countRows(t, db, table); n != 0 {
			t.Errorf("%s still has %d row(s) after factory reset", table, n)
		}
	}
	if n := countRows(t, db, "agents"); n != 1 {
		t.Errorf("agents must survive a factory reset, have %d rows", n)
	}
}

func TestFactoryResetToleratesTablesThatDoNotExistYet(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "actonos.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bm, err := NewBackupManager(dir, db, "test")
	if err != nil {
		t.Fatal(err)
	}
	// A fresh install may not have created lazily-initialised tables yet.
	if err := bm.FactoryReset(context.Background(), "RESET-ACTONOS"); err != nil {
		t.Fatalf("reset on an empty database must succeed, got %v", err)
	}
}

func TestFactoryResetRejectsWrongTokenWithoutTouchingData(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "actonos.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE messages (id TEXT); INSERT INTO messages VALUES ('m1');`); err != nil {
		t.Fatal(err)
	}
	bm, err := NewBackupManager(dir, db, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "reset-actonos", "RESET", "RESET-ACTONOS-NOW"} {
		if err := bm.FactoryReset(context.Background(), token); err == nil {
			t.Errorf("token %q must be rejected", token)
		}
	}
	if n := countRows(t, db, "messages"); n != 1 {
		t.Fatalf("a rejected reset deleted data: %d rows", n)
	}
}

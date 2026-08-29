package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	_ "modernc.org/sqlite"
)

func TestReflectionSelfReviewCycleAndProposals(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "self_review_test_*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	// Setup agent_runs and run_events tables
	schema := `
	CREATE TABLE IF NOT EXISTS agent_runs (
		id TEXT PRIMARY KEY,
		trace_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		user_prompt TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at TIMESTAMP NOT NULL,
		finished_at TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS run_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		timestamp TIMESTAMP NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating schema: %v", err)
	}

	eventBus := bus.NewEventBus()
	reflection := NewReflectionEngine(nil, nil, nil, eventBus)
	reflection.SetDB(db)
	reflection.SetDataDir(tempDir)

	ctx := context.Background()

	// Seed 3 failed runs and 1 successful run for agent_alpha in the last 24h
	recent := time.Now().UTC().Add(-2 * time.Hour)
	_, _ = db.Exec(`INSERT INTO agent_runs (id, trace_id, agent_id, user_prompt, status, started_at) VALUES ('r1', 't1', 'agent_alpha', 'task 1', 'failed', ?)`, recent)
	_, _ = db.Exec(`INSERT INTO agent_runs (id, trace_id, agent_id, user_prompt, status, started_at) VALUES ('r2', 't2', 'agent_alpha', 'task 2', 'error', ?)`, recent)
	_, _ = db.Exec(`INSERT INTO agent_runs (id, trace_id, agent_id, user_prompt, status, started_at) VALUES ('r3', 't3', 'agent_alpha', 'task 3', 'failed', ?)`, recent)
	_, _ = db.Exec(`INSERT INTO agent_runs (id, trace_id, agent_id, user_prompt, status, started_at) VALUES ('r4', 't4', 'agent_alpha', 'task 4', 'completed', ?)`, recent)

	// Seed 4 tool_failed events for agent_alpha
	_, _ = db.Exec(`INSERT INTO run_events (run_id, agent_id, event_type, timestamp) VALUES ('r1', 'agent_alpha', 'tool_failed', ?)`, recent)
	_, _ = db.Exec(`INSERT INTO run_events (run_id, agent_id, event_type, timestamp) VALUES ('r1', 'agent_alpha', 'tool_failed', ?)`, recent)
	_, _ = db.Exec(`INSERT INTO run_events (run_id, agent_id, event_type, timestamp) VALUES ('r2', 'agent_alpha', 'tool_failed', ?)`, recent)
	_, _ = db.Exec(`INSERT INTO run_events (run_id, agent_id, event_type, timestamp) VALUES ('r3', 'agent_alpha', 'tool_failed', ?)`, recent)

	// 1. Run self-review cycle
	proposals, err := reflection.RunSelfReviewCycle(ctx, "agent_alpha")
	if err != nil {
		t.Fatalf("RunSelfReviewCycle failed: %v", err)
	}

	if len(proposals) < 2 {
		t.Fatalf("expected at least 2 proposals (task failure and tool failure), got %d", len(proposals))
	}

	// 2. Query proposals via ListProposals
	list, err := reflection.ListProposals(ctx, "agent_alpha", "pending")
	if err != nil {
		t.Fatalf("ListProposals failed: %v", err)
	}
	if len(list) != len(proposals) {
		t.Fatalf("expected %d proposals in DB, got %d", len(proposals), len(list))
	}

	// 3. Verify INSIGHTS.md was created
	insightsPath := filepath.Join(tempDir, "agents", "agent_alpha", "INSIGHTS.md")
	content, err := os.ReadFile(insightsPath)
	if err != nil {
		t.Fatalf("reading INSIGHTS.md: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty INSIGHTS.md")
	}

	// 4. Apply first proposal
	first := list[0]
	if err := reflection.ApplyProposal(ctx, first.ID); err != nil {
		t.Fatalf("ApplyProposal failed: %v", err)
	}

	appliedList, err := reflection.ListProposals(ctx, "agent_alpha", "applied")
	if err != nil || len(appliedList) != 1 {
		t.Fatalf("expected 1 applied proposal, got %d (err=%v)", len(appliedList), err)
	}

	// 5. Dismiss second proposal
	second := list[1]
	if err := reflection.DismissProposal(ctx, second.ID); err != nil {
		t.Fatalf("DismissProposal failed: %v", err)
	}

	dismissedList, err := reflection.ListProposals(ctx, "agent_alpha", "dismissed")
	if err != nil || len(dismissedList) != 1 {
		t.Fatalf("expected 1 dismissed proposal, got %d (err=%v)", len(dismissedList), err)
	}
}

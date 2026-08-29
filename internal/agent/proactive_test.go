package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/system"
	_ "modernc.org/sqlite"
)

func TestProactiveEngineProbesAndActions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "proactive_test_*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	taskMgr, err := NewTaskManager(db, tempDir)
	if err != nil {
		t.Fatalf("creating task mgr: %v", err)
	}

	eventBus := bus.NewEventBus()
	pe := NewProactiveEngine(db, tempDir, taskMgr, eventBus)

	// Mock custom checkers
	pe.SetMCPChecker(func(ctx context.Context) ([]string, error) {
		return []string{"mcp-github", "mcp-slack"}, nil
	})
	pe.SetTokenBudgetChecker(func(ctx context.Context) (used, cap int64, err error) {
		return 850000, 1000000, nil // 85% usage
	})
	pe.SetInboundQueueChecker(func(ctx context.Context) (int, error) {
		return 15, nil // 15 unread messages
	})

	// Mock disk space
	system.SetFreeSpaceLookup(func(path string) (free, total uint64, err error) {
		return 100 * 1024 * 1024, 1000 * 1024 * 1024, nil // 90% used
	})
	defer system.SetFreeSpaceLookup(nil)

	// Create a stalled task in DB
	ctx := context.Background()
	_, err = taskMgr.CreateTask(ctx, AutonomousTask{
		Title:         "Stalled Mission",
		Status:        "in_progress",
		StalledCycles: 4,
	})
	if err != nil {
		t.Fatalf("creating stalled task: %v", err)
	}

	// 1. Run Scan
	anomalies, err := pe.Scan(ctx)
	if err != nil {
		t.Fatalf("proactive Scan failed: %v", err)
	}

	// Should detect at least 5 anomaly kinds: disk, mcp, task_stalled, token_budget, inbound_queue
	if len(anomalies) < 5 {
		t.Fatalf("expected at least 5 detected anomalies, got %d", len(anomalies))
	}

	// 2. Query anomalies via ListAnomalies
	activeList, err := pe.ListAnomalies(ctx, "active", "", 50)
	if err != nil {
		t.Fatalf("ListAnomalies failed: %v", err)
	}
	if len(activeList) != len(anomalies) {
		t.Fatalf("expected %d active anomalies, got %d", len(anomalies), len(activeList))
	}

	// 3. Act on an anomaly (auto_task action)
	targetAnomaly := anomalies[0]
	createdTask, err := pe.ActOnAnomaly(ctx, targetAnomaly.ID, "auto_task")
	if err != nil {
		t.Fatalf("ActOnAnomaly auto_task failed: %v", err)
	}
	if createdTask == nil {
		t.Fatal("expected created autonomous task from auto_task action")
	}

	// Verify target anomaly is now marked resolved
	resolvedList, err := pe.ListAnomalies(ctx, "resolved", "", 50)
	if err != nil {
		t.Fatalf("listing resolved anomalies: %v", err)
	}
	foundResolved := false
	for _, a := range resolvedList {
		if a.ID == targetAnomaly.ID {
			foundResolved = true
			break
		}
	}
	if !foundResolved {
		t.Fatalf("expected anomaly %s to be in resolved list", targetAnomaly.ID)
	}

	// 4. Test Config Save/Load
	cfg := ProactiveConfig{
		Enabled:              true,
		ScanIntervalMinutes:  30,
		AutoCreateTasks:      true,
		DiskThresholdPercent: 75.0,
		GlobalKillSwitch:     false,
	}
	if err := pe.SaveConfig(ctx, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	loadedCfg, err := pe.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if loadedCfg.ScanIntervalMinutes != 30 || !loadedCfg.AutoCreateTasks || loadedCfg.DiskThresholdPercent != 75.0 {
		t.Fatalf("loaded config does not match saved config: %+v", loadedCfg)
	}
}

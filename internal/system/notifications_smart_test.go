package system

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	_ "modernc.org/sqlite"
)

func TestSmartNotificationsAndQuietHours(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "notif_test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	eventBus := bus.NewEventBus()
	nm, err := NewNotificationManager(db, eventBus)
	if err != nil {
		t.Fatalf("failed to init notification manager: %v", err)
	}
	defer nm.Stop()

	ctx := context.Background()

	// 1. Test Get & Save Preferences
	prefs, err := nm.GetPreferences(ctx)
	if err != nil {
		t.Fatalf("get preferences failed: %v", err)
	}
	if prefs.QuietHoursEnabled {
		t.Errorf("expected quiet hours default disabled, got true")
	}

	prefs.QuietHoursEnabled = true
	prefs.QuietHoursStart = "22:00"
	prefs.QuietHoursEnd = "07:00"
	prefs.DailyDigestEnabled = true
	prefs.MinPushSeverity = "warning"

	if err := nm.SavePreferences(ctx, prefs); err != nil {
		t.Fatalf("save preferences failed: %v", err)
	}

	saved, err := nm.GetPreferences(ctx)
	if err != nil {
		t.Fatalf("re-get preferences failed: %v", err)
	}
	if !saved.QuietHoursEnabled || saved.MinPushSeverity != "warning" {
		t.Errorf("saved preferences mismatch: %+v", saved)
	}

	// 2. Test InQuietHours Calculation
	// 23:30 should be in quiet hours (22:00 -> 07:00)
	t2330, _ := time.Parse(time.RFC3339, "2026-08-31T23:30:00Z")
	if !nm.InQuietHours(t2330, saved) {
		t.Errorf("expected 23:30 to be inside quiet hours (22:00-07:00)")
	}

	// 14:00 should NOT be in quiet hours
	t1400, _ := time.Parse(time.RFC3339, "2026-08-31T14:00:00Z")
	if nm.InQuietHours(t1400, saved) {
		t.Errorf("expected 14:00 to be outside quiet hours")
	}

	// 05:00 should be in quiet hours
	t0500, _ := time.Parse(time.RFC3339, "2026-08-31T05:00:00Z")
	if !nm.InQuietHours(t0500, saved) {
		t.Errorf("expected 05:00 to be inside quiet hours")
	}

	// 3. Test Daily Digest Generation
	_, _ = nm.Create(ctx, Notification{
		Title:    "Test alert 1",
		Message:  "Details 1",
		Type:     "info",
		Category: "system",
	})
	_, _ = nm.Create(ctx, Notification{
		Title:    "Test alert 2",
		Message:  "Details 2",
		Type:     "warning",
		Category: "task",
	})

	digest, err := nm.GenerateDailyDigest(ctx)
	if err != nil {
		t.Fatalf("generate daily digest failed: %v", err)
	}
	if digest == nil || digest.Title != "Daily Notification Digest" {
		t.Fatalf("unexpected digest notification: %+v", digest)
	}
}

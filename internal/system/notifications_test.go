package system

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/tools"
	_ "modernc.org/sqlite"
)

func TestNotificationManager_LifecycleAndPagination(t *testing.T) {
	tempDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	eventBus := bus.NewEventBus()
	mgr, err := NewNotificationManager(db, eventBus)
	if err != nil {
		t.Fatalf("failed to create notification manager: %v", err)
	}

	ctx := context.Background()

	// 1. Initial state
	unread, err := mgr.GetUnreadCount(ctx)
	if err != nil || unread != 0 {
		t.Fatalf("expected 0 unread, got %d (err=%v)", unread, err)
	}

	// 2. Create Notifications
	n1, err := mgr.Create(ctx, Notification{
		Title:    "Approval Required",
		Message:  "Agent requested native_exec",
		Type:     "approval",
		Category: "approval",
		Link:     "/missions",
	})
	if err != nil || n1.ID == "" {
		t.Fatalf("failed to create notification: %v", err)
	}

	n2, err := mgr.Create(ctx, Notification{
		Title:    "Task Error",
		Message:  "Failed to execute heartbeat task",
		Type:     "error",
		Category: "task",
		Link:     "/missions",
	})
	if err != nil || n2.ID == "" {
		t.Fatalf("failed to create notification 2: %v", err)
	}

	// 3. Check unread count
	unread, err = mgr.GetUnreadCount(ctx)
	if err != nil || unread != 2 {
		t.Fatalf("expected 2 unread, got %d", unread)
	}

	// 4. Test List with pagination
	items, total, unreadCount, err := mgr.List(ctx, 1, 10, "", false)
	if err != nil || total != 2 || len(items) != 2 || unreadCount != 2 {
		t.Fatalf("unexpected list result: items=%d, total=%d, unread=%d", len(items), total, unreadCount)
	}

	// 5. Test Filter Type
	approvals, totalAppr, _, err := mgr.List(ctx, 1, 10, "approval", false)
	if err != nil || totalAppr != 1 || len(approvals) != 1 {
		t.Fatalf("unexpected approval filter result: items=%d, total=%d", len(approvals), totalAppr)
	}

	// 6. Mark single read
	if err := mgr.MarkAsRead(ctx, n1.ID); err != nil {
		t.Fatalf("failed to mark read: %v", err)
	}
	unread, _ = mgr.GetUnreadCount(ctx)
	if unread != 1 {
		t.Fatalf("expected 1 unread after mark single read, got %d", unread)
	}

	// 7. Mark all read
	if err := mgr.MarkAllAsRead(ctx); err != nil {
		t.Fatalf("failed to mark all read: %v", err)
	}
	unread, _ = mgr.GetUnreadCount(ctx)
	if unread != 0 {
		t.Fatalf("expected 0 unread after mark all read, got %d", unread)
	}

	// 8. Test approval helper
	mgr.NotifyApprovalRequired(ctx, tools.ApprovalRequest{
		ID:       "appr_123",
		AgentID:  "agent_system_core",
		ToolName: "native_file_write",
	})
	unread, _ = mgr.GetUnreadCount(ctx)
	if unread != 1 {
		t.Fatalf("expected 1 unread after notify approval required, got %d", unread)
	}

	// 9. Delete single and clear all
	_ = mgr.Delete(ctx, n2.ID)
	_ = mgr.ClearAll(ctx)
	items, total, _, _ = mgr.List(ctx, 1, 10, "", false)
	if total != 0 || len(items) != 0 {
		t.Fatalf("expected 0 items after clear all, got %d", total)
	}
}

func TestNotificationManager_EventBusListener(t *testing.T) {
	tempDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	eventBus := bus.NewEventBus()
	mgr, err := NewNotificationManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartBackgroundListener(ctx)
	defer mgr.Stop()

	// Publish an approval:required event
	eventBus.Publish(bus.NewEvent("approval:required", "test-agent", map[string]any{
		"approval": tools.ApprovalRequest{
			ID:       "appr_test",
			AgentID:  "test-agent",
			ToolName: "native_exec",
		},
	}))

	time.Sleep(50 * time.Millisecond)

	unread, _ := mgr.GetUnreadCount(ctx)
	if unread < 1 {
		t.Fatalf("expected at least 1 notification generated from event, got %d", unread)
	}
}

package system

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/tools"
)

// Notification represents a user-facing system, agent, or operational alert.
type Notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Type      string    `json:"type"`     // "approval", "error", "warning", "info", "success"
	Category  string    `json:"category"` // "task", "agent", "system", "approval", "security"
	Link      string    `json:"link"`     // e.g. "/missions", "/audit-logs", "/settings"
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationManager handles persistence in SQLite, querying with pagination, and automatic bus dispatch.
type NotificationManager struct {
	mu     sync.RWMutex
	db     *sql.DB
	bus    *bus.EventBus
	stopCh chan struct{}
}

// NewNotificationManager creates and initializes a NotificationManager.
func NewNotificationManager(db *sql.DB, eventBus *bus.EventBus) (*NotificationManager, error) {
	nm := &NotificationManager{
		db:     db,
		bus:    eventBus,
		stopCh: make(chan struct{}),
	}
	if err := nm.initDB(); err != nil {
		return nil, err
	}
	return nm, nil
}

func (nm *NotificationManager) initDB() error {
	if nm.db == nil {
		return nil
	}

	query := `
	CREATE TABLE IF NOT EXISTS notifications (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		type TEXT DEFAULT 'info',
		category TEXT DEFAULT 'system',
		link TEXT DEFAULT '',
		is_read BOOLEAN DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_notifications_is_read ON notifications(is_read);
	`
	_, err := nm.db.Exec(query)
	return err
}

// Create inserts a new notification and broadcasts it on the event bus.
func (nm *NotificationManager) Create(ctx context.Context, notif Notification) (*Notification, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if notif.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		notif.ID = fmt.Sprintf("notif_%d_%s", time.Now().Unix(), hex.EncodeToString(b))
	}
	if notif.Type == "" {
		notif.Type = "info"
	}
	if notif.Category == "" {
		notif.Category = "system"
	}
	if notif.CreatedAt.IsZero() {
		notif.CreatedAt = time.Now().UTC()
	}

	if nm.db != nil {
		query := `
		INSERT INTO notifications (id, title, message, type, category, link, is_read, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`
		_, err := nm.db.ExecContext(ctx, query,
			notif.ID, notif.Title, notif.Message, notif.Type, notif.Category, notif.Link,
			notif.IsRead, notif.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting notification: %w", err)
		}
	}

	if nm.bus != nil {
		nm.bus.Publish(bus.NewEvent("notification:created", "system", map[string]any{
			"notification": notif,
		}))
	}

	return &notif, nil
}

// List returns a paginated list of notifications with filtering.
func (nm *NotificationManager) List(ctx context.Context, page, limit int, filterType string, unreadOnly bool) ([]Notification, int, int, error) {
	if nm.db == nil {
		return []Notification{}, 0, 0, nil
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var whereClauses []string
	var args []any

	if filterType != "" && filterType != "all" {
		whereClauses = append(whereClauses, "type = ?")
		args = append(args, filterType)
	}
	if unreadOnly {
		whereClauses = append(whereClauses, "is_read = 0")
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total matching
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notifications %s", whereSQL)
	var total int
	if err := nm.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, 0, fmt.Errorf("counting notifications: %w", err)
	}

	// Count unread overall
	var unreadCount int
	_ = nm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE is_read = 0").Scan(&unreadCount)

	// Fetch page
	fetchQuery := fmt.Sprintf(`
		SELECT id, title, message, type, category, link, is_read, created_at
		FROM notifications
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, whereSQL)

	queryArgs := append(args, limit, offset)
	rows, err := nm.db.QueryContext(ctx, fetchQuery, queryArgs...)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("listing notifications: %w", err)
	}
	defer rows.Close()

	var items []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Title, &n.Message, &n.Type, &n.Category, &n.Link, &n.IsRead, &n.CreatedAt); err == nil {
			items = append(items, n)
		}
	}
	if items == nil {
		items = []Notification{}
	}

	return items, total, unreadCount, nil
}

// GetUnreadCount returns the current count of unread notifications.
func (nm *NotificationManager) GetUnreadCount(ctx context.Context) (int, error) {
	if nm.db == nil {
		return 0, nil
	}
	var count int
	err := nm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE is_read = 0").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetLatest returns the most recent notification.
func (nm *NotificationManager) GetLatest(ctx context.Context) (*Notification, error) {
	if nm.db == nil {
		return nil, nil
	}
	var n Notification
	err := nm.db.QueryRowContext(ctx, `
		SELECT id, title, message, type, category, link, is_read, created_at
		FROM notifications
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&n.ID, &n.Title, &n.Message, &n.Type, &n.Category, &n.Link, &n.IsRead, &n.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &n, nil
}

// MarkAsRead marks a specific notification as read.
func (nm *NotificationManager) MarkAsRead(ctx context.Context, id string) error {
	if nm.db == nil {
		return nil
	}
	_, err := nm.db.ExecContext(ctx, "UPDATE notifications SET is_read = 1 WHERE id = ?", id)
	return err
}

// MarkAllAsRead marks all unread notifications as read.
func (nm *NotificationManager) MarkAllAsRead(ctx context.Context) error {
	if nm.db == nil {
		return nil
	}
	_, err := nm.db.ExecContext(ctx, "UPDATE notifications SET is_read = 1 WHERE is_read = 0")
	return err
}

// Delete removes a notification by ID.
func (nm *NotificationManager) Delete(ctx context.Context, id string) error {
	if nm.db == nil {
		return nil
	}
	_, err := nm.db.ExecContext(ctx, "DELETE FROM notifications WHERE id = ?", id)
	return err
}

// ClearAll deletes all notifications.
func (nm *NotificationManager) ClearAll(ctx context.Context) error {
	if nm.db == nil {
		return nil
	}
	_, err := nm.db.ExecContext(ctx, "DELETE FROM notifications")
	return err
}

// StartBackgroundListener subscribes to EventBus events and automatically creates notifications.
func (nm *NotificationManager) StartBackgroundListener(ctx context.Context) {
	if nm.bus == nil {
		return
	}

	approvalSub := nm.bus.Subscribe("approval:required")
	toolErrSub := nm.bus.Subscribe(bus.EventToolExecutionError)
	actionDoneSub := nm.bus.Subscribe(bus.EventAgentActionDone)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-nm.stopCh:
				return
			case ev, ok := <-approvalSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					if appMap, ok := payload["approval"].(tools.ApprovalRequest); ok {
						nm.NotifyApprovalRequired(context.Background(), appMap)
					}
				}
			case ev, ok := <-toolErrSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					toolName, _ := payload["tool_name"].(string)
					errMsg, _ := payload["error"].(string)
					_, _ = nm.Create(context.Background(), Notification{
						Title:    fmt.Sprintf("Tool Error: %s", toolName),
						Message:  fmt.Sprintf("Agent encountered an error executing '%s': %s", toolName, errMsg),
						Type:     "error",
						Category: "agent",
						Link:     "/missions",
					})
				}
			case ev, ok := <-actionDoneSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					pType, _ := payload["type"].(string)
					if pType == "proactive_cron_notification" {
						jobName, _ := payload["job_name"].(string)
						content, _ := payload["content"].(string)
						if jobName == "" {
							jobName = "Proactive Mission Alert"
						}
						_, _ = nm.Create(context.Background(), Notification{
							Title:    jobName,
							Message:  content,
							Type:     "info",
							Category: "task",
							Link:     "/automations",
						})
					}
				}
			}
		}
	}()
}

// NotifyApprovalRequired creates an approval notification immediately.
func (nm *NotificationManager) NotifyApprovalRequired(ctx context.Context, approval tools.ApprovalRequest) {
	_, _ = nm.Create(ctx, Notification{
		Title:    fmt.Sprintf("Approval Required: %s", approval.ToolName),
		Message:  fmt.Sprintf("Agent '%s' requested execution of high-risk tool '%s'. Review and decide in Missions.", approval.AgentID, approval.ToolName),
		Type:     "approval",
		Category: "approval",
		Link:     "/missions",
	})
}

// Stop terminates background event listeners.
func (nm *NotificationManager) Stop() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	select {
	case <-nm.stopCh:
	default:
		close(nm.stopCh)
	}
	slog.Info("notification manager stopped")
}

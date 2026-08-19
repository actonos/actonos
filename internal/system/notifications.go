package system

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
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

// PushSubscription represents a Web Push API subscription from a browser Service Worker.
type PushSubscription struct {
	Endpoint  string    `json:"endpoint"`
	P256dh    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	UserAgent string    `json:"user_agent,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// NotificationManager handles persistence in SQLite, querying with pagination, VAPID Web Push, and automatic bus dispatch.
type NotificationManager struct {
	mu     sync.RWMutex
	db     *sql.DB
	bus    *bus.EventBus
	stopCh chan struct{}

	vapidPublicKey  string
	vapidPrivateKey string

	alertMu    sync.Mutex
	lastAlerts map[string]time.Time // dedup key -> last time an integration-health notification was created
}

// NewNotificationManager creates and initializes a NotificationManager.
func NewNotificationManager(db *sql.DB, eventBus *bus.EventBus) (*NotificationManager, error) {
	nm := &NotificationManager{
		db:         db,
		bus:        eventBus,
		stopCh:     make(chan struct{}),
		lastAlerts: make(map[string]time.Time),
	}
	if err := nm.initDB(); err != nil {
		return nil, err
	}
	if err := nm.initVAPIDKeys(); err != nil {
		slog.Warn("failed to initialize VAPID keys for web push", "error", err)
	}
	return nm, nil
}

func (nm *NotificationManager) initDB() error {
	if nm.db == nil {
		return nil
	}

	_, _ = nm.db.Exec("PRAGMA busy_timeout = 5000;")
	_, _ = nm.db.Exec("PRAGMA journal_mode = WAL;")

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

	CREATE TABLE IF NOT EXISTS push_subscriptions (
		endpoint TEXT PRIMARY KEY,
		p256dh TEXT NOT NULL,
		auth TEXT NOT NULL,
		user_agent TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := nm.db.Exec(query)
	return err
}

func (nm *NotificationManager) initVAPIDKeys() error {
	if nm.db == nil {
		// In-memory fallback if no DB
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return err
		}
		nm.vapidPrivateKey = priv
		nm.vapidPublicKey = pub
		return nil
	}

	var pubKey, privKey string
	errPub := nm.db.QueryRow("SELECT value FROM system_settings WHERE key = 'vapid_public_key'").Scan(&pubKey)
	errPriv := nm.db.QueryRow("SELECT value FROM system_settings WHERE key = 'vapid_private_key'").Scan(&privKey)

	if errPub == nil && errPriv == nil && pubKey != "" && privKey != "" {
		nm.vapidPublicKey = pubKey
		nm.vapidPrivateKey = privKey
		return nil
	}

	// Generate new VAPID keypair
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("generating VAPID keys: %w", err)
	}

	ctx := context.Background()
	_, _ = nm.db.ExecContext(ctx, "INSERT OR REPLACE INTO system_settings (key, value, updated_at) VALUES ('vapid_public_key', ?, CURRENT_TIMESTAMP)", pub)
	_, _ = nm.db.ExecContext(ctx, "INSERT OR REPLACE INTO system_settings (key, value, updated_at) VALUES ('vapid_private_key', ?, CURRENT_TIMESTAMP)", priv)

	nm.vapidPublicKey = pub
	nm.vapidPrivateKey = priv
	slog.Info("initialized new VAPID keypair for Web Push notifications")
	return nil
}

// GetVAPIDPublicKey returns the public key required by browser PushManager.
func (nm *NotificationManager) GetVAPIDPublicKey() string {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.vapidPublicKey
}

// SubscribePush stores or updates a Web Push subscription.
func (nm *NotificationManager) SubscribePush(ctx context.Context, sub PushSubscription) error {
	if nm.db == nil {
		return nil
	}
	if sub.Endpoint == "" || sub.P256dh == "" || sub.Auth == "" {
		return fmt.Errorf("invalid push subscription: endpoint and keys are required")
	}

	query := `
	INSERT INTO push_subscriptions (endpoint, p256dh, auth, user_agent, created_at)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(endpoint) DO UPDATE SET
		p256dh = excluded.p256dh,
		auth = excluded.auth,
		user_agent = excluded.user_agent,
		created_at = CURRENT_TIMESTAMP
	`
	_, err := nm.db.ExecContext(ctx, query, sub.Endpoint, sub.P256dh, sub.Auth, sub.UserAgent)
	return err
}

// UnsubscribePush removes a Web Push subscription by its endpoint.
func (nm *NotificationManager) UnsubscribePush(ctx context.Context, endpoint string) error {
	if nm.db == nil || endpoint == "" {
		return nil
	}
	_, err := nm.db.ExecContext(ctx, "DELETE FROM push_subscriptions WHERE endpoint = ?", endpoint)
	return err
}

// ListPushSubscriptions returns all registered Web Push subscriptions.
func (nm *NotificationManager) ListPushSubscriptions(ctx context.Context) ([]PushSubscription, error) {
	if nm.db == nil {
		return []PushSubscription{}, nil
	}
	rows, err := nm.db.QueryContext(ctx, "SELECT endpoint, p256dh, auth, user_agent, created_at FROM push_subscriptions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []PushSubscription
	for rows.Next() {
		var s PushSubscription
		if err := rows.Scan(&s.Endpoint, &s.P256dh, &s.Auth, &s.UserAgent, &s.CreatedAt); err == nil {
			subs = append(subs, s)
		}
	}
	return subs, nil
}

// SendPushToAll broadcasts a notification to all registered Web Push subscriptions.
func (nm *NotificationManager) SendPushToAll(ctx context.Context, notif Notification) {
	nm.mu.RLock()
	pubKey := nm.vapidPublicKey
	privKey := nm.vapidPrivateKey
	nm.mu.RUnlock()

	if nm.db == nil || pubKey == "" || privKey == "" {
		return
	}

	subs, err := nm.ListPushSubscriptions(ctx)
	if err != nil || len(subs) == 0 {
		return
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return
	}

	for _, sub := range subs {
		go func(s PushSubscription) {
			pushSub := &webpush.Subscription{
				Endpoint: s.Endpoint,
				Keys: webpush.Keys{
					P256dh: s.P256dh,
					Auth:   s.Auth,
				},
			}
			resp, pushErr := webpush.SendNotification(payload, pushSub, &webpush.Options{
				Subscriber:      "mailto:admin@actonos.local",
				VAPIDPublicKey:  pubKey,
				VAPIDPrivateKey: privKey,
				TTL:             86400,
				Urgency:         webpush.UrgencyHigh,
			})
			if pushErr != nil {
				slog.Debug("failed to send web push notification", "endpoint", s.Endpoint, "error", pushErr)
			}
			if resp != nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
					slog.Info("removing expired push endpoint", "endpoint", s.Endpoint, "status", resp.StatusCode)
					_ = nm.UnsubscribePush(context.Background(), s.Endpoint)
				}
			}
		}(sub)
	}
}

// Create inserts a new notification and broadcasts it on the event bus and Web Push.
func (nm *NotificationManager) Create(ctx context.Context, notif Notification) (*Notification, error) {
	nm.mu.Lock()

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
			nm.mu.Unlock()
			return nil, fmt.Errorf("inserting notification: %w", err)
		}
	}

	if nm.bus != nil {
		nm.bus.Publish(bus.NewEvent("notification:created", "system", map[string]any{
			"notification": notif,
		}))
	}

	nm.mu.Unlock()

	// Dispatch Web Push to Service Workers in background goroutine
	go nm.SendPushToAll(context.Background(), notif)

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

// integrationAlertCooldown is how long we suppress a repeat notification for
// the same failing integration (channel account, connector provider, or MCP
// server) so a persistently-broken connection doesn't spam the web UI with a
// new notification every retry/poll cycle.
const integrationAlertCooldown = 15 * time.Minute

// shouldNotifyIntegration reports whether enough time has passed since the
// last notification for the given dedup key to justify creating a new one.
func (nm *NotificationManager) shouldNotifyIntegration(key string) bool {
	nm.alertMu.Lock()
	defer nm.alertMu.Unlock()
	if last, ok := nm.lastAlerts[key]; ok && time.Since(last) < integrationAlertCooldown {
		return false
	}
	nm.lastAlerts[key] = time.Now()
	return true
}

// clearIntegrationAlert resets the dedup cooldown for a key, used when an
// integration recovers so the next failure (if any) is reported immediately.
func (nm *NotificationManager) clearIntegrationAlert(key string) {
	nm.alertMu.Lock()
	defer nm.alertMu.Unlock()
	delete(nm.lastAlerts, key)
}

// StartBackgroundListener subscribes to EventBus events and automatically creates notifications.
func (nm *NotificationManager) StartBackgroundListener(ctx context.Context) {
	if nm.bus == nil {
		return
	}

	approvalSub := nm.bus.Subscribe("approval:required")
	toolErrSub := nm.bus.Subscribe(bus.EventToolExecutionError)
	actionDoneSub := nm.bus.Subscribe(bus.EventAgentActionDone)
	tokenExpiredSub := nm.bus.Subscribe(bus.EventTokenExpired)
	tokenFailedSub := nm.bus.Subscribe(bus.EventTokenFailed)
	channelErrSub := nm.bus.Subscribe(bus.EventChannelAdapterError)
	channelRecoveredSub := nm.bus.Subscribe(bus.EventChannelAdapterRecovered)
	mcpErrSub := nm.bus.Subscribe(bus.EventMCPServerError)
	mcpRecoveredSub := nm.bus.Subscribe(bus.EventMCPServerRecovered)

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
			case ev, ok := <-tokenExpiredSub:
				if !ok {
					return
				}
				provider := ev.AgentID
				reason := "no_refresh_token"
				if payload, ok := ev.Payload.(map[string]any); ok {
					if r, ok := payload["reason"].(string); ok && r != "" {
						reason = r
					}
				}
				if nm.shouldNotifyIntegration("connector:" + provider) {
					_, _ = nm.Create(context.Background(), Notification{
						Title:    fmt.Sprintf("Connector cần kết nối lại: %s", provider),
						Message:  fmt.Sprintf("Kết nối '%s' đã hết hạn refresh token (%s). Vui lòng vào Connectors để xác thực lại.", provider, reason),
						Type:     "warning",
						Category: "system",
						Link:     "/connectors",
					})
				}
			case ev, ok := <-tokenFailedSub:
				if !ok {
					return
				}
				provider := ev.AgentID
				errMsg, _ := ev.Payload.(string)
				if nm.shouldNotifyIntegration("connector:" + provider) {
					_, _ = nm.Create(context.Background(), Notification{
						Title:    fmt.Sprintf("Lỗi làm mới token: %s", provider),
						Message:  fmt.Sprintf("Không thể làm mới token cho '%s': %s", provider, errMsg),
						Type:     "error",
						Category: "system",
						Link:     "/connectors",
					})
				}
			case ev, ok := <-channelErrSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					channel, _ := payload["channel"].(string)
					name, _ := payload["name"].(string)
					errMsg, _ := payload["error"].(string)
					accountID, _ := payload["account_id"].(string)
					if name == "" {
						name = accountID
					}
					if nm.shouldNotifyIntegration("channel:" + accountID) {
						_, _ = nm.Create(context.Background(), Notification{
							Title:    fmt.Sprintf("Kênh chat lỗi: %s (%s)", name, channel),
							Message:  fmt.Sprintf("Không thể kết nối tài khoản '%s' trên %s: %s", name, channel, errMsg),
							Type:     "error",
							Category: "system",
							Link:     "/channels",
						})
					}
				}
			case ev, ok := <-channelRecoveredSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					channel, _ := payload["channel"].(string)
					name, _ := payload["name"].(string)
					accountID, _ := payload["account_id"].(string)
					if name == "" {
						name = accountID
					}
					nm.clearIntegrationAlert("channel:" + accountID)
					_, _ = nm.Create(context.Background(), Notification{
						Title:    fmt.Sprintf("Kênh chat đã kết nối lại: %s (%s)", name, channel),
						Message:  fmt.Sprintf("Tài khoản '%s' trên %s đã hoạt động trở lại.", name, channel),
						Type:     "success",
						Category: "system",
						Link:     "/channels",
					})
				}
			case ev, ok := <-mcpErrSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					serverID, _ := payload["server_id"].(string)
					name, _ := payload["name"].(string)
					errMsg, _ := payload["error"].(string)
					if name == "" {
						name = serverID
					}
					if nm.shouldNotifyIntegration("mcp:" + serverID) {
						_, _ = nm.Create(context.Background(), Notification{
							Title:    fmt.Sprintf("MCP server lỗi: %s", name),
							Message:  fmt.Sprintf("MCP server '%s' không kết nối được hoặc bị ngắt kết nối bất ngờ: %s", name, errMsg),
							Type:     "error",
							Category: "system",
							Link:     "/tools",
						})
					}
				}
			case ev, ok := <-mcpRecoveredSub:
				if !ok {
					return
				}
				if payload, ok := ev.Payload.(map[string]any); ok {
					serverID, _ := payload["server_id"].(string)
					name, _ := payload["name"].(string)
					if name == "" {
						name = serverID
					}
					nm.clearIntegrationAlert("mcp:" + serverID)
					_, _ = nm.Create(context.Background(), Notification{
						Title:    fmt.Sprintf("MCP server đã kết nối lại: %s", name),
						Message:  fmt.Sprintf("MCP server '%s' đã hoạt động trở lại.", name),
						Type:     "success",
						Category: "system",
						Link:     "/tools",
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
						link := "/automations"
						category := "task"
						if strings.HasPrefix(jobName, "Mission:") || strings.Contains(jobName, "Mission") || strings.Contains(jobName, "Heartbeat") {
							link = "/missions"
							category = "task"
						} else if strings.Contains(jobName, "Direct Agent") || strings.Contains(jobName, "Agent Notification") || strings.Contains(jobName, "Channel") {
							link = "/chat"
							category = "agent"
						} else if strings.HasPrefix(jobName, "Cron:") || strings.Contains(jobName, "Cron") || strings.Contains(jobName, "Scheduled Job") {
							link = "/automations"
							category = "task"
						}
						_, _ = nm.Create(context.Background(), Notification{
							Title:    jobName,
							Message:  content,
							Type:     "info",
							Category: category,
							Link:     link,
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

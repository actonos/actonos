package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type realtimeSnapshot struct {
	Type                string    `json:"type"`
	Timestamp           time.Time `json:"timestamp"`
	Metrics             any       `json:"metrics,omitempty"`
	Runs                any       `json:"runs,omitempty"`
	Approvals           any       `json:"approvals,omitempty"`
	Tokens              any       `json:"tokens,omitempty"`
	NotificationsUnread int       `json:"notifications_unread"`
	LatestNotification  any       `json:"latest_notification,omitempty"`
}

type realtimeHub struct {
	server    *Server
	mu        sync.RWMutex
	refreshMu sync.Mutex
	snapshot  realtimeSnapshot
	refreshed time.Time
}

func newRealtimeHub(server *Server) *realtimeHub {
	return &realtimeHub{server: server}
}

func (h *realtimeHub) get(ctx context.Context) realtimeSnapshot {
	h.mu.RLock()
	if time.Since(h.refreshed) < 1500*time.Millisecond {
		snapshot := h.snapshot
		h.mu.RUnlock()
		return snapshot
	}
	h.mu.RUnlock()

	h.refreshMu.Lock()
	defer h.refreshMu.Unlock()
	h.mu.RLock()
	if time.Since(h.refreshed) < 1500*time.Millisecond {
		snapshot := h.snapshot
		h.mu.RUnlock()
		return snapshot
	}
	h.mu.RUnlock()

	snapshot := h.server.collectRealtimeSnapshot(ctx)
	h.mu.Lock()
	h.snapshot = snapshot
	h.refreshed = time.Now()
	h.mu.Unlock()
	return snapshot
}

func (s *Server) handleRealtimeStream(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx := conn.CloseRead(r.Context())
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.writeRealtimeSnapshot(ctx, conn); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) writeRealtimeSnapshot(ctx context.Context, conn *websocket.Conn) error {
	snapshot := s.realtime.get(ctx)
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, payload)
}

func (s *Server) collectRealtimeSnapshot(ctx context.Context) realtimeSnapshot {
	snapshot := realtimeSnapshot{Type: "snapshot", Timestamp: time.Now().UTC()}
	if s.hal != nil {
		snapshot.Metrics, _ = s.hal.GetMetrics(ctx)
	}
	if s.runStore != nil {
		snapshot.Runs, _ = s.runStore.List(ctx, 30)
	}
	if s.approvalMgr != nil {
		snapshot.Approvals, _ = s.approvalMgr.List(ctx, "pending", 100)
	}
	if s.tokenTracker != nil {
		snapshot.Tokens, _ = s.tokenTracker.GetSummary(ctx)
	}
	if s.notifMgr != nil {
		snapshot.NotificationsUnread, _ = s.notifMgr.GetUnreadCount(ctx)
		snapshot.LatestNotification, _ = s.notifMgr.GetLatest(ctx)
	}
	return snapshot
}

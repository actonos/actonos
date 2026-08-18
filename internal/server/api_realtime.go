package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

type realtimeSnapshot struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Metrics   any       `json:"metrics,omitempty"`
	Runs      any       `json:"runs,omitempty"`
	Approvals any       `json:"approvals,omitempty"`
	Tokens    any       `json:"tokens,omitempty"`
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
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, payload)
}

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRealtimeStreamEmitsSnapshot(t *testing.T) {
	srv := newTestServer(t)
	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/realtime"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dialing realtime websocket: %v", err)
	}
	defer conn.CloseNow()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading realtime snapshot: %v", err)
	}
	var snapshot realtimeSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatalf("decoding realtime snapshot: %v", err)
	}
	if snapshot.Type != "snapshot" || snapshot.Metrics == nil || snapshot.Runs == nil || snapshot.Approvals == nil {
		t.Fatalf("unexpected realtime snapshot: %+v", snapshot)
	}
}

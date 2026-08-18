package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/system"
	"github.com/coder/websocket"
)

type countingHAL struct {
	mu    sync.Mutex
	calls int
}

func (h *countingHAL) RuntimeMode() string { return "test" }
func (h *countingHAL) GetMetrics(context.Context) (*system.SystemMetrics, error) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	return &system.SystemMetrics{}, nil
}
func (h *countingHAL) ScanWifi(context.Context) ([]system.WifiNetwork, error) {
	return nil, nil
}
func (h *countingHAL) ConnectWifi(context.Context, string, string) error { return nil }
func (h *countingHAL) GetWifiStatus(context.Context) (*system.WifiStatus, error) {
	return &system.WifiStatus{}, nil
}
func (h *countingHAL) RestartDaemon(context.Context) error { return nil }
func (h *countingHAL) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

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

func TestRealtimeHubCachesConcurrentSnapshots(t *testing.T) {
	srv := newTestServer(t)
	hal := &countingHAL{}
	srv.hal = hal
	srv.realtime = newRealtimeHub(srv)

	const clients = 12
	var wg sync.WaitGroup
	wg.Add(clients)
	for range clients {
		go func() {
			defer wg.Done()
			snapshot := srv.realtime.get(context.Background())
			if snapshot.Metrics == nil {
				t.Error("expected metrics in realtime snapshot")
			}
		}()
	}
	wg.Wait()

	if got := hal.callCount(); got != 1 {
		t.Fatalf("GetMetrics called %d times, want 1", got)
	}
}

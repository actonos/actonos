package system

import (
	"context"
	"sync"
	"testing"
)

type recordingNotifier struct {
	mu    sync.Mutex
	items []Notification
}

func (n *recordingNotifier) Create(_ context.Context, notif Notification) (*Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.items = append(n.items, notif)
	cp := notif
	return &cp, nil
}

func TestMaybeNotifyOncePerLatestVersion(t *testing.T) {
	eng := NewOTAEngine(t.TempDir())
	n := &recordingNotifier{}
	res := &CheckResult{UpdateAvailable: true, LatestVersion: "1.0.1", CurrentVersion: "1.0.0"}
	if err := eng.MaybeNotify(context.Background(), res, n); err != nil {
		t.Fatal(err)
	}
	if err := eng.MaybeNotify(context.Background(), res, n); err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.items) != 1 {
		t.Fatalf("created %d notifications, want 1", len(n.items))
	}
	if n.items[0].Link != "#/settings?view=maintenance" {
		t.Fatalf("link = %q", n.items[0].Link)
	}
}

func TestMaybeNotifySkipsRateLimit(t *testing.T) {
	eng := NewOTAEngine(t.TempDir())
	n := &recordingNotifier{}
	res := &CheckResult{UpdateAvailable: false, ErrorCode: ErrCodeRateLimit, LatestVersion: "1.0.1"}
	if err := eng.MaybeNotify(context.Background(), res, n); err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.items) != 0 {
		t.Fatalf("429 tick created %d notifications", len(n.items))
	}
}

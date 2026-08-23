package channels

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

type mockAdapter struct {
	name      string
	started   bool
	stopped   bool
	sentMsgs  []OutboundMessage
}

func (m *mockAdapter) Name() string { return m.name }
func (m *mockAdapter) Start(ctx context.Context) error {
	m.started = true
	return nil
}
func (m *mockAdapter) Stop() error {
	m.stopped = true
	return nil
}
func (m *mockAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func TestChannelManager_DynamicAdapterLifecycle(t *testing.T) {
	eventBus := bus.NewEventBus()
	defer eventBus.Close()

	mgr := NewChannelManager(eventBus, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("failed to start ChannelManager: %v", err)
	}
	defer mgr.Stop()

	// Register dynamic adapter
	mockTelegram := &mockAdapter{name: "telegram"}
	if err := mgr.RegisterAdapter(mockTelegram); err != nil {
		t.Fatalf("failed to register adapter: %v", err)
	}

	if !mockTelegram.started {
		t.Error("expected adapter to be started upon registration")
	}

	// Retrieve adapter
	adapter, exists := mgr.GetAdapter("telegram")
	if !exists || adapter.Name() != "telegram" {
		t.Errorf("expected to find telegram adapter, got %v", adapter)
	}

	// Send message
	msg := OutboundMessage{
		ChannelID: "telegram",
		Recipient: "123456",
		Content:   "Hello from ActonOS",
	}
	if err := mgr.SendMessage(ctx, msg); err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	if len(mockTelegram.sentMsgs) != 1 || mockTelegram.sentMsgs[0].Content != "Hello from ActonOS" {
		t.Errorf("expected message to be received by mock adapter, got %v", mockTelegram.sentMsgs)
	}

	// Test accounts sync and binding
	accounts := []ChannelAccount{
		{
			ID:            "tg_main",
			Name:          "Main Bot",
			Channel:       "telegram",
			Enabled:       true,
			BoundAgentIDs: []string{"agent_support"},
		},
	}
	if err := mgr.SyncAccounts(ctx, accounts); err != nil {
		t.Fatalf("failed to sync accounts: %v", err)
	}

	if bound := mgr.FindBoundAgent("telegram", "tg_main"); bound != "agent_support" {
		t.Errorf("expected bound agent 'agent_support', got '%s'", bound)
	}

	// Unregister adapter
	if err := mgr.UnregisterAdapter("telegram"); err != nil {
		t.Fatalf("failed to unregister adapter: %v", err)
	}
	if !mockTelegram.stopped {
		t.Error("expected adapter to be stopped upon unregistration")
	}
}

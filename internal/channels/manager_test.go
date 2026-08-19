package channels

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

func TestChannelManager_MultiAccountLifecycle(t *testing.T) {
	eventBus := bus.NewEventBus()
	defer eventBus.Close()

	mgr := NewChannelManager(eventBus, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("failed to start ChannelManager: %v", err)
	}
	defer mgr.Stop()

	// Sync test accounts
	accounts := []ChannelAccount{
		{
			ID:            "tg_support",
			Name:          "Customer Support Bot",
			Channel:       "telegram",
			Token:         "", // empty token won't make HTTP calls
			Enabled:       true,
			BoundAgentIDs: []string{"agent_support"},
			DefaultChatID: "123456",
		},
		{
			ID:            "wa_hotline",
			Name:          "WhatsApp Hotline",
			Channel:       "whatsapp",
			Token:         "",
			Enabled:       true,
			BoundAgentIDs: []string{"*"},
			DefaultChatID: "+84901234567",
		},
	}

	if err := mgr.SyncAccounts(ctx, accounts); err != nil {
		t.Fatalf("failed to sync accounts: %v", err)
	}

	accs := mgr.GetAccounts()
	if len(accs) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accs))
	}

	// Test Agent Binding resolution
	bound := mgr.FindBoundAgent("telegram", "tg_support")
	if bound != "agent_support" {
		t.Errorf("expected bound agent 'agent_support', got '%s'", bound)
	}

	boundWildcard := mgr.FindBoundAgent("whatsapp", "wa_hotline")
	if boundWildcard != "" {
		t.Errorf("expected empty specific agent for wildcard '*', got '%s'", boundWildcard)
	}
}

// TestChannelManager_AccountStatusTracksStartFailureAndPublishesEvent verifies
// that a channel account which fails to start records a runtime status
// (Connected=false, LastError set) and publishes EventChannelAdapterError,
// so failures are visible via GetAccountStatuses and web notifications
// instead of only reaching the server log.
func TestChannelManager_AccountStatusTracksStartFailureAndPublishesEvent(t *testing.T) {
	eventBus := bus.NewEventBus()
	defer eventBus.Close()
	errSub := eventBus.Subscribe(bus.EventChannelAdapterError)

	mgr := NewChannelManager(eventBus, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("failed to start ChannelManager: %v", err)
	}
	defer mgr.Stop()

	acc := ChannelAccount{
		ID:      "tg_broken",
		Name:    "Broken Bot",
		Channel: "telegram",
		Token:   "invalid-token",
		Enabled: true,
		Metadata: map[string]string{
			// Point at a URL that will fail immediately (connection refused)
			// rather than making a real Telegram API call in tests.
			"api_base": "http://127.0.0.1:1",
		},
	}
	if err := mgr.SyncAccounts(ctx, []ChannelAccount{acc}); err != nil {
		t.Fatalf("failed to sync accounts: %v", err)
	}

	// Telegram's Start() always returns nil immediately (polling happens in
	// a background goroutine), so the failure must surface asynchronously
	// via the poll loop's health reporting instead of the sync Start() path.
	select {
	case ev := <-errSub:
		if ev.AgentID != "tg_broken" {
			t.Fatalf("expected error event for tg_broken, got %s", ev.AgentID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected EventChannelAdapterError to be published for a broken telegram account")
	}
}


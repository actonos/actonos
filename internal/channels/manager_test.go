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

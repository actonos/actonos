package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actonos/actonos/internal/bus"
)

func TestDiscord_And_WebhookAdapter(t *testing.T) {
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	bus := bus.NewEventBus()
	defer bus.Close()

	discord := NewDiscordAdapter(server.URL, bus)
	_ = discord.Start(context.Background())

	msg := OutboundMessage{
		ChannelID: "discord",
		Content:   "Test Alert from ActonOS",
	}

	if err := discord.SendMessage(context.Background(), msg); err != nil {
		t.Fatalf("discord send failed: %v", err)
	}

	if !received {
		t.Fatal("expected test server to receive message")
	}

	// Test webhook
	webhook := NewWebhookAdapter(server.URL, bus)
	if err := webhook.SendMessage(context.Background(), msg); err != nil {
		t.Fatalf("webhook send failed: %v", err)
	}
}

package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actonos/actonos/internal/bus"
)

func TestPairingManager(t *testing.T) {
	pm, err := NewPairingManager(nil)
	if err != nil {
		t.Fatalf("failed to create pairing manager: %v", err)
	}

	code, err := pm.GeneratePairingCode("telegram")
	if err != nil {
		t.Fatalf("failed to generate code: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got %s", code)
	}

	// Unpaired user check
	if pm.IsAuthorized("telegram", "12345") {
		t.Fatal("user 12345 should not be authorized before pairing")
	}

	// Validate wrong code
	matched, err := pm.ValidateAndPair("telegram", "000000", "12345", "testuser")
	if err != nil || matched {
		t.Fatalf("wrong code should not pair: matched=%v, err=%v", matched, err)
	}

	// Validate correct code
	matched, err = pm.ValidateAndPair("telegram", code, "12345", "testuser")
	if err != nil || !matched {
		t.Fatalf("correct code should pair: matched=%v, err=%v", matched, err)
	}

	// Check authorization
	if !pm.IsAuthorized("telegram", "12345") {
		t.Fatal("user 12345 should be authorized after pairing")
	}

	// Check listing
	users := pm.ListAuthorized("telegram")
	if len(users) != 1 || users[0].SenderID != "12345" {
		t.Fatalf("expected 1 user in listing, got %v", users)
	}

	// Revoke user
	if err := pm.RevokeUser("telegram", "12345"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if pm.IsAuthorized("telegram", "12345") {
		t.Fatal("user 12345 should not be authorized after revocation")
	}
}

func TestWhatsAppWebhookVerification(t *testing.T) {
	eb := bus.NewEventBus()
	defer eb.Close()

	pm, _ := NewPairingManager(nil)
	wa := NewWhatsAppAdapter("fake_token", "123456", "my_secret_token", eb, pm)

	challenge, ok := wa.VerifyWebhook("subscribe", "my_secret_token", "test_challenge_123")
	if !ok || challenge != "test_challenge_123" {
		t.Fatalf("webhook verification failed: ok=%v, challenge=%s", ok, challenge)
	}

	_, ok = wa.VerifyWebhook("subscribe", "wrong_token", "test_challenge_123")
	if ok {
		t.Fatal("webhook verification should fail with wrong token")
	}
}

func TestDiscord_And_WebhookAdapter(t *testing.T) {
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	eb := bus.NewEventBus()
	defer eb.Close()

	discord := NewDiscordAdapter(server.URL, eb)
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
	webhook := NewWebhookAdapter(server.URL, eb)
	if err := webhook.SendMessage(context.Background(), msg); err != nil {
		t.Fatalf("webhook send failed: %v", err)
	}
}

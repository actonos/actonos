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

func TestPairingPolicyAndOperatorAllow(t *testing.T) {
	pm, err := NewPairingManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	if pm.ChannelRequiresPairing("discord") {
		t.Fatal("pairing should be off until enabled")
	}
	if err := pm.SetChannelRequiresPairing("discord", true); err != nil {
		t.Fatal(err)
	}
	if !pm.ChannelRequiresPairing("discord") {
		t.Fatal("expected pairing required after policy save")
	}
	code, err := pm.GeneratePairingCode("discord")
	if err != nil {
		t.Fatal(err)
	}
	codes := pm.ListActiveCodes()
	if len(codes) != 1 || codes[0].Code != code {
		t.Fatalf("expected active code %s, got %+v", code, codes)
	}
	pm.NoteUnpaired("discord", "u-9", "Ada", "hello")
	if len(pm.ListPending()) != 1 {
		t.Fatalf("expected one pending sender")
	}
	if err := pm.AuthorizeSender("discord", "u-9", "Ada"); err != nil {
		t.Fatal(err)
	}
	if !pm.IsAuthorized("discord", "u-9") {
		t.Fatal("operator allow must authorize sender")
	}
	if len(pm.ListPending()) != 0 {
		t.Fatal("pending sender should clear after allow")
	}
}

func TestWebhookAdapter(t *testing.T) {
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	eb := bus.NewEventBus()
	defer eb.Close()

	msg := OutboundMessage{
		ChannelID: "webhook",
		Content:   "Test Alert from ActonOS",
	}

	// Test webhook
	webhook := NewWebhookAdapter(server.URL, eb)
	if err := webhook.SendMessage(context.Background(), msg); err != nil {
		t.Fatalf("webhook send failed: %v", err)
	}
	if !received {
		t.Fatal("expected test server to receive message")
	}
}

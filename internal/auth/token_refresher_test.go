package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/memory"
)

func TestTokenRefreshDaemon_ScanAndRefresh(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "auth_test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	vault, err := memory.NewVault(db, "test-vault-secret", nil)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	eventBus := bus.NewEventBus()
	defer eventBus.Close()

	// Mock Token Server
	refreshCalls := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			refreshCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access-token-999",
				"refresh_token": "new-refresh-token-888",
				"token_type":    "Bearer",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer mockServer.Close()

	stateStore := NewStateStore(10 * time.Minute)
	oauthEngine := NewOAuthEngine(stateStore)

	daemon := NewTokenRefreshDaemon(oauthEngine, vault, db, eventBus)
	daemon.SetRefreshBuffer(10 * time.Minute) // Buffer of 10 min

	providerID := "google"
	daemon.RegisterProviderConfig(OAuthProviderConfig{
		ProviderID: providerID,
		TokenURL:   mockServer.URL,
		ClientID:   "test-client-id",
	})

	ctx := context.Background()

	// 1. Store a token expiring in 2 minutes (which is <= 10 minute buffer)
	expiringToken := &TokenResponse{
		AccessToken:  "old-access-token-111",
		RefreshToken: "old-refresh-token-000",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().UTC().Add(2 * time.Minute),
	}

	if err := daemon.SaveToken(ctx, providerID, expiringToken); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Subscribe to refresh event
	refreshEventCh := eventBus.Subscribe(bus.EventTokenRefreshed)

	// 2. Trigger check and refresh
	refreshed := daemon.CheckAndRefreshAll(ctx)
	if refreshed != 1 {
		t.Fatalf("expected 1 token refreshed, got %d", refreshed)
	}

	if refreshCalls != 1 {
		t.Fatalf("expected 1 call to mock refresh server, got %d", refreshCalls)
	}

	// Verify event received
	select {
	case ev := <-refreshEventCh:
		if ev.AgentID != providerID {
			t.Fatalf("expected provider '%s' in event, got '%s'", providerID, ev.AgentID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for token refreshed event")
	}

	// 3. Verify updated token in database/vault
	stored, err := daemon.GetToken(ctx, providerID)
	if err != nil {
		t.Fatalf("failed to retrieve refreshed token: %v", err)
	}

	if stored.AccessToken != "new-access-token-999" {
		t.Fatalf("expected updated access token 'new-access-token-999', got '%s'", stored.AccessToken)
	}
	if stored.RefreshToken != "new-refresh-token-888" {
		t.Fatalf("expected updated refresh token 'new-refresh-token-888', got '%s'", stored.RefreshToken)
	}
}

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/memory"
)

func newRefreshDaemon(t *testing.T, tokenURL string) *TokenRefreshDaemon {
	t.Helper()
	db, err := memory.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vault, err := memory.NewVault(db, "test-vault-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	eb := bus.NewEventBus()
	t.Cleanup(eb.Close)
	d := NewTokenRefreshDaemon(NewOAuthEngine(NewStateStore(time.Minute)), vault, db, eb)
	d.SetRefreshBuffer(5 * time.Minute)
	d.RegisterProviderConfig(OAuthProviderConfig{ProviderID: "mailcal:a1", TokenURL: tokenURL, ClientID: "cid"})
	return d
}

func TestRefreshNowRefreshesOnceForConcurrentCallers(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "refresh_token": "rot", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer srv.Close()
	d := newRefreshDaemon(t, srv.URL)
	ctx := context.Background()
	if err := d.SaveToken(ctx, "mailcal:a1", &TokenResponse{AccessToken: "old", RefreshToken: "r0", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := d.RefreshNow(ctx, "mailcal:a1")
			if err != nil || got.AccessToken != "fresh" {
				t.Errorf("got %+v, %v", got, err)
			}
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("the refresh endpoint was called %d times, want exactly 1", n)
	}
}

func TestRefreshNowSkipsAFreshToken(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer srv.Close()
	d := newRefreshDaemon(t, srv.URL)
	ctx := context.Background()
	_ = d.SaveToken(ctx, "mailcal:a1", &TokenResponse{AccessToken: "ok", RefreshToken: "r", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Hour)})
	got, err := d.RefreshNow(ctx, "mailcal:a1")
	if err != nil || got.AccessToken != "ok" || calls.Load() != 0 {
		t.Fatalf("got %+v err=%v calls=%d", got, err, calls.Load())
	}
}

func TestRefreshNowReportsARevokedGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	d := newRefreshDaemon(t, srv.URL)
	ctx := context.Background()
	_ = d.SaveToken(ctx, "mailcal:a1", &TokenResponse{AccessToken: "old", RefreshToken: "r", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Minute)})
	if _, err := d.RefreshNow(ctx, "mailcal:a1"); !errors.Is(err, ErrTokenRefresh) {
		t.Fatalf("want ErrTokenRefresh, got %v", err)
	}
	if _, err := d.RefreshNow(ctx, "mailcal:unknown"); err == nil {
		t.Fatal("an unknown provider must fail")
	}
}

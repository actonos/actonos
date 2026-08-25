package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

func TestDCRManager_RegisterAndGet(t *testing.T) {
	mgr := NewDCRManager()
	_, err := mgr.RegisterClient(context.Background(), ClientMetadata{ClientName: "x"})
	if err == nil {
		t.Fatal("expected redirect_uris required")
	}
	got, err := mgr.RegisterClient(context.Background(), ClientMetadata{
		ClientName:   "acton-cli",
		RedirectURIs: []string{"http://127.0.0.1/callback"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID == "" || got.ClientSecret == "" {
		t.Fatal("expected generated credentials")
	}
	loaded, ok := mgr.GetClient(got.ClientID)
	if !ok || loaded.ClientName != "acton-cli" {
		t.Fatalf("get client: %+v ok=%v", loaded, ok)
	}
}

func TestOAuthEngine_ExchangeAndRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grant := r.Form.Get("grant_type")
		resp := TokenResponse{AccessToken: "access-" + grant, RefreshToken: "refresh", TokenType: "Bearer", ExpiresIn: 60}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	engine := NewOAuthEngine(NewStateStore(time.Minute))
	cfg := OAuthProviderConfig{ProviderID: "test", ClientID: "id", ClientSecret: "secret", TokenURL: server.URL, AuthURL: server.URL}
	tok, err := engine.ExchangeCode(context.Background(), cfg, "code", "http://localhost/cb", "verifier")
	if err != nil || tok.AccessToken != "access-authorization_code" {
		t.Fatalf("exchange: %+v err=%v", tok, err)
	}
	refreshed, err := engine.RefreshToken(context.Background(), cfg, "refresh")
	if err != nil || refreshed.AccessToken != "access-refresh_token" {
		t.Fatalf("refresh: %+v err=%v", refreshed, err)
	}
}

func TestTokenRefreshDaemon_StartStop(t *testing.T) {
	eb := bus.NewEventBus()
	defer eb.Close()
	d := NewTokenRefreshDaemon(nil, nil, nil, eb)
	d.SetCheckInterval(50 * time.Millisecond)
	d.SetRefreshBuffer(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	d.Stop()
}

func TestOAuthEngine_ExchangeCodeHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer server.Close()
	engine := NewOAuthEngine(NewStateStore(time.Minute))
	_, err := engine.ExchangeCode(context.Background(), OAuthProviderConfig{TokenURL: server.URL, ClientID: "id"}, "c", "http://localhost/cb", "v")
	if err == nil {
		t.Fatal("expected exchange error")
	}
}

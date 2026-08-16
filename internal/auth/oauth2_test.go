package auth

import (
	"net/url"
	"testing"
	"time"
)

func TestPKCE_GenerationAndVerification(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("failed to generate code verifier: %v", err)
	}

	if len(verifier) < 43 || len(verifier) > 128 {
		t.Fatalf("code verifier length out of RFC 7636 spec: %d", len(verifier))
	}

	challenge := GenerateCodeChallenge(verifier)
	if challenge == "" {
		t.Fatalf("expected non-empty code challenge")
	}

	// S256 verification should succeed
	if !VerifyPKCE(verifier, challenge) {
		t.Fatalf("PKCE verification failed for valid verifier")
	}

	// Tampered verifier must fail
	if VerifyPKCE(verifier+"tampered", challenge) {
		t.Fatalf("PKCE verification should fail for tampered verifier")
	}
}

func TestOAuthEngine_BuildAuthURL(t *testing.T) {
	stateStore := NewStateStore(10 * time.Minute)
	engine := NewOAuthEngine(stateStore)

	cfg := OAuthProviderConfig{
		ProviderID: "google",
		AuthURL:    "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:   "https://oauth2.googleapis.com/token",
		ClientID:   "test-client-id-12345",
		Scopes:     []string{"https://www.googleapis.com/auth/gmail.readonly"},
	}

	redirectURI := "http://localhost:8080/api/integrations/callback"
	authURL, state, verifier, err := engine.BuildAuthURL(cfg, redirectURI, nil)
	if err != nil {
		t.Fatalf("BuildAuthURL failed: %v", err)
	}

	if state == "" || verifier == "" {
		t.Fatalf("expected non-empty state and verifier")
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse generated auth URL: %v", err)
	}

	q := parsed.Query()
	if q.Get("response_type") != "code" {
		t.Fatalf("expected response_type=code, got %s", q.Get("response_type"))
	}
	if q.Get("client_id") != "test-client-id-12345" {
		t.Fatalf("expected correct client_id, got %s", q.Get("client_id"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected S256 challenge method, got %s", q.Get("code_challenge_method"))
	}
	if q.Get("state") != state {
		t.Fatalf("expected state matching returned state")
	}

	// State should be consumable once
	session, err := stateStore.Consume(state)
	if err != nil {
		t.Fatalf("expected state store consume to succeed: %v", err)
	}
	if session.CodeVerifier != verifier {
		t.Fatalf("expected verifier in session to match")
	}

	// Second consume must fail (CSRF replay prevention)
	_, err = stateStore.Consume(state)
	if err == nil {
		t.Fatalf("expected replay consume to fail, got nil")
	}
}

func TestStateStore_ExpiredState(t *testing.T) {
	// TTL of 1 millisecond
	store := NewStateStore(1 * time.Millisecond)
	state, _ := store.Save("google", "verifier123", "http://localhost")

	time.Sleep(5 * time.Millisecond)

	_, err := store.Consume(state)
	if err == nil {
		t.Fatalf("expected expired state to be rejected, got nil")
	}
}

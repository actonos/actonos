package mailcal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/plugin"
	"github.com/actonos/actonos/internal/security"
)

// OAuth flow errors.
var (
	ErrOAuthNotConfigured = errors.New("oauth client credentials are not configured")
	ErrOAuthState         = errors.New("the sign-in request is unknown, expired or was not started from this browser")
	ErrNoRefreshToken     = errors.New("the provider did not return a refresh token; remove the app's access in the provider's account settings and connect again")
)

// ClientSource supplies the user's own OAuth client (bring-your-own client), for example from the vault.
type ClientSource interface {
	Client(ctx context.Context, provider string) (clientID, clientSecret string, err error)
}

// TokenStore is the part of auth.TokenRefreshDaemon the flow needs.
type TokenStore interface {
	SaveToken(ctx context.Context, provider string, token *auth.TokenResponse) error
	DeleteToken(ctx context.Context, provider string) error
	RegisterProviderConfig(cfg auth.OAuthProviderConfig)
}

// OAuthFlow runs sign-in for provider plugins. The callback is public (the browser arrives from
// another site, so the SameSite=Strict session cookie is not sent); it is protected by a
// single-use state, PKCE, and a nonce cookie that only the browser which started the flow holds.
type OAuthFlow struct {
	svc     *Service
	engine  *auth.OAuthEngine
	states  *auth.StateStore
	tokens  TokenStore
	clients ClientSource

	mu       sync.Mutex
	nonces   map[string]nonceEntry // state -> sha256(nonce)
	checkURL func(ctx context.Context, rawURL string) error
}

type nonceEntry struct {
	hash    [32]byte
	expires time.Time
}

// NewOAuthFlow builds the flow. states must be the same store the engine was created with.
func NewOAuthFlow(svc *Service, engine *auth.OAuthEngine, states *auth.StateStore, tokens TokenStore, clients ClientSource) *OAuthFlow {
	return &OAuthFlow{svc: svc, engine: engine, states: states, tokens: tokens, clients: clients,
		nonces: map[string]nonceEntry{}, checkURL: security.ValidateOutboundURL}
}

func (f *OAuthFlow) config(ctx context.Context, acct Account) (auth.OAuthProviderConfig, plugin.PluginOAuth, error) {
	spec, ok := f.svc.OAuthSpec(acct.PluginID)
	if !ok {
		return auth.OAuthProviderConfig{}, spec, fmt.Errorf("plugin %s does not support sign-in", acct.PluginID)
	}
	for _, raw := range []string{spec.AuthURL, spec.TokenURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.User != nil {
			return auth.OAuthProviderConfig{}, spec, fmt.Errorf("the plugin's oauth endpoint %q must be a plain https URL", raw)
		}
	}
	// The core itself calls token_url, so a hostile manifest must not aim it at the LAN.
	if err := f.checkURL(ctx, spec.TokenURL); err != nil {
		return auth.OAuthProviderConfig{}, spec, fmt.Errorf("token_url refused: %w", err)
	}
	id, secret, err := f.clients.Client(ctx, spec.Provider)
	if err != nil || id == "" {
		return auth.OAuthProviderConfig{}, spec, ErrOAuthNotConfigured
	}
	return auth.OAuthProviderConfig{ProviderID: ProviderKey(acct.ID), AuthURL: spec.AuthURL, TokenURL: spec.TokenURL,
		ClientID: id, ClientSecret: secret, Scopes: spec.Scopes}, spec, nil
}

// Start returns the provider URL to open and the nonce to set as an HttpOnly, SameSite=Lax
// cookie on the response to the same request.
func (f *OAuthFlow) Start(ctx context.Context, accountID, redirectURI string) (authURL, nonce string, err error) {
	acct, err := f.svc.Accounts().Get(ctx, accountID)
	if err != nil {
		return "", "", err
	}
	if acct.Auth.Type != "oauth" {
		return "", "", fmt.Errorf("%w: account %s is not an OAuth account", ErrInvalidAccount, acct.ID)
	}
	cfg, _, err := f.config(ctx, acct)
	if err != nil {
		return "", "", err
	}
	if acct.Auth.Provider != cfg.ProviderID {
		acct.Auth.Provider = cfg.ProviderID
		if err := f.svc.Accounts().SetAuth(ctx, acct.ID, acct.Auth); err != nil {
			return "", "", err
		}
	}
	authURL, state, _, err := f.engine.BuildAuthURL(cfg, redirectURI, nil)
	if err != nil {
		return "", "", err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	nonce = hex.EncodeToString(raw)
	f.mu.Lock()
	f.sweepLocked()
	f.nonces[state] = nonceEntry{hash: sha256.Sum256([]byte(nonce)), expires: time.Now().Add(10 * time.Minute)}
	f.mu.Unlock()
	_ = f.svc.Accounts().SetStatus(ctx, acct.ID, StatusAuthExpired, "waiting for sign-in")
	return authURL, nonce, nil
}

func (f *OAuthFlow) sweepLocked() {
	now := time.Now()
	for k, v := range f.nonces {
		if now.After(v.expires) {
			delete(f.nonces, k)
		}
	}
}

// Complete finishes sign-in. The state is consumed first, so a failed attempt cannot be replayed.
func (f *OAuthFlow) Complete(ctx context.Context, state, code, nonce string) (string, error) {
	sess, err := f.states.Consume(state)
	if err != nil {
		return "", ErrOAuthState
	}
	f.mu.Lock()
	entry, ok := f.nonces[state]
	delete(f.nonces, state)
	f.mu.Unlock()
	given := sha256.Sum256([]byte(nonce))
	if !ok || time.Now().After(entry.expires) || subtle.ConstantTimeCompare(entry.hash[:], given[:]) != 1 {
		return "", ErrOAuthState
	}
	if !strings.HasPrefix(sess.Provider, "mailcal:") {
		return "", ErrOAuthState
	}
	accountID := strings.TrimPrefix(sess.Provider, "mailcal:")
	acct, err := f.svc.Accounts().Get(ctx, accountID)
	if err != nil {
		return "", err
	}
	cfg, _, err := f.config(ctx, acct)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(code) == "" {
		return "", ErrOAuthState
	}
	tok, err := f.engine.ExchangeCode(ctx, cfg, code, sess.RedirectURI, sess.CodeVerifier)
	if err != nil {
		return "", err
	}
	if tok.RefreshToken == "" {
		return "", ErrNoRefreshToken
	}
	f.tokens.RegisterProviderConfig(cfg)
	if err := f.tokens.SaveToken(ctx, cfg.ProviderID, tok); err != nil {
		return "", err
	}
	if err := f.svc.Accounts().SetStatus(ctx, acct.ID, StatusOK, ""); err != nil {
		return "", err
	}
	return acct.ID, nil
}

// Forget deletes the stored token of a removed account, so no credential outlives its account.
func (f *OAuthFlow) Forget(ctx context.Context, accountID string) error {
	return f.tokens.DeleteToken(ctx, ProviderKey(accountID))
}

// RegisterAll registers every OAuth account's provider config with the refresh daemon, so
// background renewal keeps working after a restart. Accounts whose client is missing are skipped.
func (f *OAuthFlow) RegisterAll(ctx context.Context) int {
	accounts, err := f.svc.Accounts().List(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, a := range accounts {
		if a.Auth.Type != "oauth" {
			continue
		}
		if cfg, _, err := f.config(ctx, a); err == nil {
			f.tokens.RegisterProviderConfig(cfg)
			n++
		}
	}
	return n
}

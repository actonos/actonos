package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tetratelabs/wazero/api"
)

// Return codes of acton_oauth.get_token. A positive value is the length of the JSON answer,
// which the guest then reads with acton_sys.read_response.
const (
	OAuthErrNoHost      int32 = -1 // no host context
	OAuthErrDenied      int32 = -2 // manifest does not allow it, or the account is not this plugin's
	OAuthErrAuthExpired int32 = -3 // the user must sign in again
	OAuthErrUnavailable int32 = -4 // no broker, unknown account or the provider could not be reached
)

// Errors a TokenBroker returns so the host can pick the right code.
var (
	ErrTokenForbidden   = errors.New("the account does not belong to this plugin")
	ErrTokenAuthExpired = errors.New("the account needs to sign in again")
)

// TokenGrant is a short-lived access token and the provider it belongs to.
type TokenGrant struct {
	Provider    string
	AccessToken string
	ExpiresAt   time.Time
}

// TokenBroker hands out access tokens. Implementations must verify that accountID belongs to
// pluginID: the plugin id comes from the host context, never from the guest.
type TokenBroker interface {
	AccessToken(ctx context.Context, pluginID, accountID string) (TokenGrant, error)
}

// SetTokenBroker sets the broker used by plugins loaded afterwards.
func (m *Manager) SetTokenBroker(b TokenBroker) {
	m.mu.Lock()
	m.tokens = b
	m.mu.Unlock()
}

// oauthGetToken implements acton_oauth.get_token(accountPtr, accountLen) -> int32.
func oauthGetToken(ctx context.Context, m api.Module, ptr, length uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.Gate == nil {
		return OAuthErrNoHost
	}
	if len(h.Manifest.Permissions.OAuth) == 0 {
		return OAuthErrDenied // do not even wake the broker for a plugin with no OAuth permission
	}
	if h.Tokens == nil {
		return OAuthErrUnavailable
	}
	raw, err := readBufferFromMemory(m.Memory(), ptr, length)
	if err != nil || len(raw) == 0 || len(raw) > 256 {
		return OAuthErrDenied
	}
	grant, err := h.Tokens.AccessToken(ctx, h.PluginID, string(raw))
	switch {
	case errors.Is(err, ErrTokenForbidden):
		h.Record("WARN", "oauth token request refused: account does not belong to this plugin")
		return OAuthErrDenied
	case errors.Is(err, ErrTokenAuthExpired):
		return OAuthErrAuthExpired
	case err != nil:
		h.Record("WARN", fmt.Sprintf("oauth token unavailable: %v", err))
		return OAuthErrUnavailable
	}
	if err := h.Gate.CheckOAuthAccess(grant.Provider); err != nil {
		h.Record("WARN", err.Error())
		return OAuthErrDenied
	}
	h.rememberSecret(grant.AccessToken) // never appears in plugin logs
	body, _ := json.Marshal(map[string]any{"access_token": grant.AccessToken, "expires_at": grant.ExpiresAt.UTC().Format(time.RFC3339)})
	h.mu.Lock()
	h.LastResponse = body
	h.mu.Unlock()
	return int32(len(body))
}

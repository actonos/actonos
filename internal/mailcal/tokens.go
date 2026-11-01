package mailcal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/plugin"
)

// ProviderKey is the key under which an account's OAuth token is stored. It is derived from
// the account id so two accounts of one provider never share a token.
func ProviderKey(accountID string) string { return "mailcal:" + accountID }

// TokenSource is the part of auth.TokenRefreshDaemon the broker needs.
type TokenSource interface {
	GetToken(ctx context.Context, provider string) (*auth.StoredTokenData, error)
	RefreshNow(ctx context.Context, provider string) (*auth.StoredTokenData, error)
}

// TokenBroker implements plugin.TokenBroker: it gives a provider plugin the access token of
// one of that plugin's own accounts, never the refresh token, never another plugin's account.
type TokenBroker struct {
	accounts *AccountStore
	src      TokenSource
	specs    func(pluginID string) (plugin.PluginOAuth, bool)
	// skew is how long before expiry a token is renewed instead of returned.
	skew time.Duration
}

var _ plugin.TokenBroker = (*TokenBroker)(nil)

// NewTokenBroker builds a broker.
func NewTokenBroker(accounts *AccountStore, src TokenSource, specs func(pluginID string) (plugin.PluginOAuth, bool)) *TokenBroker {
	return &TokenBroker{accounts: accounts, src: src, specs: specs, skew: 60 * time.Second}
}

// AccessToken implements plugin.TokenBroker.
func (b *TokenBroker) AccessToken(ctx context.Context, pluginID, accountID string) (plugin.TokenGrant, error) {
	acct, err := b.accounts.Get(ctx, accountID)
	if err != nil {
		// An unknown account and another plugin's account look the same to the guest.
		return plugin.TokenGrant{}, plugin.ErrTokenForbidden
	}
	if acct.PluginID != pluginID {
		return plugin.TokenGrant{}, plugin.ErrTokenForbidden
	}
	if acct.Auth.Type != "oauth" {
		return plugin.TokenGrant{}, fmt.Errorf("account %s does not use OAuth", acct.ID)
	}
	spec, ok := b.specs(pluginID)
	if !ok {
		return plugin.TokenGrant{}, fmt.Errorf("plugin %s declares no oauth block", pluginID)
	}
	if acct.Status == StatusAuthExpired {
		return plugin.TokenGrant{}, plugin.ErrTokenAuthExpired
	}
	key := ProviderKey(acct.ID)
	tok, err := b.src.GetToken(ctx, key)
	if err != nil {
		_ = b.accounts.SetStatus(context.WithoutCancel(ctx), acct.ID, StatusAuthExpired, "not signed in")
		return plugin.TokenGrant{}, plugin.ErrTokenAuthExpired
	}
	if time.Until(tok.ExpiresAt) <= b.skew {
		tok, err = b.src.RefreshNow(ctx, key)
		if err != nil {
			if errors.Is(err, auth.ErrTokenRefresh) {
				_ = b.accounts.SetStatus(context.WithoutCancel(ctx), acct.ID, StatusAuthExpired, "the sign-in was revoked or expired")
				return plugin.TokenGrant{}, plugin.ErrTokenAuthExpired
			}
			return plugin.TokenGrant{}, err
		}
	}
	return plugin.TokenGrant{Provider: spec.Provider, AccessToken: tok.AccessToken, ExpiresAt: tok.ExpiresAt}, nil
}

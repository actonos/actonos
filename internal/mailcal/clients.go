package mailcal

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/actonos/actonos/internal/memory"
)

var providerName = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

// VaultClients keeps the user's own OAuth client id and secret in the encrypted vault.
type VaultClients struct{ Vault *memory.Vault }

var _ ClientSource = VaultClients{}

func clientKeys(provider string) (idKey, secretKey string, err error) {
	if !providerName.MatchString(provider) {
		return "", "", fmt.Errorf("invalid provider name %q", provider)
	}
	return "mailcal.oauth." + provider + ".client_id", "mailcal.oauth." + provider + ".client_secret", nil
}

// Client implements ClientSource.
func (v VaultClients) Client(ctx context.Context, provider string) (string, string, error) {
	idKey, secretKey, err := clientKeys(provider)
	if err != nil {
		return "", "", err
	}
	id, err := v.Vault.GetSecret(ctx, idKey)
	if err != nil {
		return "", "", ErrOAuthNotConfigured
	}
	secret, _ := v.Vault.GetSecret(ctx, secretKey) // public clients have no secret
	return id, secret, nil
}

// Set stores the client. The secret may be empty for PKCE-only public clients.
func (v VaultClients) Set(ctx context.Context, provider, clientID, clientSecret string) error {
	idKey, secretKey, err := clientKeys(provider)
	if err != nil {
		return err
	}
	if clientID == "" {
		return errors.New("client id is required")
	}
	if err := v.Vault.SetSecret(ctx, idKey, clientID); err != nil {
		return err
	}
	return v.Vault.SetSecret(ctx, secretKey, clientSecret)
}

// Configured reports whether a client id exists, without revealing it.
func (v VaultClients) Configured(ctx context.Context, provider string) bool {
	id, _, err := v.Client(ctx, provider)
	return err == nil && id != ""
}

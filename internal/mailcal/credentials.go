package mailcal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/actonos/actonos/internal/plugin"
)

// SecretStore is the part of memory.Vault the credential broker needs.
type SecretStore interface {
	GetSecret(ctx context.Context, keyName string) (string, error)
	SetSecret(ctx context.Context, keyName, secretValue string) error
	DeleteSecret(ctx context.Context, keyName string) error
}

// PasswordKey is the vault key of an account's password.
func PasswordKey(accountID string) string { return "mailcal.account." + accountID + ".password" }

// CredentialBroker implements plugin.CredentialBroker for password accounts (CalDAV, IMAP/SMTP)
// and owns the only code path that writes those passwords into the vault.
type CredentialBroker struct {
	accounts *AccountStore
	secrets  SecretStore
}

var _ plugin.CredentialBroker = (*CredentialBroker)(nil)

// NewCredentialBroker builds a broker.
func NewCredentialBroker(accounts *AccountStore, secrets SecretStore) *CredentialBroker {
	return &CredentialBroker{accounts: accounts, secrets: secrets}
}

// Credentials implements plugin.CredentialBroker. An unknown account and another plugin's
// account are indistinguishable to the caller.
func (b *CredentialBroker) Credentials(ctx context.Context, pluginID, accountID string) (plugin.Credentials, error) {
	acct, err := b.accounts.Get(ctx, accountID)
	if err != nil || acct.PluginID != pluginID {
		return plugin.Credentials{}, plugin.ErrCredentialsForbidden
	}
	key := acct.Auth.SecretRefs["password"]
	if acct.Auth.Type != "password" || key == "" {
		return plugin.Credentials{}, fmt.Errorf("account %s has no stored password", acct.ID)
	}
	if acct.Status == StatusAuthExpired {
		return plugin.Credentials{}, plugin.ErrCredentialsForbidden
	}
	password, err := b.secrets.GetSecret(ctx, key)
	if err != nil {
		return plugin.Credentials{}, fmt.Errorf("reading the stored password: %w", err)
	}
	user, _ := acct.Config["username"].(string)
	if strings.TrimSpace(user) == "" {
		user = acct.Email
	}
	if user == "" {
		return plugin.Credentials{}, errors.New("the account has no username")
	}
	return plugin.Credentials{Username: user, Password: password}, nil
}

// SetPassword stores the password in the vault and points the account at it. The account
// record only ever holds the key's name.
func (b *CredentialBroker) SetPassword(ctx context.Context, accountID, password string) error {
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("%w: the password is empty", ErrInvalidAccount)
	}
	acct, err := b.accounts.Get(ctx, accountID)
	if err != nil {
		return err
	}
	key := PasswordKey(acct.ID)
	if err := b.secrets.SetSecret(ctx, key, password); err != nil {
		return err
	}
	if err := b.accounts.SetAuth(ctx, acct.ID, AuthRef{Type: "password", SecretRefs: map[string]string{"password": key}}); err != nil {
		return err
	}
	return b.accounts.SetStatus(ctx, acct.ID, StatusOK, "")
}

// Forget deletes the stored password of a removed account.
func (b *CredentialBroker) Forget(ctx context.Context, accountID string) error {
	return b.secrets.DeleteSecret(ctx, PasswordKey(accountID))
}

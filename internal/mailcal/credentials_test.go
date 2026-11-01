package mailcal_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/plugin"
)

func credEnv(t *testing.T) (*mailcal.CredentialBroker, *mailcal.Service, mailcal.Account, *memory.Vault) {
	t.Helper()
	db, err := memory.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vault, err := memory.NewVault(db, "vault-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := mailcal.NewService(db.SQLDB())
	if err != nil {
		t.Fatal(err)
	}
	acct, err := svc.Accounts().Create(context.Background(), mailcal.NewAccount{Label: "Fastmail", PluginID: "caldav", Kind: mailcal.KindCalendar,
		Email: "me@example.com", Config: map[string]any{"username": "me@fastmail.example"}})
	if err != nil {
		t.Fatal(err)
	}
	return mailcal.NewCredentialBroker(svc.Accounts(), vault), svc, acct, vault
}

func TestPasswordsLiveInTheVaultAndNeverInTheAccountRecord(t *testing.T) {
	b, svc, acct, vault := credEnv(t)
	ctx := context.Background()
	if err := b.SetPassword(ctx, acct.ID, "app-password-123"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Accounts().Get(ctx, acct.ID)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "app-password-123") {
		t.Fatalf("the password leaked into the account record: %s", raw)
	}
	if got.Auth.Type != "password" || got.Auth.SecretRefs["password"] != mailcal.PasswordKey(acct.ID) {
		t.Fatalf("auth ref: %+v", got.Auth)
	}
	stored, err := vault.GetSecret(ctx, mailcal.PasswordKey(acct.ID))
	if err != nil || stored != "app-password-123" {
		t.Fatalf("vault: %q %v", stored, err)
	}
	creds, err := b.Credentials(ctx, "caldav", acct.ID)
	if err != nil || creds.Username != "me@fastmail.example" || creds.Password != "app-password-123" {
		t.Fatalf("%+v %v", creds, err)
	}
}

func TestCredentialsNeverGoToAnotherPlugin(t *testing.T) {
	b, _, acct, _ := credEnv(t)
	ctx := context.Background()
	_ = b.SetPassword(ctx, acct.ID, "pw")
	for _, tc := range []struct{ plugin, account string }{{"imap", acct.ID}, {"caldav", "acct_missing"}} {
		if _, err := b.Credentials(ctx, tc.plugin, tc.account); !errors.Is(err, plugin.ErrCredentialsForbidden) {
			t.Errorf("%v: want ErrCredentialsForbidden, got %v", tc, err)
		}
	}
}

func TestSetPasswordRejectsEmptyAndForgetDeletesTheSecret(t *testing.T) {
	b, _, acct, vault := credEnv(t)
	ctx := context.Background()
	if err := b.SetPassword(ctx, acct.ID, "   "); !errors.Is(err, mailcal.ErrInvalidAccount) {
		t.Fatalf("empty password: %v", err)
	}
	_ = b.SetPassword(ctx, acct.ID, "pw")
	if err := b.Forget(ctx, acct.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.GetSecret(ctx, mailcal.PasswordKey(acct.ID)); err == nil {
		t.Fatal("the secret must be gone with its account")
	}
	if _, err := b.Credentials(ctx, "caldav", acct.ID); err == nil {
		t.Fatal("no credentials after Forget")
	}
}

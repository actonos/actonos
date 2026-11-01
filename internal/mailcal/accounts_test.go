package mailcal_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
)

func TestAccountLifecycle(t *testing.T) {
	store, err := mailcal.NewAccountStore(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := store.Create(ctx, mailcal.NewAccount{
		Label: "  Work  ", PluginID: "gmail", Kind: mailcal.KindBoth, Email: "Me@Example.COM",
		Config: map[string]any{"calendar": "primary"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Label != "Work" || a.Email != "me@example.com" || a.Status != mailcal.StatusOK || a.Auth.Type != "none" {
		t.Fatalf("unexpected account %+v", a)
	}
	got, err := store.Get(ctx, a.ID)
	if err != nil || got.Config["calendar"] != "primary" {
		t.Fatalf("round trip failed: %+v %v", got, err)
	}

	updated, err := store.Update(ctx, a.ID, "Personal", "other@example.com", map[string]any{"calendar": "home"})
	if err != nil || updated.Label != "Personal" || updated.Email != "other@example.com" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if err := store.SetStatus(ctx, a.ID, mailcal.StatusAuthExpired, "token revoked"); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, a.ID)
	if got.Status != mailcal.StatusAuthExpired || got.StatusDetail != "token revoked" {
		t.Fatalf("status not stored: %+v", got)
	}
	if err := store.SetAuth(ctx, a.ID, mailcal.AuthRef{Type: "oauth", Provider: "mailcal:" + a.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, a.ID)
	if got.Auth.Type != "oauth" || got.Auth.Provider != "mailcal:"+a.ID {
		t.Fatalf("auth not stored: %+v", got.Auth)
	}

	all, err := store.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %v %v", all, err)
	}
	if err := store.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, a.ID); !errors.Is(err, mailcal.ErrAccountNotFound) {
		t.Fatalf("deleted account still found: %v", err)
	}
	for name, fn := range map[string]func() error{
		"delete":     func() error { return store.Delete(ctx, "nope") },
		"set status": func() error { return store.SetStatus(ctx, "nope", "ok", "") },
		"set auth":   func() error { return store.SetAuth(ctx, "nope", mailcal.AuthRef{Type: "none"}) },
	} {
		if err := fn(); !errors.Is(err, mailcal.ErrAccountNotFound) {
			t.Errorf("%s of an unknown account: %v", name, err)
		}
	}
}

func TestAccountValidation(t *testing.T) {
	store, _ := mailcal.NewAccountStore(newDB(t))
	ctx := context.Background()
	huge := map[string]any{"blob": strings.Repeat("x", 20*1024)}
	cases := map[string]mailcal.NewAccount{
		"no label":      {PluginID: "p", Kind: mailcal.KindMail},
		"no plugin":     {Label: "x", Kind: mailcal.KindMail},
		"bad kind":      {Label: "x", PluginID: "p", Kind: "fax"},
		"bad email":     {Label: "x", PluginID: "p", Kind: mailcal.KindMail, Email: "not an email"},
		"bad auth type": {Label: "x", PluginID: "p", Kind: mailcal.KindMail, Auth: mailcal.AuthRef{Type: "magic"}},
		"huge config":   {Label: "x", PluginID: "p", Kind: mailcal.KindMail, Config: huge},
	}
	for name, in := range cases {
		if _, err := store.Create(ctx, in); !errors.Is(err, mailcal.ErrInvalidAccount) {
			t.Errorf("%s: want ErrInvalidAccount, got %v", name, err)
		}
	}
}

func TestAccountConfigMayNotHoldSecrets(t *testing.T) {
	store, _ := mailcal.NewAccountStore(newDB(t))
	ctx := context.Background()
	for name, cfg := range map[string]map[string]any{
		"password":     {"password": "hunter2"},
		"nested token": {"imap": map[string]any{"access_token": "abc"}},
		"api key":      {"apiKey": "k"},
		"in a list":    {"servers": []any{map[string]any{"client_secret": "s"}}},
		"private key":  {"private_key": "-----BEGIN"},
	} {
		_, err := store.Create(ctx, mailcal.NewAccount{Label: "x", PluginID: "p", Kind: mailcal.KindMail, Config: cfg})
		if !errors.Is(err, mailcal.ErrInvalidAccount) || !strings.Contains(err.Error(), "vault") {
			t.Errorf("%s: want a pointer to the vault, got %v", name, err)
		}
	}
	a, err := store.Create(ctx, mailcal.NewAccount{Label: "ok", PluginID: "p", Kind: mailcal.KindMail,
		Config: map[string]any{"imap_host": "imap.example.com", "imap_port": 993}})
	if err != nil {
		t.Fatalf("ordinary settings must be accepted: %v", err)
	}
	if _, err := store.Update(ctx, a.ID, "ok", "", map[string]any{"smtp_password": "x"}); !errors.Is(err, mailcal.ErrInvalidAccount) {
		t.Fatalf("updates are checked too: %v", err)
	}
}

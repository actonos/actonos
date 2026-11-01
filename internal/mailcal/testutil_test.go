package mailcal_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/tools"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "mailcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// fixture is a service with the reference provider registered and one "both" account.
type fixture struct {
	svc      *mailcal.Service
	registry *tools.ToolRegistry
	account  mailcal.Account
}

func newFixture(t *testing.T, provider mailcal.Provider) *fixture {
	t.Helper()
	svc, err := mailcal.NewService(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if provider == nil {
		provider = mailcal.NewMemProvider(refstore.DefaultSeed())
	}
	ctx := context.Background()
	if err := svc.RegisterProvider(ctx, "refmem", "Reference", provider); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewToolRegistry(nil)
	svc.SetToolRegistry(reg)
	acct, err := svc.Accounts().Create(ctx, mailcal.NewAccount{
		Label: "Work", PluginID: "refmem", Kind: mailcal.KindBoth, Email: refstore.SelfEmail,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncTools(ctx); err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, registry: reg, account: acct}
}

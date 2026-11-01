package mailcal_test

import (
	"context"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/plugin"
)

func TestProvidersTellTheUIWhichSignInFormToShow(t *testing.T) {
	svc, err := mailcal.NewService(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	caller := mailcal.StoreCaller{Store: refstore.New(refstore.DefaultSeed())}
	svc.PluginActivated(context.Background(), plugin.PluginManifest{ID: "dav", Capabilities: []string{"calendar@1"},
		Permissions: plugin.PluginPermissions{Credentials: true}}, caller)
	svc.PluginActivated(context.Background(), plugin.PluginManifest{ID: "plain", Capabilities: []string{"mail@1"}}, caller)
	got := map[string]string{}
	for _, p := range svc.Providers() {
		got[p.PluginID] = p.AuthKind
	}
	if got["dav"] != "password" || got["plain"] != "" {
		t.Fatalf("%v", got)
	}
}

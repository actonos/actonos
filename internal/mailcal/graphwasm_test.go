package mailcal_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
	"github.com/actonos/actonos/internal/plugin"
)

func graphManifest(t *testing.T) plugin.PluginManifest {
	t.Helper()
	raw, err := os.ReadFile("plugins/graph/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m plugin.PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestGraphManifestAsksForOnlyWhatItNeeds(t *testing.T) {
	m := graphManifest(t)
	if m.OAuth == nil || m.OAuth.Provider != "microsoft" || len(m.Permissions.OAuth) != 1 || m.Permissions.OAuth[0] != "microsoft" {
		t.Fatalf("oauth block: %+v %+v", m.OAuth, m.Permissions.OAuth)
	}
	if len(m.Permissions.Secrets) != 0 || m.Permissions.Workspace || m.Permissions.Storage || len(m.Permissions.BusEvents) != 0 {
		t.Fatalf("the Graph plugin needs no secrets, storage, workspace or bus: %+v", m.Permissions)
	}
	for _, h := range m.Permissions.NetOutbound {
		if h == "*" || h[0] == '*' {
			t.Fatalf("wildcard host %q", h)
		}
	}
	for _, s := range m.OAuth.Scopes {
		if s == "Mail.ReadWrite.All" || s == "Calendars.ReadWrite.Shared" || s == "Mail.ReadWrite.Shared" {
			t.Fatalf("scope %q reaches other people's mailboxes", s)
		}
	}
}

// The real compiled plugin loads, exports the whole ABI and answers a probe without network.
func TestGraphWasmDeclaresItsCapabilities(t *testing.T) {
	m := graphManifest(t)
	host := &plugin.HostContext{PluginID: m.ID, Manifest: m, Gate: plugin.NewSecurityGate(m)}
	inst := refplugintest.InstanceOf(t, refplugintest.BuildGuest(t, "./internal/mailcal/plugins/graph/wasm"), m, host)
	p := mailcal.NewBridgeProvider(inst)
	caps, err := p.Capabilities(context.Background(), abi.AccountRef{ID: "probe"})
	if err != nil || !caps.Supports(abi.ExportMailSend) || !caps.Supports(abi.ExportCalFreeBusy) {
		t.Fatalf("%+v %v", caps, err)
	}
	// With no token broker the plugin must fail with a coded error, not crash or hang.
	if _, err := p.MailList(context.Background(), abi.AccountRef{ID: "probe"}, abi.MailListParams{}); mailcal.AsError(err) == nil {
		t.Fatalf("expected a coded error, got %v", err)
	}
}

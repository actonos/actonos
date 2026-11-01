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

func TestCalDAVManifestUsesCredentialsAndNoOAuth(t *testing.T) {
	raw, err := os.ReadFile("plugins/caldav/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m plugin.PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !m.Permissions.Credentials || m.OAuth != nil || len(m.Permissions.OAuth) != 0 || len(m.Permissions.Secrets) != 0 {
		t.Fatalf("the CalDAV plugin gets its login through acton_account only: %+v", m.Permissions)
	}
	for _, h := range m.Permissions.NetOutbound {
		if h == "*" || h[0] == '*' {
			t.Fatalf("wildcard host %q", h)
		}
	}
	if len(m.Capabilities) != 1 || m.Capabilities[0] != "calendar@1" {
		t.Fatalf("calendar only: %v", m.Capabilities)
	}
	host := &plugin.HostContext{PluginID: m.ID, Manifest: m, Gate: plugin.NewSecurityGate(m)}
	inst := refplugintest.InstanceOf(t, refplugintest.BuildGuest(t, "./internal/mailcal/plugins/caldav/wasm"), m, host)
	p := mailcal.NewBridgeProvider(inst)
	caps, err := p.Capabilities(context.Background(), abi.AccountRef{ID: "probe"})
	if err != nil || caps.Supports(abi.ExportMailSend) || !caps.Supports(abi.ExportCalCreateEvent) || caps.Supports(abi.ExportCalFreeBusy) {
		t.Fatalf("%+v %v", caps, err)
	}
}

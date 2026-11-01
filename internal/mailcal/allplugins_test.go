package mailcal_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/certificate"
	"github.com/actonos/actonos/internal/mailcal/conformance"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
	"github.com/actonos/actonos/internal/plugin"
)

// Every shipped provider plugin is held to the same bar. A new plugin directory under plugins/
// is picked up automatically: no core file needs to change and no list needs to be edited.
func shippedPlugins(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob("plugins/*/manifest.json")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no shipped plugins found: %v", err)
	}
	for i, m := range dirs {
		dirs[i] = filepath.Dir(m)
	}
	return dirs
}

func loadManifest(t *testing.T, dir string) plugin.PluginManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m plugin.PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	return m
}

func TestEveryShippedPluginAsksForOnlyWhatAProviderNeeds(t *testing.T) {
	for _, dir := range shippedPlugins(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			m := loadManifest(t, dir)
			if m.ID == "" || m.Version == "" || len(m.Capabilities) == 0 {
				t.Fatalf("incomplete manifest: %+v", m)
			}
			for _, c := range m.Capabilities {
				if c != abi.CapMail && c != abi.CapCalendar {
					t.Errorf("capability %q is not a mail or calendar ABI", c)
				}
			}
			for _, h := range append(append([]string{}, m.Permissions.NetOutbound...), m.Permissions.NetTCP...) {
				if strings.Contains(h, "*") && !strings.HasPrefix(h, "*.") {
					t.Errorf("wildcard host %q is too broad", h)
				}
				if h == "*" || strings.HasPrefix(h, "*:") {
					t.Errorf("host %q allows any destination", h)
				}
			}
			if len(m.Permissions.Secrets) != 0 || m.Permissions.Storage || m.Permissions.Workspace || len(m.Permissions.BusEvents) != 0 {
				t.Errorf("a provider needs no secrets, storage, workspace or bus: %+v", m.Permissions)
			}
			if m.OAuth != nil {
				if len(m.Permissions.OAuth) != 1 || m.Permissions.OAuth[0] != m.OAuth.Provider {
					t.Errorf("oauth block %q and permissions.oauth %v disagree", m.OAuth.Provider, m.Permissions.OAuth)
				}
				if m.Permissions.Credentials {
					t.Error("a plugin signs in either through OAuth or with a password, not both")
				}
			} else if len(m.Permissions.OAuth) != 0 {
				t.Errorf("permissions.oauth %v without an oauth block", m.Permissions.OAuth)
			}
		})
	}
}

// The compiled module loads, every operation it advertises is a real ABI export, and offline it
// behaves: no crash, no hang, only coded errors. A certificate for that build verifies.
func TestEveryShippedPluginLoadsAndCertifies(t *testing.T) {
	for _, dir := range shippedPlugins(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			m := loadManifest(t, dir)
			wasm := refplugintest.BuildGuest(t, "./internal/mailcal/plugins/"+filepath.Base(dir)+"/wasm")
			newHost := func() *plugin.HostContext {
				return &plugin.HostContext{PluginID: m.ID, Manifest: m, Gate: plugin.NewSecurityGate(m)}
			}
			p := mailcal.NewBridgeProvider(refplugintest.InstanceOf(t, wasm, m, newHost()))
			caps, err := p.Capabilities(context.Background(), abi.AccountRef{ID: "probe"})
			if err != nil || len(caps.Ops) == 0 {
				t.Fatalf("capabilities: %+v %v", caps, err)
			}
			known := map[string]bool{}
			for _, e := range abi.AllExports {
				known[e] = true
			}
			for _, op := range caps.Ops {
				if !known[op] {
					t.Errorf("advertises %q which is not an ABI export", op)
				}
			}
			for _, a := range caps.ABI {
				declared := false
				for _, c := range m.Capabilities {
					declared = declared || c == a
				}
				if !declared {
					t.Errorf("module speaks %q but the manifest does not declare it", a)
				}
			}

			passed := t.Run("shape", func(t *testing.T) {
				conformance.RunShape(t, mailcal.NewBridgeProvider(refplugintest.InstanceOf(t, wasm, m, newHost())), abi.AccountRef{ID: "probe"})
			})
			cert := certificate.Certificate{PluginID: m.ID, Version: m.Version, ABI: caps.ABI, Ops: caps.Ops,
				WasmSHA256: certificate.HashWasm(wasm), ShapeOK: passed}
			if !certificate.Verify(cert, wasm) {
				t.Fatal("a passing run must produce a certificate that verifies")
			}
		})
	}
}

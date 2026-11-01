package mailcal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/certificate"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/plugin"
)

func TestAProviderIsReportedVerifiedOnlyWhileItsCertificateMatchesTheModule(t *testing.T) {
	dir := t.TempDir()
	wasm := []byte("module bytes")
	pdir := filepath.Join(dir, "certified")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "plugin.wasm"), wasm, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := certificate.Write(filepath.Join(pdir, certificate.CertificateFile), certificate.Certificate{
		PluginID: "certified", ABI: []string{"mail@1"}, WasmSHA256: certificate.HashWasm(wasm), ShapeOK: true, At: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	svc, err := mailcal.NewService(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	svc.SetPluginsDir(dir)
	caller := mailcal.StoreCaller{Store: refstore.New(refstore.DefaultSeed())}
	for _, id := range []string{"certified", "uncertified"} {
		svc.PluginActivated(context.Background(), plugin.PluginManifest{ID: id, Capabilities: []string{"mail@1"}}, caller)
	}
	got := map[string]bool{}
	for _, p := range svc.Providers() {
		got[p.PluginID] = p.Verified
	}
	if !got["certified"] || got["uncertified"] {
		t.Fatalf("verified flags: %v", got)
	}
}

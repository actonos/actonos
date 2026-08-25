package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

func createTestActonPkg(t *testing.T, id, name, version string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	manifest := PluginManifest{
		ID:           id,
		Name:         name,
		Version:      version,
		Capabilities: []string{string(CapabilityTool)},
		Tools: []PluginToolDef{
			{
				Name:        "test_tool",
				Description: "A test tool",
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshaling test manifest: %v", err)
	}

	mf, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("creating manifest.json in zip: %v", err)
	}
	if _, err := mf.Write(manifestBytes); err != nil {
		t.Fatalf("writing manifest.json: %v", err)
	}

	wasm := []byte("\x00asm\x01\x00\x00\x00")
	wf, err := zw.Create("plugin.wasm")
	if err != nil {
		t.Fatalf("creating plugin.wasm in zip: %v", err)
	}
	if _, err := wf.Write(wasm); err != nil {
		t.Fatalf("writing plugin.wasm: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating test signing key: %v", err)
	}
	SetPluginVerifyKeys([]ed25519.PublicKey{pub})
	t.Cleanup(func() { SetPluginVerifyKeys(nil) })
	sf, err := zw.Create("signature.sig")
	if err != nil {
		t.Fatalf("creating signature.sig in zip: %v", err)
	}
	if _, err := sf.Write(ed25519.Sign(priv, wasm)); err != nil {
		t.Fatalf("writing signature.sig: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip writer: %v", err)
	}

	return buf.Bytes()
}

func TestPluginRegistryManager_FetchCatalog(t *testing.T) {
	mockCatalog := []RegistryPlugin{
		{
			ID:          "test-plugin-1",
			Name:        "Test Plugin 1",
			Version:     "1.0.0",
			Description: "First test plugin",
			Category:    "channel",
			Stars:       100,
		},
		{
			ID:          "test-plugin-2",
			Name:        "Test Plugin 2",
			Version:     "2.1.0",
			Description: "Second test plugin",
			Category:    "tool",
			Stars:       250,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": "1.0",
			"plugins": mockCatalog,
		})
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "actonos-plugins-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	prm := NewPluginRegistryManagerWithURLs(tempDir, nil, nil, server.URL, server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := prm.FetchRemoteCatalog(ctx); err != nil {
		t.Fatalf("FetchRemoteCatalog failed: %v", err)
	}

	catalog := prm.ListCatalog(ctx, nil)
	if len(catalog) != 2 {
		t.Fatalf("expected 2 plugins in catalog, got %d", len(catalog))
	}
	if catalog[0].ID != "test-plugin-1" || catalog[1].ID != "test-plugin-2" {
		t.Errorf("unexpected catalog items: %+v", catalog)
	}
}

func TestPluginRegistryManager_EmptyCatalogWhenUnreachable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "actonos-plugins-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Registry URL pointing to non-existent endpoint
	prm := NewPluginRegistryManagerWithURLs(tempDir, nil, nil, "http://127.0.0.1:54321/unreachable", "http://127.0.0.1:54321/unreachable")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// ListCatalog should gracefully return an empty slice when remote is unreachable
	catalog := prm.ListCatalog(ctx, nil)
	if len(catalog) != 0 {
		t.Errorf("expected empty catalog when remote is unreachable without fallback, got %d items", len(catalog))
	}
}

func TestPluginRegistryManager_InstallPlugin(t *testing.T) {
	pkgBytes := createTestActonPkg(t, "sample-plugin", "Sample Plugin", "1.2.3")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(pkgBytes)
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "actonos-plugins-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	eb := bus.NewEventBus()
	progressCh := eb.Subscribe(bus.EventPluginProgress)
	defer eb.Unsubscribe(bus.EventPluginProgress, progressCh)

	installedCh := eb.Subscribe(bus.EventPluginInstalled)
	defer eb.Unsubscribe(bus.EventPluginInstalled, installedCh)

	prm := NewPluginRegistryManagerWithURLs(tempDir, nil, eb, server.URL+"/registry", server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := prm.InstallPlugin(ctx, "sample-plugin", server.URL+"/sample-plugin.actonpkg")
	if err != nil {
		t.Fatalf("InstallPlugin failed: %v", err)
	}

	if info == nil || info.Manifest.ID != "sample-plugin" {
		t.Fatalf("unexpected plugin info: %+v", info)
	}

	// Verify files written to disk
	pluginDir := filepath.Join(tempDir, "sample-plugin")
	if _, err := os.Stat(filepath.Join(pluginDir, "manifest.json")); err != nil {
		t.Errorf("manifest.json was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.wasm")); err != nil {
		t.Errorf("plugin.wasm was not created: %v", err)
	}

	// Verify progress events were emitted
	select {
	case ev := <-progressCh:
		if ev.Type != bus.EventPluginProgress {
			t.Errorf("unexpected event type: %s", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for plugin progress event")
	}

	// Verify installed event was emitted
	select {
	case ev := <-installedCh:
		if ev.Type != bus.EventPluginInstalled {
			t.Errorf("unexpected event type: %s", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for plugin installed event")
	}

	// Verify ListCatalog marks plugin as Installed
	catalog := prm.ListCatalog(ctx, []PluginInfo{*info})
	for _, p := range catalog {
		if p.ID == "sample-plugin" {
			if !p.Installed {
				t.Errorf("expected sample-plugin to be marked Installed: true")
			}
		}
	}
}

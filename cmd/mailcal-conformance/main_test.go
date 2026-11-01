package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal/certificate"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
)

func writePlugin(t *testing.T, wasm []byte) (wasmPath, manifestPath string) {
	t.Helper()
	dir := t.TempDir()
	wasmPath = filepath.Join(dir, "plugin.wasm")
	manifestPath = filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(wasmPath, wasm, 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(map[string]any{"id": "refplugin", "name": "Reference", "version": "1.0.0",
		"capabilities": []string{"mail@1", "calendar@1"}})
	if err := os.WriteFile(manifestPath, m, 0o644); err != nil {
		t.Fatal(err)
	}
	return wasmPath, manifestPath
}

func TestTheReferencePluginIsCertifiedForExactlyThisBuild(t *testing.T) {
	wasm := refplugintest.WASM(t)
	wasmPath, manifestPath := writePlugin(t, wasm)
	var out, errb bytes.Buffer
	if code := run([]string{"--wasm", wasmPath, "--manifest", manifestPath, "--seed", "reference"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out.String(), errb.String())
	}
	cert, err := certificate.Read(filepath.Join(filepath.Dir(wasmPath), certificate.CertificateFile))
	if err != nil {
		t.Fatal(err)
	}
	if !cert.ShapeOK || !cert.SeededOK || cert.Live || cert.PluginID != "refplugin" || len(cert.Ops) == 0 {
		t.Fatalf("%+v", cert)
	}
	if !certificate.Verify(cert, wasm) {
		t.Fatal("the certificate must match the tested module")
	}
	changed := append([]byte(nil), wasm...)
	changed[len(changed)-1] ^= 1
	if certificate.Verify(cert, changed) {
		t.Fatal("a different build must not match")
	}
}

func TestACertificateNeverContainsSecrets(t *testing.T) {
	t.Setenv("MAILCAL_PLUGIN_TOKEN", "super-secret-token-value")
	t.Setenv("MAILCAL_PLUGIN_PASSWORD", "super-secret-password")
	t.Setenv("MAILCAL_PLUGIN_USERNAME", "someone@example.com")
	wasmPath, manifestPath := writePlugin(t, refplugintest.WASM(t))
	var out, errb bytes.Buffer
	if code := run([]string{"--wasm", wasmPath, "--manifest", manifestPath, "--live"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out.String(), errb.String())
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(wasmPath), certificate.CertificateFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"super-secret-token-value", "super-secret-password", "someone@example.com"} {
		if strings.Contains(string(raw), secret) || strings.Contains(out.String(), secret) || strings.Contains(errb.String(), secret) {
			t.Fatalf("%q leaked", secret)
		}
	}
}

func TestBadInputIsAUsageErrorNotACertificate(t *testing.T) {
	wasmPath, manifestPath := writePlugin(t, []byte("not wasm"))
	var out, errb bytes.Buffer
	if code := run([]string{"--wasm", wasmPath, "--manifest", manifestPath}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(wasmPath), certificate.CertificateFile)); err == nil {
		t.Fatal("no certificate may be written for a module that does not load")
	}
	if code := run([]string{"--wasm", wasmPath}, &out, &errb); code != 2 {
		t.Fatalf("missing manifest should be exit 2, got %d", code)
	}
	if code := run([]string{"--wasm", wasmPath, "--manifest", manifestPath, "--seed", "bogus"}, &out, &errb); code != 2 {
		t.Fatalf("bad --seed should be exit 2, got %d", code)
	}
}

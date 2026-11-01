package certificate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cert(wasm []byte, over func(*Certificate)) Certificate {
	c := Certificate{PluginID: "p", Version: "1", ABI: []string{"mail@1"}, WasmSHA256: HashWasm(wasm), ShapeOK: true, At: time.Now()}
	if over != nil {
		over(&c)
	}
	return c
}

func TestACertificateStopsMatchingWhenThePluginChanges(t *testing.T) {
	wasm := []byte("\x00asm module bytes")
	c := cert(wasm, nil)
	if !Verify(c, wasm) {
		t.Fatal("the tested build must verify")
	}
	changed := append([]byte(nil), wasm...)
	changed[3] ^= 1
	if Verify(c, changed) {
		t.Fatal("changing one byte must invalidate the certificate")
	}
}

func TestACertificateForAnUnknownABIIsRejected(t *testing.T) {
	wasm := []byte("m")
	if Verify(cert(wasm, func(c *Certificate) { c.ABI = []string{"mail@9"} }), wasm) {
		t.Fatal("an ABI this core does not speak is not verified")
	}
	if Verify(cert(wasm, func(c *Certificate) { c.ABI = nil }), wasm) {
		t.Fatal("no ABI at all is not verified")
	}
}

func TestAFailingShapeRunIsNeverCertified(t *testing.T) {
	wasm := []byte("m")
	if Verify(cert(wasm, func(c *Certificate) { c.ShapeOK = false }), wasm) {
		t.Fatal("a failed run must not verify")
	}
	if Verify(cert(wasm, func(c *Certificate) { c.WasmSHA256 = "" }), wasm) {
		t.Fatal("a certificate without a hash must not verify")
	}
}

func TestVerifyDirTreatsEveryProblemAsNotVerified(t *testing.T) {
	dir := t.TempDir()
	if VerifyDir(dir) {
		t.Fatal("an empty directory is not verified")
	}
	wasm := []byte("module")
	if err := os.WriteFile(filepath.Join(dir, "plugin.wasm"), wasm, 0o644); err != nil {
		t.Fatal(err)
	}
	if VerifyDir(dir) {
		t.Fatal("a module without a certificate is not verified")
	}
	if err := Write(filepath.Join(dir, CertificateFile), cert(wasm, nil)); err != nil {
		t.Fatal(err)
	}
	if !VerifyDir(dir) {
		t.Fatal("module and matching certificate should verify")
	}
	if err := os.WriteFile(filepath.Join(dir, CertificateFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if VerifyDir(dir) {
		t.Fatal("a corrupt certificate is not verified")
	}
}

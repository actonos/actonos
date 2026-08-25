package plugin

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
)

func TestVerifyPluginSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetPluginVerifyKeys(nil) })
	SetPluginVerifyKeys([]ed25519.PublicKey{pub})

	wasm := []byte("\x00asm\x01\x00\x00\x00 module")
	sig := ed25519.Sign(priv, wasm)

	if err := VerifyPluginSignature(wasm, sig); err != nil {
		t.Fatalf("valid raw signature rejected: %v", err)
	}
	if err := VerifyPluginSignature(wasm, []byte(hex.EncodeToString(sig))); err != nil {
		t.Fatalf("valid hex signature rejected: %v", err)
	}
	if err := VerifyPluginSignature(wasm, nil); !errors.Is(err, ErrMissingSignature) {
		t.Fatalf("expected missing signature, got %v", err)
	}
	if err := VerifyPluginSignature(wasm, []byte("not-a-signature")); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected invalid signature, got %v", err)
	}
	if err := VerifyOptionalPluginSignature(wasm, nil); err != nil {
		t.Fatalf("optional verify should allow unsigned: %v", err)
	}

	t.Cleanup(clearAllowUnsignedPlugins)
	SetAllowUnsignedPlugins(false)
	if err := VerifyRemotePluginPackage(wasm, nil); !errors.Is(err, ErrMissingSignature) {
		t.Fatalf("remote install must reject unsigned packages, got %v", err)
	}
	SetAllowUnsignedPlugins(true)
	if err := VerifyRemotePluginPackage(wasm, nil); err != nil {
		t.Fatalf("unsigned override should allow remote install: %v", err)
	}
}

func TestVerifyRemotePluginPackageRequiresKeys(t *testing.T) {
	t.Cleanup(func() {
		SetPluginVerifyKeys(nil)
		clearAllowUnsignedPlugins()
	})
	SetPluginVerifyKeys(nil)
	SetAllowUnsignedPlugins(false)
	wasm := []byte("\x00asm\x01\x00\x00\x00")
	sig := make([]byte, ed25519.SignatureSize)
	if err := VerifyRemotePluginPackage(wasm, sig); !errors.Is(err, ErrNoPluginVerifyKeys) {
		t.Fatalf("expected no keys error, got %v", err)
	}
}

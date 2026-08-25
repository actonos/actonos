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
	t.Cleanup(func() {
		SetPluginVerifyKeys(nil)
		clearAllowUnsignedPlugins()
		clearRequireSignedPlugins()
	})
	SetPluginVerifyKeys([]ed25519.PublicKey{pub})

	manifest := []byte(`{"id":"demo"}`)
	wasm := []byte("\x00asm\x01\x00\x00\x00 module")
	sig := ed25519.Sign(priv, PluginSignatureMessage(manifest, wasm))

	if err := VerifyPluginSignature(manifest, wasm, sig); err != nil {
		t.Fatalf("valid raw signature rejected: %v", err)
	}
	if err := VerifyPluginSignature(manifest, wasm, []byte(hex.EncodeToString(sig))); err != nil {
		t.Fatalf("valid hex signature rejected: %v", err)
	}
	if err := VerifyPluginSignature(manifest, wasm, nil); !errors.Is(err, ErrMissingSignature) {
		t.Fatalf("expected missing signature, got %v", err)
	}
	if err := VerifyPluginSignature(manifest, wasm, []byte("not-a-signature")); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected invalid signature, got %v", err)
	}
	if err := VerifyPluginSignature(manifest, wasm, ed25519.Sign(priv, wasm)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("raw-wasm signature must not verify: %v", err)
	}
	if err := VerifyOptionalPluginSignature(manifest, wasm, nil); err != nil {
		t.Fatalf("optional verify should allow unsigned: %v", err)
	}
	if err := VerifyRemotePluginPackage(manifest, wasm, nil); err != nil {
		t.Fatalf("SDK pack without signature.sig must install: %v", err)
	}

	SetRequireSignedPlugins(true)
	if err := VerifyRemotePluginPackage(manifest, wasm, nil); !errors.Is(err, ErrMissingSignature) {
		t.Fatalf("required signatures must reject unsigned packages, got %v", err)
	}
	SetAllowUnsignedPlugins(true)
	if err := VerifyRemotePluginPackage(manifest, wasm, nil); err != nil {
		t.Fatalf("unsigned override should allow remote install: %v", err)
	}
}

func TestVerifyRemotePluginPackageRequiresKeys(t *testing.T) {
	t.Cleanup(func() {
		SetPluginVerifyKeys(nil)
		clearAllowUnsignedPlugins()
		clearRequireSignedPlugins()
	})
	SetPluginVerifyKeys(nil)
	wasm := []byte("\x00asm\x01\x00\x00\x00")
	sig := make([]byte, ed25519.SignatureSize)
	if err := VerifyRemotePluginPackage(nil, wasm, sig); !errors.Is(err, ErrNoPluginVerifyKeys) {
		t.Fatalf("expected no keys error, got %v", err)
	}
}

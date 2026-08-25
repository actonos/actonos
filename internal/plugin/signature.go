package plugin

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	ErrMissingSignature   = errors.New("plugin package is unsigned")
	ErrInvalidSignature   = errors.New("plugin signature verification failed")
	ErrNoPluginVerifyKeys = errors.New("no plugin signing keys configured")
)

var (
	verifyKeysMu     sync.RWMutex
	extraVerifyKeys  []ed25519.PublicKey
	allowUnsignedMu  sync.RWMutex
	allowUnsigned    *bool
	requireSignedMu  sync.RWMutex
	requireSigned    *bool
)

// SetPluginVerifyKeys replaces process-local extra keys (tests). Official keys
// still come from ACTONOS_PLUGIN_PUBKEYS.
func SetPluginVerifyKeys(keys []ed25519.PublicKey) {
	verifyKeysMu.Lock()
	defer verifyKeysMu.Unlock()
	extraVerifyKeys = append([]ed25519.PublicKey(nil), keys...)
}

// SetAllowUnsignedPlugins overrides ACTONOS_ALLOW_UNSIGNED_PLUGINS for tests.
func SetAllowUnsignedPlugins(allow bool) {
	allowUnsignedMu.Lock()
	defer allowUnsignedMu.Unlock()
	v := allow
	allowUnsigned = &v
}

func clearAllowUnsignedPlugins() {
	allowUnsignedMu.Lock()
	defer allowUnsignedMu.Unlock()
	allowUnsigned = nil
}

func unsignedPluginsAllowed() bool {
	allowUnsignedMu.RLock()
	override := allowUnsigned
	allowUnsignedMu.RUnlock()
	if override != nil {
		return *override
	}
	return os.Getenv("ACTONOS_ALLOW_UNSIGNED_PLUGINS") == "1"
}

// SetRequireSignedPlugins overrides ACTONOS_REQUIRE_SIGNED_PLUGINS for tests.
func SetRequireSignedPlugins(require bool) {
	requireSignedMu.Lock()
	defer requireSignedMu.Unlock()
	v := require
	requireSigned = &v
}

func clearRequireSignedPlugins() {
	requireSignedMu.Lock()
	defer requireSignedMu.Unlock()
	requireSigned = nil
}

func signedPluginsRequired() bool {
	if unsignedPluginsAllowed() {
		return false
	}
	requireSignedMu.RLock()
	override := requireSigned
	requireSignedMu.RUnlock()
	if override != nil {
		return *override
	}
	return os.Getenv("ACTONOS_REQUIRE_SIGNED_PLUGINS") == "1"
}

// PluginSignatureMessage is SHA-256(manifest.json || plugin.wasm), matching acton-plugin sign.
func PluginSignatureMessage(manifest, wasm []byte) []byte {
	h := sha256.New()
	h.Write(manifest)
	h.Write(wasm)
	return h.Sum(nil)
}

func pluginVerifyKeys() []ed25519.PublicKey {
	var keys []ed25519.PublicKey
	if raw := strings.TrimSpace(os.Getenv("ACTONOS_PLUGIN_PUBKEYS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if k, err := parseEd25519PublicKey([]byte(strings.TrimSpace(part))); err == nil {
				keys = append(keys, k)
			}
		}
	}
	verifyKeysMu.RLock()
	keys = append(keys, extraVerifyKeys...)
	verifyKeysMu.RUnlock()
	return keys
}

func parseEd25519PublicKey(raw []byte) (ed25519.PublicKey, error) {
	trimmed := strings.TrimSpace(string(raw))
	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		decoded = raw
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ed25519 public key must be %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func decodePluginSignature(sig []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(sig))
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}
	if len(sig) == ed25519.SignatureSize {
		return sig, nil
	}
	return nil, ErrInvalidSignature
}

// VerifyPluginSignature checks Ed25519(sig, SHA-256(manifest||wasm)) against configured public keys.
func VerifyPluginSignature(manifest, wasm, sig []byte) error {
	if len(wasm) == 0 {
		return fmt.Errorf("%w: empty wasm module", ErrInvalidSignature)
	}
	if len(sig) == 0 {
		return ErrMissingSignature
	}
	rawSig, err := decodePluginSignature(sig)
	if err != nil {
		return err
	}
	keys := pluginVerifyKeys()
	if len(keys) == 0 {
		return ErrNoPluginVerifyKeys
	}
	msg := PluginSignatureMessage(manifest, wasm)
	for _, key := range keys {
		if ed25519.Verify(key, msg, rawSig) {
			return nil
		}
	}
	return ErrInvalidSignature
}

// VerifyRemotePluginPackage verifies a signature when one is present.
// Unsigned packages match acton-plugin pack (signature.sig is optional) unless
// ACTONOS_REQUIRE_SIGNED_PLUGINS=1. A present signature is always verified.
func VerifyRemotePluginPackage(manifest, wasm, sig []byte) error {
	if len(sig) == 0 {
		if signedPluginsRequired() {
			return ErrMissingSignature
		}
		return nil
	}
	return VerifyPluginSignature(manifest, wasm, sig)
}

// VerifyOptionalPluginSignature verifies a signature when one is present.
// Missing signatures are allowed for operator-uploaded packages.
func VerifyOptionalPluginSignature(manifest, wasm, sig []byte) error {
	if len(sig) == 0 {
		return nil
	}
	return VerifyPluginSignature(manifest, wasm, sig)
}

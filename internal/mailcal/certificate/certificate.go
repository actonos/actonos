package certificate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

// CertificateFile is the name of the certificate next to a plugin's plugin.wasm.
const CertificateFile = "conformance.json"

// Certificate records that one exact build of a provider plugin passed the conformance suite.
// It is information for the person installing the plugin, not a security boundary: a plugin is
// always treated as untrusted code no matter what its certificate says.
type Certificate struct {
	PluginID   string   `json:"plugin_id"`
	Version    string   `json:"version"`
	ABI        []string `json:"abi"`
	Ops        []string `json:"ops"`
	WasmSHA256 string   `json:"wasm_sha256"`
	ShapeOK    bool     `json:"shape_ok"`
	SeededOK   bool     `json:"seeded_ok,omitempty"`
	// Live is true when the shape checks ran against a real account rather than offline.
	Live bool      `json:"live,omitempty"`
	At   time.Time `json:"at"`
	Tool string    `json:"tool"`
}

// HashWasm returns the SHA-256 of a plugin module in hex.
func HashWasm(wasm []byte) string {
	sum := sha256.Sum256(wasm)
	return hex.EncodeToString(sum[:])
}

// Verify reports whether c vouches for exactly this module: the suite passed, the module is the
// one that was tested, and it speaks an ABI version this core understands.
func Verify(c Certificate, wasm []byte) bool {
	if !c.ShapeOK || c.WasmSHA256 == "" || c.WasmSHA256 != HashWasm(wasm) {
		return false
	}
	for _, a := range c.ABI {
		if a == abi.CapMail || a == abi.CapCalendar {
			return true
		}
	}
	return false
}

// Write stores a certificate as JSON.
func Write(path string, c Certificate) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Read loads a certificate.
func Read(path string) (Certificate, error) {
	var c Certificate
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("reading %s: %w", path, err)
	}
	return c, nil
}

// VerifyDir checks the certificate and module that sit in a plugin directory. Any missing or
// unreadable file means "not verified", never an error.
func VerifyDir(dir string) bool {
	c, err := Read(filepath.Join(dir, CertificateFile))
	if err != nil {
		return false
	}
	wasm, err := os.ReadFile(filepath.Join(dir, "plugin.wasm"))
	if err != nil {
		return false
	}
	return Verify(c, wasm)
}

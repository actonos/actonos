// Package refplugintest builds and instantiates the reference WASM provider for tests.
package refplugintest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/actonos/actonos/internal/plugin"
)

var (
	buildOnce sync.Once
	wasmBytes []byte
	buildErr  error
)

func moduleRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// .../internal/mailcal/refplugin/refplugintest/build.go -> module root
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

// WASM compiles the guest once per test process and returns the module bytes. It skips
// the test when the Go toolchain is unavailable.
func WASM(tb testing.TB) []byte {
	tb.Helper()
	buildOnce.Do(func() {
		goBin, err := exec.LookPath("go")
		if err != nil {
			buildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "refplugin-*")
		if err != nil {
			buildErr = err
			return
		}
		out := filepath.Join(dir, "refplugin.wasm")
		cmd := exec.Command(goBin, "build", "-buildmode=c-shared", "-o", out, "./internal/mailcal/refplugin/guest")
		cmd.Dir = moduleRoot()
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
		if combined, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err: err, output: string(combined)}
			return
		}
		wasmBytes, buildErr = os.ReadFile(out)
	})
	if buildErr != nil {
		tb.Skipf("cannot build the reference WASM provider: %v", buildErr)
	}
	return wasmBytes
}

type buildError struct {
	err    error
	output string
}

func (e *buildError) Error() string { return e.err.Error() + ": " + e.output }

var (
	loaderOnce sync.Once
	sharedLoad *plugin.WasmLoader
	loadErr    error
)

const compiledID = "refplugin"

// sharedLoader compiles the guest once per process. Instances are cheap: each gets its
// own memory and its own empty dataset, so tests stay isolated.
func sharedLoader(tb testing.TB) *plugin.WasmLoader {
	tb.Helper()
	wasm := WASM(tb)
	loaderOnce.Do(func() {
		ctx := context.Background()
		l, err := plugin.NewWasmLoader(ctx)
		if err != nil {
			loadErr = err
			return
		}
		if _, err := l.Compile(ctx, compiledID, wasm); err != nil {
			loadErr = err
			return
		}
		sharedLoad = l
	})
	if loadErr != nil {
		tb.Fatalf("preparing the reference provider: %v", loadErr)
	}
	return sharedLoad
}

// Instance returns a running reference provider with a fresh DefaultSeed dataset. It is
// closed automatically when the test ends.
func Instance(tb testing.TB) *plugin.PluginInstance {
	tb.Helper()
	loader := sharedLoader(tb)
	ctx := context.Background()
	manifest := plugin.PluginManifest{ID: compiledID, Name: "Reference mail/calendar", Version: "1.0.0",
		Capabilities: []string{"mail@1", "calendar@1"}}
	host := &plugin.HostContext{PluginID: compiledID, Manifest: manifest, Gate: plugin.NewSecurityGate(manifest)}
	inst, err := loader.Instantiate(ctx, compiledID, manifest, host)
	if err != nil {
		tb.Fatalf("instantiating reference provider: %v", err)
	}
	tb.Cleanup(func() { _ = inst.Close(ctx) })
	return inst
}

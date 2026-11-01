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

var (
	guestMu    sync.Mutex
	guestCache = map[string][]byte{}
)

// BuildGuest compiles any guest package of this module (for example
// "./internal/mailcal/refplugin/netguest") to WebAssembly once per test process. It skips the
// test when the Go toolchain is unavailable and fails it when the build is broken.
func BuildGuest(tb testing.TB, pkg string) []byte {
	tb.Helper()
	guestMu.Lock()
	defer guestMu.Unlock()
	if b, ok := guestCache[pkg]; ok {
		return b
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		tb.Skipf("no Go toolchain to build %s: %v", pkg, err)
	}
	dir, err := os.MkdirTemp("", "guest-*")
	if err != nil {
		tb.Fatal(err)
	}
	out := filepath.Join(dir, "guest.wasm")
	cmd := exec.Command(goBin, "build", "-buildmode=c-shared", "-o", out, pkg)
	cmd.Dir = moduleRoot()
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if combined, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("building %s: %v\n%s", pkg, err, combined)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		tb.Fatal(err)
	}
	guestCache[pkg] = b
	return b
}

var (
	loadersMu sync.Mutex
	loaders   = map[string]*plugin.WasmLoader{}
)

// InstanceOf instantiates a guest built with BuildGuest. The module is compiled once per
// process and manifest.ID; instances are cheap, so tests can create one per case. Set
// host.Manifest and host.Gate from the manifest before calling.
func InstanceOf(tb testing.TB, wasm []byte, manifest plugin.PluginManifest, host *plugin.HostContext) *plugin.PluginInstance {
	tb.Helper()
	ctx := context.Background()
	loadersMu.Lock()
	loader, ok := loaders[manifest.ID]
	if !ok {
		var err error
		if loader, err = plugin.NewWasmLoader(ctx); err != nil {
			loadersMu.Unlock()
			tb.Fatal(err)
		}
		if _, err := loader.Compile(ctx, manifest.ID, wasm); err != nil {
			loadersMu.Unlock()
			tb.Fatalf("compiling %s: %v", manifest.ID, err)
		}
		loaders[manifest.ID] = loader
	}
	loadersMu.Unlock()
	inst, err := loader.Instantiate(ctx, manifest.ID, manifest, host)
	if err != nil {
		tb.Fatalf("instantiating %s: %v", manifest.ID, err)
	}
	tb.Cleanup(func() { _ = inst.Close(ctx) })
	return inst
}

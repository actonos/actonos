package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/certificate"
	"github.com/actonos/actonos/internal/mailcal/conformance"
	"github.com/actonos/actonos/internal/plugin"
)

const toolName = "mailcal-conformance 1"

type options struct {
	wasm, manifest, out, seed, account string
	live                               bool
}

// run returns the process exit code: 0 certified, 1 the suite failed, 2 bad usage or load error.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mailcal-conformance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.wasm, "wasm", "", "path to plugin.wasm")
	fs.StringVar(&o.manifest, "manifest", "", "path to manifest.json")
	fs.StringVar(&o.out, "out", "", "where to write the certificate (default: conformance.json next to the wasm)")
	fs.StringVar(&o.seed, "seed", "none", "reference: also run the seeded behaviour suite; none: shape only")
	fs.StringVar(&o.account, "account", "", "JSON file with the account reference to use ({\"id\":...,\"config\":{...}})")
	fs.BoolVar(&o.live, "live", false, "run against a real account, using MAILCAL_PLUGIN_* environment variables")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if o.wasm == "" || o.manifest == "" || (o.seed != "none" && o.seed != "reference") {
		fmt.Fprintln(stderr, "usage: mailcal-conformance --wasm plugin.wasm --manifest manifest.json [--seed reference|none] [--account account.json] [--live] [--out conformance.json]")
		return 2
	}
	cert, ok, err := certify(context.Background(), o, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "mailcal-conformance:", err)
		return 2
	}
	out := o.out
	if out == "" {
		out = filepath.Join(filepath.Dir(o.wasm), certificate.CertificateFile)
	}
	if !ok {
		fmt.Fprintf(stdout, "FAIL %s: the suite found problems; no certificate written\n", cert.PluginID)
		return 1
	}
	if err := certificate.Write(out, cert); err != nil {
		fmt.Fprintln(stderr, "mailcal-conformance:", err)
		return 2
	}
	fmt.Fprintf(stdout, "OK %s %s: certified %s (wasm sha256 %s)\n", cert.PluginID, cert.Version, out, cert.WasmSHA256[:12])
	return 0
}

func certify(ctx context.Context, o options, stdout io.Writer) (certificate.Certificate, bool, error) {
	var cert certificate.Certificate
	wasm, err := os.ReadFile(o.wasm)
	if err != nil {
		return cert, false, fmt.Errorf("reading the plugin: %w", err)
	}
	var manifest plugin.PluginManifest
	raw, err := os.ReadFile(o.manifest)
	if err != nil {
		return cert, false, fmt.Errorf("reading the manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return cert, false, fmt.Errorf("parsing the manifest: %w", err)
	}
	acct := abi.AccountRef{ID: "conformance"}
	if o.account != "" {
		raw, err := os.ReadFile(o.account)
		if err != nil {
			return cert, false, fmt.Errorf("reading the account file: %w", err)
		}
		if err := json.Unmarshal(raw, &acct); err != nil {
			return cert, false, fmt.Errorf("parsing the account file: %w", err)
		}
	}

	loader, err := plugin.NewWasmLoader(ctx)
	if err != nil {
		return cert, false, fmt.Errorf("starting the WASM runtime: %w", err)
	}
	defer loader.Close(ctx)
	if _, err := loader.Compile(ctx, manifest.ID, wasm); err != nil {
		return cert, false, fmt.Errorf("compiling the plugin: %w", err)
	}

	// Every instance is wired the way the Manager would wire it: same gate, same host modules.
	instantiate := func() (*plugin.PluginInstance, error) {
		host := &plugin.HostContext{PluginID: manifest.ID, Manifest: manifest, Gate: plugin.NewSecurityGate(manifest)}
		if o.live {
			host.Tokens = envTokens{provider: oauthProvider(manifest)}
			host.Credentials = envCredentials{}
		}
		return loader.Instantiate(ctx, manifest.ID, manifest, host)
	}

	probe, err := instantiate()
	if err != nil {
		return cert, false, fmt.Errorf("instantiating the plugin: %w", err)
	}
	caps, err := mailcal.NewBridgeProvider(probe).Capabilities(ctx, acct)
	_ = probe.Close(ctx)
	if err != nil {
		return cert, false, fmt.Errorf("the plugin did not answer the capabilities probe: %w", err)
	}

	tests := []testing.InternalTest{{Name: "Shape", F: func(t *testing.T) {
		fmt.Fprintln(stdout, "running the shape checks")
		inst, err := instantiate()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = inst.Close(ctx) })
		conformance.RunShape(t, mailcal.NewBridgeProvider(inst), acct)
	}}}
	if o.seed == "reference" {
		tests = append(tests, testing.InternalTest{Name: "Seeded", F: func(t *testing.T) {
			fmt.Fprintln(stdout, "running the seeded behaviour checks")
			conformance.RunSeeded(t, func(t *testing.T) mailcal.Provider {
				inst, err := instantiate()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = inst.Close(ctx) })
				return mailcal.NewBridgeProvider(inst)
			})
		}})
	}
	testing.Init()
	// testing prints "no tests to run" to stderr when the synthetic root has no subtests of its
	// own; the named checks above did run (they announce themselves on stdout) and failures are
	// reported by testing.RunTests as usual.
	passed := testing.RunTests(func(_, _ string) (bool, error) { return true, nil }, tests)

	cert = certificate.Certificate{
		PluginID: manifest.ID, Version: manifest.Version, ABI: caps.ABI, Ops: caps.Ops,
		WasmSHA256: certificate.HashWasm(wasm), ShapeOK: passed, SeededOK: passed && o.seed == "reference",
		Live: o.live, At: time.Now().UTC().Truncate(time.Second), Tool: toolName,
	}
	return cert, passed, nil
}

func oauthProvider(m plugin.PluginManifest) string {
	if m.OAuth != nil {
		return m.OAuth.Provider
	}
	return ""
}

// envTokens and envCredentials read the live login from the environment so that nothing secret
// is ever a flag, a file argument or part of the certificate.
type envTokens struct{ provider string }

func (e envTokens) AccessToken(_ context.Context, _, _ string) (plugin.TokenGrant, error) {
	tok := os.Getenv("MAILCAL_PLUGIN_TOKEN")
	if tok == "" {
		return plugin.TokenGrant{}, errors.New("MAILCAL_PLUGIN_TOKEN is not set")
	}
	return plugin.TokenGrant{Provider: e.provider, AccessToken: tok, ExpiresAt: time.Now().Add(10 * time.Minute)}, nil
}

type envCredentials struct{}

func (envCredentials) Credentials(_ context.Context, _, _ string) (plugin.Credentials, error) {
	user, pass := os.Getenv("MAILCAL_PLUGIN_USERNAME"), os.Getenv("MAILCAL_PLUGIN_PASSWORD")
	if user == "" || pass == "" {
		return plugin.Credentials{}, errors.New("MAILCAL_PLUGIN_USERNAME and MAILCAL_PLUGIN_PASSWORD are not set")
	}
	return plugin.Credentials{Username: user, Password: pass}, nil
}

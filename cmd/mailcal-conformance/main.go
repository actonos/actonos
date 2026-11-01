// Command mailcal-conformance runs the provider conformance suite against one compiled plugin
// and writes a certificate for exactly that build.
//
//	mailcal-conformance --wasm plugin.wasm --manifest manifest.json --out conformance.json
//
// By default the run is offline: no token, no credentials, no network. Operations that need an
// account must then fail with a coded error rather than crash or hang, which is what the suite
// checks. A person with a real sandbox account can add --live; the token or login comes from
// environment variables (never from flags, files or the certificate):
//
//	MAILCAL_PLUGIN_TOKEN       OAuth access token handed to acton_oauth
//	MAILCAL_PLUGIN_USERNAME    login handed to acton_account
//	MAILCAL_PLUGIN_PASSWORD    app password handed to acton_account
//
// --seed reference additionally runs the seeded behaviour suite and is meant for plugins built on
// refstore. The suite lives in a package that needs *testing.T, so the command drives it through
// testing.RunTests: that is the same code path go test uses, with no second implementation.
package main

import (
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

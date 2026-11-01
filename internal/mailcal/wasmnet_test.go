package mailcal_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
	"github.com/actonos/actonos/internal/plugin"
)

type cannedTransport struct {
	mu     sync.Mutex
	status int
	body   string
	seen   []http.Header
	urls   []string
}

func (c *cannedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, req.Header.Clone())
	c.urls = append(c.urls, req.URL.String())
	return &http.Response{StatusCode: c.status, Body: io.NopCloser(bytes.NewBufferString(c.body)), Header: http.Header{}, Request: req}, nil
}

type grantBroker struct{ err error }

func (g grantBroker) AccessToken(_ context.Context, pluginID, accountID string) (plugin.TokenGrant, error) {
	if g.err != nil {
		return plugin.TokenGrant{}, g.err
	}
	return plugin.TokenGrant{Provider: "google", AccessToken: "tok-" + accountID, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func probeInstance(t *testing.T, perms plugin.PluginPermissions, broker plugin.TokenBroker, tr http.RoundTripper) *plugin.PluginInstance {
	t.Helper()
	manifest := plugin.PluginManifest{ID: "netprobe", Name: "probe", Version: "1", Capabilities: []string{"mail@1"}, Permissions: perms}
	host := &plugin.HostContext{PluginID: manifest.ID, Manifest: manifest, Gate: plugin.NewSecurityGate(manifest),
		Tokens: broker, HTTPClient: &http.Client{Transport: tr}}
	return refplugintest.InstanceOf(t, refplugintest.BuildGuest(t, "./internal/mailcal/refplugin/netguest"), manifest, host)
}

func TestAWasmProviderGetsATokenAndCallsTheNetworkThroughTheHost(t *testing.T) {
	tr := &cannedTransport{status: 200, body: "hello"}
	inst := probeInstance(t, plugin.PluginPermissions{NetOutbound: []string{"1.1.1.1"}, OAuth: []string{"google"}}, grantBroker{}, tr)
	p := mailcal.NewBridgeProvider(inst)
	res, err := p.MailList(context.Background(), abi.AccountRef{ID: "acct_9", Config: map[string]any{"url": "https://1.1.1.1/api"}}, abi.MailListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Subject != "200 hello" {
		t.Fatalf("unexpected result %+v", res.Messages)
	}
	if got := tr.seen[0].Get("Authorization"); got != "Bearer tok-acct_9" {
		t.Fatalf("the provider must send the brokered token, got %q", got)
	}
}

func TestAWasmProviderCannotReachHostsOutsideItsManifestOrWithoutOAuthPermission(t *testing.T) {
	tr := &cannedTransport{status: 200, body: "x"}
	acct := abi.AccountRef{ID: "acct_9", Config: map[string]any{"url": "https://1.1.1.1/api"}}

	// Network allowlist does not include the host.
	inst := probeInstance(t, plugin.PluginPermissions{NetOutbound: []string{"8.8.8.8"}, OAuth: []string{"google"}}, grantBroker{}, tr)
	if _, err := mailcal.NewBridgeProvider(inst).MailList(context.Background(), acct, abi.MailListParams{}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("a host outside net_outbound must be refused with 'invalid', got %v", err)
	}
	if len(tr.seen) != 0 {
		t.Fatal("the request must not reach the network")
	}

	// No OAuth permission: no token, so the provider cannot even try.
	inst = probeInstance(t, plugin.PluginPermissions{NetOutbound: []string{"1.1.1.1"}}, grantBroker{}, tr)
	if _, err := mailcal.NewBridgeProvider(inst).MailList(context.Background(), acct, abi.MailListParams{}); err == nil {
		t.Fatal("a plugin without permissions.oauth must not receive a token")
	}
	if len(tr.seen) != 0 {
		t.Fatal("no request without a token")
	}
}

func TestAnExpiredLoginSurfacesAsAuthExpiredAndAProviderErrorKeepsItsCode(t *testing.T) {
	tr := &cannedTransport{status: 200, body: "x"}
	acct := abi.AccountRef{ID: "acct_9", Config: map[string]any{"url": "https://1.1.1.1/api"}}
	perms := plugin.PluginPermissions{NetOutbound: []string{"1.1.1.1"}, OAuth: []string{"google"}}

	inst := probeInstance(t, perms, grantBroker{err: plugin.ErrTokenAuthExpired}, tr)
	if _, err := mailcal.NewBridgeProvider(inst).MailList(context.Background(), acct, abi.MailListParams{}); !mailcal.IsCode(err, abi.ErrAuthExpired) {
		t.Fatalf("got %v", err)
	}

	for status, want := range map[int]abi.ErrorCode{401: abi.ErrAuthExpired, 404: abi.ErrNotFound, 429: abi.ErrRateLimited, 503: abi.ErrTransient, 400: abi.ErrInvalid} {
		tr := &cannedTransport{status: status, body: "nope"}
		inst := probeInstance(t, perms, grantBroker{}, tr)
		if _, err := mailcal.NewBridgeProvider(inst).MailList(context.Background(), acct, abi.MailListParams{}); !mailcal.IsCode(err, want) {
			t.Errorf("HTTP %d: want %s, got %v", status, want, err)
		}
	}
}

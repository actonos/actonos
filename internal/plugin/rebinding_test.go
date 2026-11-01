package plugin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/security"
)

// The default client must refuse to connect to a private address even when the name it was
// given resolved to a public one a moment ago.
func TestThePluginHTTPClientDialsOnlyTheVettedAddress(t *testing.T) {
	var lookups atomic.Int32
	h := &HostContext{Dial: security.DialPolicy{Resolve: func(context.Context, string) ([]net.IP, error) {
		if lookups.Add(1) == 1 {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}, ConnectTimeout: 300 * time.Millisecond}}
	tr := sandboxedHTTPClient(h, time.Second).Transport.(*http.Transport)
	conn, err := tr.DialContext(context.Background(), "tcp", "evil.example:443")
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("nothing listens on the documentation address, so the dial must fail")
	}
	if lookups.Load() != 1 {
		t.Fatalf("the dialer must resolve the name exactly once (got %d lookups); a second lookup is the rebinding window", lookups.Load())
	}
}

func TestThePluginHTTPClientRefusesLoopbackAndPrivateTargetsAtDialTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secret admin page")) }))
	defer srv.Close()
	client := sandboxedHTTPClient(&HostContext{}, time.Second)
	resp, err := client.Get(srv.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, security.ErrUnsafeAddress) {
		t.Fatalf("a loopback server must be unreachable even if the URL check were bypassed, got %v", err)
	}
	if client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("a proxy would resolve names outside the policy")
	}
}

func TestACallerSuppliedClientIsStillHonoured(t *testing.T) {
	custom := &http.Client{Transport: staticTransport{status: 200, body: "ok"}}
	got := sandboxedHTTPClient(&HostContext{HTTPClient: custom}, time.Second)
	if _, ok := got.Transport.(staticTransport); !ok {
		t.Fatalf("an injected client (tests) must keep its transport, got %T", got.Transport)
	}
}

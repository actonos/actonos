package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMailcalOAuthCallbackFailsClosedAndLeaksNothing(t *testing.T) {
	srv := newTestServer(t)
	for _, target := range []string{
		"/api/mailcal/oauth/callback",
		"/api/mailcal/oauth/callback?state=nope&code=abc",
		"/api/mailcal/oauth/callback?error=access_denied&error_description=secret-details",
	} {
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/#/mailcal?oauth=") {
			t.Fatalf("%s: code=%d location=%q", target, rec.Code, loc)
		}
		if strings.Contains(loc, "secret-details") || strings.Contains(loc, "abc") {
			t.Fatalf("the redirect must not echo provider data: %q", loc)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("the callback response must not be cached or leak a referrer: %v", rec.Header())
		}
	}
}

func TestMailcalOAuthStartNeedsAFlow(t *testing.T) {
	srv := newTestServer(t)
	status, _ := call(t, srv, http.MethodPost, "/api/mailcal/accounts/acct_x/oauth/start", "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", status)
	}
	status, env := call(t, srv, http.MethodGet, "/api/mailcal/oauth/clients/google", "")
	if status != http.StatusOK || dataOf(t, env)["configured"] != false {
		t.Fatalf("client status: %d %v", status, env)
	}
}

func TestAnOAuthAccountIsNotConnectedUntilTheSignInFinishes(t *testing.T) {
	srv := newTestServer(t)
	status, env := call(t, srv, http.MethodPost, "/api/mailcal/accounts", `{"label":"G","plugin_id":"refmem","kind":"both","auth":{"type":"oauth"}}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, env)
	}
	if got := dataOf(t, env)["status"]; got != "auth_expired" {
		t.Fatalf("a fresh OAuth account must not look connected, got %v", got)
	}
}

func TestOAuthSignInNeedsHTTPSExceptOnThisMachine(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		header string
		tls    bool
		want   bool
	}{
		{"plain http on a LAN address", "192.168.1.20:8080", "", false, false},
		{"plain http on a .local name", "acton-mini.local:8080", "", false, false},
		{"plain http on a tailnet name", "acton-mini.tail1234.ts.net", "", false, false},
		{"localhost is fine for providers", "localhost:8080", "", false, true},
		{"loopback address", "127.0.0.1:8080", "", false, true},
		{"loopback ipv6", "[::1]:8080", "", false, true},
		{"behind a proxy that says https", "agent.example.com", "https", false, true},
		{"direct TLS (Tailscale HTTPS)", "acton-mini.tail1234.ts.net", "", true, true},
		{"a forged proto header other than https", "192.168.1.20:8080", "http", false, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/api/mailcal/accounts/a/oauth/start", nil)
		r.Host = c.host
		if c.header != "" {
			r.Header.Set("X-Forwarded-Proto", c.header)
		}
		if c.tls {
			r.TLS = &tls.ConnectionState{}
		}
		if got := oauthOriginAllowed(r); got != c.want {
			t.Errorf("%s: allowed=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestHTTPSRequiredMessageSaysWhatToDo(t *testing.T) {
	srv := newTestServer(t)
	if msg := srv.httpsRequiredMessage(); !strings.Contains(msg, "https") {
		t.Fatalf("message must explain the https requirement: %q", msg)
	}
}

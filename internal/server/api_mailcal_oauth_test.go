package server

import (
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
		if rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/mailcal?oauth=") {
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

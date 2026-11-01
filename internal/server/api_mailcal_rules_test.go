package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
)

func TestMailcalRulesAndProposalsOverHTTP(t *testing.T) {
	srv := newTestServer(t)
	status, env := call(t, srv, http.MethodPost, "/api/mailcal/accounts", `{"label":"Work","plugin_id":"refmem","kind":"both","email":"me@example.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("account: %d %v", status, env)
	}
	acct := dataOf(t, env)["id"].(string)

	// The master switch starts off.
	_, env = call(t, srv, http.MethodGet, "/api/mailcal/settings", "")
	if dataOf(t, env)["enabled"] != false {
		t.Fatalf("writes must start disabled: %v", env)
	}
	if status, _ = call(t, srv, http.MethodPut, "/api/mailcal/settings", `{"enabled":true}`); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}

	// Dangerous rules are refused with a 400.
	for _, body := range []string{
		`{"account_id":"` + acct + `","action":"reply","effect":"allow","enabled":true,"match":{"recipients":{"domains":["*"]},"body":{"mode":"free"}},"limits":{"per_hour":1,"per_day":1}}`,
		`{"account_id":"` + acct + `","action":"reply","effect":"allow","enabled":true,"match":{"recipients":{"domains":["gmail.com"]},"body":{"mode":"free"}},"limits":{"per_hour":1,"per_day":1}}`,
	} {
		if status, env = call(t, srv, http.MethodPost, "/api/mailcal/rules", body); status != http.StatusBadRequest {
			t.Fatalf("dangerous rule accepted: %d %v", status, env)
		}
	}

	// A draft, evaluated: no rule yet, so it needs approval and says why.
	d, err := srv.mailcal.CreateDraft(context.Background(), acct, "agent", "", mailcal.DraftPayload{
		To: []abi.Address{{Email: "alice@partner.com"}}, Subject: "Re: Q3 invoice", BodyText: "Received, thank you.", InReplyTo: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	_, env = call(t, srv, http.MethodGet, "/api/mailcal/drafts/"+d.ID+"/evaluation", "")
	if dataOf(t, env)["verdict"] != "needs_approval" {
		t.Fatalf("evaluation: %v", env)
	}

	// "Approve and create a rule" makes a pending proposal, not a rule.
	status, env = call(t, srv, http.MethodPost, "/api/mailcal/drafts/"+d.ID+"/rule-proposal", "")
	if status != http.StatusCreated {
		t.Fatalf("proposal: %d %v", status, env)
	}
	propID := dataOf(t, env)["id"].(string)
	_, env = call(t, srv, http.MethodGet, "/api/mailcal/rules", "")
	if dataOf(t, env)["count"].(float64) != 0 {
		t.Fatalf("a proposal must not be a rule: %v", env)
	}
	if status, env = call(t, srv, http.MethodPost, "/api/mailcal/proposals/"+propID+"/approve", ""); status != http.StatusOK {
		t.Fatalf("approve: %d %v", status, env)
	}
	if status, _ = call(t, srv, http.MethodPost, "/api/mailcal/proposals/"+propID+"/approve", ""); status != http.StatusBadRequest {
		t.Fatalf("second approval must fail, got %d", status)
	}
	_, env = call(t, srv, http.MethodGet, "/api/mailcal/rules", "")
	if dataOf(t, env)["count"].(float64) != 1 {
		t.Fatalf("approved proposal should be one rule: %v", env)
	}
	_, env = call(t, srv, http.MethodGet, "/api/mailcal/drafts/"+d.ID+"/evaluation", "")
	if v := dataOf(t, env); v["verdict"] != "allow" {
		t.Fatalf("the same draft should now be allowed: %v", env)
	}
}

package policy

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)

func zeroUsage(string, time.Duration) (int, error) { return 0, nil }

func replyRule() Rule {
	return Rule{
		ID: "r1", Enabled: true, AccountID: "a1", Action: ActionReply, Effect: EffectAllow,
		Match: Match{
			Recipients: Recipients{Addresses: []string{"Boss@Partner.com"}},
			Body:       BodyMatch{Mode: BodyTemplate, Template: "Thanks, received."},
		},
		Limits: Limits{PerHour: 2, PerDay: 5},
	}
}

func mustValid(t *testing.T, r Rule) Rule {
	t.Helper()
	if err := r.Validate(); err != nil {
		t.Fatalf("rule should be valid: %v", err)
	}
	return r
}

func goodReply() Request {
	return Request{
		AccountID: "a1", Action: ActionReply, To: []string{"boss@partner.com"},
		Subject: "Re: plan", Body: "Thanks, received.",
		Reply: &ReplyFacts{Verified: true, Participants: []string{"boss@partner.com", "me@example.com"}},
	}
}

func TestAllowsExactTemplateReply(t *testing.T) {
	d := Evaluate([]Rule{mustValid(t, replyRule())}, goodReply(), now, zeroUsage)
	if d.Verdict != VerdictAllow || d.RuleID != "r1" {
		t.Fatalf("want allow by r1, got %+v", d)
	}
}

func TestDefaultIsNeedsApprovalWithoutRules(t *testing.T) {
	d := Evaluate(nil, goodReply(), now, zeroUsage)
	if d.Verdict != VerdictNeedsApproval || len(d.Reasons) == 0 {
		t.Fatalf("got %+v", d)
	}
}

func TestAdversarialRequestsNeverAllow(t *testing.T) {
	rules := []Rule{mustValid(t, replyRule())}
	cases := map[string]func(*Request){
		"cyrillic lookalike domain": func(r *Request) { r.To = []string{"boss@раrtner.com"} },
		"lookalike local part":      func(r *Request) { r.To = []string{"b0ss@partner.com"} },
		"subdomain":                 func(r *Request) { r.To = []string{"boss@mail.partner.com"} },
		"suffix trick":              func(r *Request) { r.To = []string{"boss@evilpartner.com"} },
		"extra recipient in cc":     func(r *Request) { r.Cc = []string{"attacker@evil.com"} },
		"bcc present":               func(r *Request) { r.Bcc = []string{"boss@partner.com"} },
		"display name smuggling":    func(r *Request) { r.To = []string{"boss@partner.com <attacker@evil.com>"} },
		"list in one address":       func(r *Request) { r.To = []string{"boss@partner.com,attacker@evil.com"} },
		"attachment":                func(r *Request) { r.Attachments = 1 },
		"forward by subject":        func(r *Request) { r.Subject = "Fwd: secrets" },
		"forward by body":           func(r *Request) { r.Body = "Thanks, received.\n---------- Forwarded message ----------" },
		"different text":            func(r *Request) { r.Body = "Thanks, received. Password is hunter2" },
		"not a verified reply":      func(r *Request) { r.Reply.Verified = false },
		"no reply facts":            func(r *Request) { r.Reply = nil },
		"recipient not in thread":   func(r *Request) { r.Reply.Participants = []string{"me@example.com"} },
		"other account":             func(r *Request) { r.AccountID = "a2" },
		"wrong action":              func(r *Request) { r.Action = ActionSendNew },
		"unparseable recipient":     func(r *Request) { r.To = []string{"not an email"} },
		"empty recipients":          func(r *Request) { r.To = nil },
		"destructive":               func(r *Request) { r.Destructive = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := goodReply()
			mutate(&req)
			d := Evaluate(rules, req, now, zeroUsage)
			if d.Verdict == VerdictAllow {
				t.Fatalf("must not allow, got %+v", d)
			}
		})
	}
}

func TestCaseAndIDNNormalization(t *testing.T) {
	r := replyRule()
	r.Match.Recipients.Addresses = []string{"Boss@Bücher.example"}
	rules := []Rule{mustValid(t, r)}
	req := goodReply()
	req.To = []string{"BOSS@xn--bcher-kva.example"}
	req.Reply.Participants = []string{"boss@bücher.example"}
	if d := Evaluate(rules, req, now, zeroUsage); d.Verdict != VerdictAllow {
		t.Fatalf("the same mailbox written differently should match: %+v", d)
	}
}

func TestDenyBeatsAllow(t *testing.T) {
	deny := Rule{ID: "d1", Enabled: true, AccountID: "a1", Action: ActionReply, Effect: EffectDeny,
		Match: Match{Recipients: Recipients{Domains: []string{"partner.com"}}}}
	d := Evaluate([]Rule{mustValid(t, replyRule()), mustValid(t, deny)}, goodReply(), now, zeroUsage)
	if d.Verdict != VerdictDeny || d.RuleID != "d1" {
		t.Fatalf("got %+v", d)
	}
	// A deny rule also catches a Bcc recipient.
	req := goodReply()
	req.To = []string{"someone@else.com"}
	req.Bcc = []string{"x@partner.com"}
	if d := Evaluate([]Rule{mustValid(t, deny)}, req, now, zeroUsage); d.Verdict != VerdictDeny {
		t.Fatalf("deny should see Bcc: %+v", d)
	}
}

func TestRuleLifecycle(t *testing.T) {
	disabled := replyRule()
	disabled.Enabled = false
	past := replyRule()
	exp := now.Add(-time.Second)
	past.ExpiresAt = &exp
	for name, r := range map[string]Rule{"disabled": disabled, "expired": past} {
		if d := Evaluate([]Rule{mustValid(t, r)}, goodReply(), now, zeroUsage); d.Verdict != VerdictNeedsApproval {
			t.Errorf("%s rule must not apply: %+v", name, d)
		}
	}
}

func TestLimitsAndUnknownUsageFailClosed(t *testing.T) {
	rules := []Rule{mustValid(t, replyRule())}
	hourFull := func(_ string, w time.Duration) (int, error) {
		if w == time.Hour {
			return 2, nil
		}
		return 2, nil
	}
	if d := Evaluate(rules, goodReply(), now, hourFull); d.Verdict != VerdictNeedsApproval {
		t.Fatalf("limit reached must ask: %+v", d)
	}
	broken := func(string, time.Duration) (int, error) { return 0, errors.New("db down") }
	if d := Evaluate(rules, goodReply(), now, broken); d.Verdict != VerdictNeedsApproval {
		t.Fatalf("usage error must ask: %+v", d)
	}
	if d := Evaluate(rules, goodReply(), now, nil); d.Verdict != VerdictNeedsApproval {
		t.Fatalf("nil usage must ask: %+v", d)
	}
}

func TestLinksBlockedUnlessAllowed(t *testing.T) {
	r := replyRule()
	r.Match.Body = BodyMatch{Mode: BodyFree}
	rules := []Rule{mustValid(t, r)}
	req := goodReply()
	req.Body = "see https://evil.example/x"
	if d := Evaluate(rules, req, now, zeroUsage); d.Verdict == VerdictAllow {
		t.Fatalf("link must block: %+v", d)
	}
	req.Body = "see www.evil.example"
	if d := Evaluate(rules, req, now, zeroUsage); d.Verdict == VerdictAllow {
		t.Fatalf("www link must block: %+v", d)
	}
	req.Body = "Sounds good, talk tomorrow."
	if d := Evaluate(rules, req, now, zeroUsage); d.Verdict != VerdictAllow {
		t.Fatalf("plain free text should pass: %+v", d)
	}
	r.Match.Body.AllowLinks = true
	req.Body = "see https://docs.partner.com/x"
	if d := Evaluate([]Rule{mustValid(t, r)}, req, now, zeroUsage); d.Verdict != VerdictAllow {
		t.Fatalf("explicit allow_links should pass: %+v", d)
	}
}

func TestValidateRejectsDangerousRules(t *testing.T) {
	mk := func(mut func(*Rule)) error {
		r := replyRule()
		mut(&r)
		return r.Validate()
	}
	bad := map[string]func(*Rule){
		"wildcard address":  func(r *Rule) { r.Match.Recipients.Addresses = []string{"*@partner.com"} },
		"star domain":       func(r *Rule) { r.Match.Recipients.Domains = []string{"*"} },
		"star-dot domain":   func(r *Rule) { r.Match.Recipients.Domains = []string{"*.partner.com"} },
		"tld only":          func(r *Rule) { r.Match.Recipients.Domains = []string{"com"} },
		"public domain":     func(r *Rule) { r.Match.Recipients.Domains = []string{"gmail.com"} },
		"public via IDN":    func(r *Rule) { r.Match.Recipients.Domains = []string{"GMAIL.com"} },
		"nobody":            func(r *Rule) { r.Match.Recipients = Recipients{} },
		"no limits":         func(r *Rule) { r.Limits = Limits{} },
		"template no text":  func(r *Rule) { r.Match.Body = BodyMatch{Mode: BodyTemplate} },
		"unknown body mode": func(r *Rule) { r.Match.Body = BodyMatch{Mode: "x"} },
		"no account":        func(r *Rule) { r.AccountID = "" },
		"bad action":        func(r *Rule) { r.Action = "delete" },
		"bad effect":        func(r *Rule) { r.Effect = "maybe" },
	}
	for name, mut := range bad {
		if err := mk(mut); !errors.Is(err, ErrInvalidRule) {
			t.Errorf("%s: want ErrInvalidRule, got %v", name, err)
		}
	}
	if err := mk(func(r *Rule) {
		r.AllowPublicDomain = true
		r.Match.Recipients = Recipients{Domains: []string{"gmail.com"}}
	}); err != nil {
		t.Errorf("explicit allow_public_domain should be accepted: %v", err)
	}
}

func TestReplyRuleForcesReplyOnly(t *testing.T) {
	r := replyRule()
	r.Match.ReplyOnly = false
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if !r.Match.ReplyOnly {
		t.Fatal("a reply rule must be reply-only")
	}
}

func calRule() Rule {
	return Rule{ID: "c1", Enabled: true, AccountID: "a1", Action: ActionCalendarCreate, Effect: EffectAllow,
		Match:  Match{Calendar: &CalendarMatch{WorkingHours: "09:00-18:00", MaxMinutes: 60, Domains: []string{"partner.com"}}},
		Limits: Limits{PerHour: 3, PerDay: 10}}
}

func calReq(start, end time.Time, attendees ...string) Request {
	return Request{AccountID: "a1", Action: ActionCalendarCreate, Calendar: &CalendarFacts{Start: start, End: end, Attendees: attendees}}
}

func TestCalendarCreate(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	rules := []Rule{mustValid(t, calRule())}
	at := func(h, m int) time.Time { return time.Date(2026, 11, 3, h, m, 0, 0, loc) }

	if d := Evaluate(rules, calReq(at(10, 0), at(11, 0), "a@partner.com"), now, zeroUsage); d.Verdict != VerdictAllow {
		t.Fatalf("should allow: %+v", d)
	}
	bad := map[string]Request{
		"too long":          calReq(at(10, 0), at(11, 1), "a@partner.com"),
		"before hours":      calReq(at(8, 0), at(8, 30), "a@partner.com"),
		"after hours":       calReq(at(17, 30), at(18, 30), "a@partner.com"),
		"outsider attendee": calReq(at(10, 0), at(10, 30), "a@partner.com", "x@evil.com"),
		"ends before start": calReq(at(11, 0), at(10, 0), "a@partner.com"),
		"crosses midnight":  calReq(at(17, 0), at(17, 0).Add(25*time.Hour), "a@partner.com"),
		"no facts":          {AccountID: "a1", Action: ActionCalendarCreate},
	}
	for name, req := range bad {
		if d := Evaluate(rules, req, now, zeroUsage); d.Verdict == VerdictAllow {
			t.Errorf("%s must not allow: %+v", name, d)
		}
	}
}

func TestCalendarAcceptChecksOrganizer(t *testing.T) {
	r := Rule{ID: "c2", Enabled: true, AccountID: "a1", Action: ActionCalendarAccept, Effect: EffectAllow,
		Match:  Match{Calendar: &CalendarMatch{Domains: []string{"partner.com"}}},
		Limits: Limits{PerHour: 3, PerDay: 10}}
	rules := []Rule{mustValid(t, r)}
	ok := Request{AccountID: "a1", Action: ActionCalendarAccept, Calendar: &CalendarFacts{Organizer: "boss@partner.com"}}
	if d := Evaluate(rules, ok, now, zeroUsage); d.Verdict != VerdictAllow {
		t.Fatalf("got %+v", d)
	}
	ok.Calendar.Organizer = "boss@evil.com"
	if d := Evaluate(rules, ok, now, zeroUsage); d.Verdict == VerdictAllow {
		t.Fatalf("outside organizer must not pass: %+v", d)
	}
	ok.Calendar.Organizer = ""
	if d := Evaluate(rules, ok, now, zeroUsage); d.Verdict == VerdictAllow {
		t.Fatalf("unknown organizer must not pass: %+v", d)
	}
}

func FuzzEvaluateNeverPanicsAndNeverAllowsUnlistedRecipient(f *testing.F) {
	f.Add("boss@partner.com", "Thanks, received.")
	f.Add("x@evil.com", "hi")
	f.Add("бoss@partner.com", "")
	rules := []Rule{replyRule()}
	if err := rules[0].Validate(); err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, to, body string) {
		req := goodReply()
		req.To = []string{to}
		req.Body = body
		d := Evaluate(rules, req, now, zeroUsage)
		if d.Verdict == VerdictAllow {
			norm, err := normList([]string{to})
			if err != nil || len(norm) != 1 || norm[0] != "boss@partner.com" {
				t.Fatalf("allowed recipient %q (normalized %v)", to, norm)
			}
		}
	})
}

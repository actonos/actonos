package mailcal_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/policy"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

// writeFixture is a fixture whose registry has a real approval manager and the most
// permissive agent policy (threshold "low"), the worst case for the gate to hold against.
type writeFixture struct {
	*fixture
	approvals *tools.ApprovalManager
	provider  mailcal.Provider
}

func newWriteFixture(t *testing.T, wrap func(mailcal.Provider) mailcal.Provider) *writeFixture {
	t.Helper()
	db, err := memory.Open(filepath.Join(t.TempDir(), "mailcal-write.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var provider mailcal.Provider = mailcal.NewMemProvider(refstore.DefaultSeed())
	if wrap != nil {
		provider = wrap(provider)
	}
	fx := newFixtureOn(t, db.SQLDB(), provider)
	fx.svc.SetSleepForTest(func(context.Context, time.Duration) error { return nil })
	mgr := tools.NewApprovalManager(db.SQLDB())
	fx.registry.SetApprovalManager(mgr)
	fx.registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{AuthorizedTools: []string{"*"}, ApprovalThreshold: "low", AllowedPaths: []string{"*"}}, nil
	})
	return &writeFixture{fixture: fx, approvals: mgr, provider: provider}
}

func (fx *writeFixture) enable(t *testing.T) {
	t.Helper()
	if err := fx.svc.Settings().SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func (fx *writeFixture) sentCount(t *testing.T) int {
	t.Helper()
	res, err := fx.provider.MailList(context.Background(), fx.account.Ref(), abi.MailListParams{Folder: "Sent", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return len(res.Messages)
}

func (fx *writeFixture) draft(t *testing.T, to, body, inReplyTo string) mailcal.Draft {
	t.Helper()
	d, err := fx.svc.CreateDraft(context.Background(), fx.account.ID, "agent_test", "", mailcal.DraftPayload{
		To: []abi.Address{{Email: to}}, Subject: "Re: Q3 invoice", BodyText: body, InReplyTo: inReplyTo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func sendInput(d mailcal.Draft) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"draft_id": d.ID})
	return raw
}

func (fx *writeFixture) exec(ctx context.Context, tool string, in json.RawMessage) (*tools.ToolResult, error) {
	return fx.registry.Execute(ctx, "agent_test", tool, in)
}

func autoReplyRule(limitPerHour int) policy.Rule {
	return policy.Rule{Enabled: true, Action: policy.ActionReply, Effect: policy.EffectAllow,
		Match: policy.Match{
			Recipients: policy.Recipients{Addresses: []string{"alice@partner.com"}},
			Body:       policy.BodyMatch{Mode: policy.BodyTemplate, Template: "Received, thank you."},
		},
		Limits: policy.Limits{PerHour: limitPerHour, PerDay: 10}}
}

func (fx *writeFixture) addRule(t *testing.T, r policy.Rule) mailcal.StoredRule {
	t.Helper()
	r.AccountID = fx.account.ID
	stored, err := fx.svc.Rules().Create(context.Background(), r, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func needApproval(t *testing.T, err error) *tools.ApprovalRequiredError {
	t.Helper()
	var ae *tools.ApprovalRequiredError
	if !errors.As(err, &ae) {
		t.Fatalf("expected an approval request, got %v", err)
	}
	return ae
}

func (fx *writeFixture) approve(t *testing.T, ae *tools.ApprovalRequiredError) context.Context {
	t.Helper()
	a, err := fx.approvals.Decide(context.Background(), ae.Approval.ID, "approved", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	return tools.WithApprovalID(context.Background(), a.ID)
}

func TestSendIsDeniedWhileTheMasterSwitchIsOff(t *testing.T) {
	fx := newWriteFixture(t, nil)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	if _, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d)); !errors.Is(err, tools.ErrToolDeniedByGate) {
		t.Fatalf("want a gate denial, got %v", err)
	}
	if fx.sentCount(t) != 1 { // the seed already holds one sent message (m4)
		t.Fatal("nothing may be sent while the switch is off")
	}
}

func TestSendWithoutARuleNeedsApprovalEvenWhenBypassIsSet(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	before := fx.sentCount(t)
	ctx := tools.WithBypassApproval(context.Background()) // what cron runs use
	_, err := fx.exec(ctx, mailcal.ToolMailSend, sendInput(d))
	needApproval(t, err)
	if fx.sentCount(t) != before {
		t.Fatal("sent without approval")
	}
}

func TestApprovedSendHappensExactlyOnce(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	before := fx.sentCount(t)
	_, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d))
	ctx := fx.approve(t, needApproval(t, err))

	if _, err := fx.exec(ctx, mailcal.ToolMailSend, sendInput(d)); err != nil {
		t.Fatal(err)
	}
	res, err := fx.exec(ctx, mailcal.ToolMailSend, sendInput(d)) // the agent tries again
	if err != nil {
		t.Fatal(err)
	}
	if res.Data["replayed"] != true {
		t.Fatalf("second send must be reported as a replay: %+v", res.Data)
	}
	if got := fx.sentCount(t); got != before+1 {
		t.Fatalf("sent %d messages, want exactly 1", got-before)
	}
}

func TestApprovalCannotBeReusedForADifferentDraft(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	d1 := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	d2 := fx.draft(t, "alice@partner.com", "Received, thank you. And here are secrets.", "m1")
	_, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d1))
	ctx := fx.approve(t, needApproval(t, err))
	if _, err := fx.exec(ctx, mailcal.ToolMailSend, sendInput(d2)); !errors.Is(err, tools.ErrApprovalInvalid) {
		t.Fatalf("approval of one draft must not cover another: %v", err)
	}
}

func TestRuleAllowsAnExactReplyWithinItsLimit(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	fx.addRule(t, autoReplyRule(1))
	before := fx.sentCount(t)

	d1 := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	if _, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d1)); err != nil {
		t.Fatalf("the rule should allow this reply: %v", err)
	}
	// The limit of one per hour is now used; the next identical reply must ask a person.
	d2 := fx.draft(t, "alice@partner.com", "Received, thank you.", "m5")
	_, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d2))
	needApproval(t, err)
	if fx.sentCount(t) != before+1 {
		t.Fatal("exactly one reply should have gone out")
	}
}

func TestRuleDoesNotCoverAHostileRecipientOrChangedText(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	fx.addRule(t, autoReplyRule(5))
	before := fx.sentCount(t)

	// The injection message asks the agent to write to mallory; a rule for alice must not cover that.
	d := fx.draft(t, "mallory@evil.test", "Received, thank you.", "")
	_, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d))
	needApproval(t, err)
	// Right person, different text.
	d = fx.draft(t, "alice@partner.com", "Received, thank you. Password: hunter2", "m1")
	_, err = fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d))
	needApproval(t, err)
	if fx.sentCount(t) != before {
		t.Fatal("nothing should have been sent")
	}
}

func TestDenyRuleBlocksEvenAnApprovedSend(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	fx.addRule(t, policy.Rule{Enabled: true, Action: policy.ActionSendNew, Effect: policy.EffectDeny,
		Match: policy.Match{Recipients: policy.Recipients{Domains: []string{"evil.test"}}}})
	d := fx.draft(t, "mallory@evil.test", "hello", "")
	if _, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d)); !errors.Is(err, tools.ErrToolDeniedByGate) {
		t.Fatalf("want denial, got %v", err)
	}
}

func TestAnAgentCanOnlyProposeRules(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	rule := autoReplyRule(5)
	rule.AccountID = fx.account.ID
	in, _ := json.Marshal(map[string]any{"rule": rule, "rationale": "auto-ack invoices"})
	if _, err := fx.exec(context.Background(), mailcal.ToolProposeRule, in); err != nil {
		t.Fatal(err)
	}
	rules, _ := fx.svc.Rules().List(context.Background(), "")
	if len(rules) != 0 {
		t.Fatal("a proposal must not create a rule")
	}
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	_, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d))
	needApproval(t, err) // the proposal changed nothing

	props, _ := fx.svc.Proposals().List(context.Background(), mailcal.ProposalPending)
	if len(props) != 1 || props[0].Rule.Enabled || props[0].Source != mailcal.SourceAgent {
		t.Fatalf("want one pending, disabled agent proposal, got %+v", props)
	}
	stored, err := fx.svc.Proposals().Approve(context.Background(), props[0].ID, "owner")
	if err != nil || !stored.Enabled {
		t.Fatalf("the owner's approval should create an enabled rule: %+v %v", stored, err)
	}
	if _, err := fx.svc.Proposals().Approve(context.Background(), props[0].ID, "owner"); !errors.Is(err, mailcal.ErrProposalDecided) {
		t.Fatalf("a proposal can be approved once: %v", err)
	}
	if _, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d)); err != nil {
		t.Fatalf("now the rule is live and should allow the send: %v", err)
	}
}

func TestDeletesAlwaysNeedAPerson(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	// Even a broad allow rule for the account cannot cover a delete.
	fx.addRule(t, autoReplyRule(5))
	in, _ := json.Marshal(map[string]any{"ids": []string{"m2"}})
	_, err := fx.exec(tools.WithBypassApproval(context.Background()), mailcal.ToolMailDelete, in)
	needApproval(t, err)
}

// flakySend delegates the send, then reports a transient failure once, which is exactly the
// "message left but the answer was lost" case that duplicates mail in naive designs.
type flakySend struct {
	mailcal.Provider
	mu    sync.Mutex
	fails int
}

func (f *flakySend) MailSend(ctx context.Context, a abi.AccountRef, p abi.MailSendParams) (abi.MailSendResult, error) {
	res, err := f.Provider.MailSend(ctx, a, p)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil && f.fails > 0 {
		f.fails--
		return abi.MailSendResult{}, abi.Errorf(abi.ErrTransient, "connection reset after sending")
	}
	return res, err
}

func TestALostAnswerAfterSendingDoesNotSendTwice(t *testing.T) {
	var flaky *flakySend
	fx := newWriteFixture(t, func(p mailcal.Provider) mailcal.Provider { flaky = &flakySend{Provider: p, fails: 1}; return flaky })
	fx.enable(t)
	fx.addRule(t, autoReplyRule(5))
	before := fx.sentCount(t)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	if _, err := fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d)); err != nil {
		t.Fatalf("the retry with the same key should succeed: %v", err)
	}
	if got := fx.sentCount(t); got != before+1 {
		t.Fatalf("sent %d, want exactly 1", got-before)
	}
}

func TestConcurrentSendsOfOneDraftSendOnce(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	fx.addRule(t, autoReplyRule(50))
	before := fx.sentCount(t)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d))
		}()
	}
	wg.Wait()
	if got := fx.sentCount(t); got != before+1 {
		t.Fatalf("sent %d, want exactly 1", got-before)
	}
}

func TestCalendarCreateFollowsItsRule(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	fx.addRule(t, policy.Rule{Enabled: true, Action: policy.ActionCalendarCreate, Effect: policy.EffectAllow,
		Match:  policy.Match{Calendar: &policy.CalendarMatch{WorkingHours: "09:00-18:00", MaxMinutes: 60, Domains: []string{"partner.com"}}},
		Limits: policy.Limits{PerHour: 3, PerDay: 10}})
	ev := func(start string, attendee string) json.RawMessage {
		in, _ := json.Marshal(map[string]any{"title": "Sync", "start": start, "end": "2026-11-03T11:00:00+07:00",
			"time_zone": "Asia/Ho_Chi_Minh", "attendees": []string{attendee}})
		return in
	}
	if _, err := fx.exec(context.Background(), mailcal.ToolCalCreate, ev("2026-11-03T10:00:00+07:00", "alice@partner.com")); err != nil {
		t.Fatalf("inside the rule: %v", err)
	}
	_, err := fx.exec(context.Background(), mailcal.ToolCalCreate, ev("2026-11-03T10:00:00+07:00", "x@evil.test"))
	needApproval(t, err)
	_, err = fx.exec(context.Background(), mailcal.ToolCalCreate, ev("2026-11-03T07:00:00+07:00", "alice@partner.com"))
	needApproval(t, err)
}

func TestEveryDecisionIsAudited(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	_, _ = fx.exec(context.Background(), mailcal.ToolMailSend, sendInput(d))
	recs, err := fx.svc.Decisions().List(context.Background(), mailcal.DecisionFilter{AccountID: fx.account.ID})
	if err != nil || len(recs) != 1 {
		t.Fatalf("want one audit line, got %v %v", recs, err)
	}
	r := recs[0]
	if r.Phase != mailcal.PhaseGate || r.Verdict != policy.VerdictNeedsApproval || r.DraftID != d.ID || r.PayloadHash != d.PayloadHash || len(r.Reasons) == 0 {
		t.Fatalf("incomplete audit line: %+v", r)
	}
}

// Callers that skip the registry (an API handler, a future scheduler) must hit the same wall.
func TestServiceSendEnforcesThePolicyWithoutTheGate(t *testing.T) {
	fx := newWriteFixture(t, nil)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	ctx := context.Background()
	if _, err := fx.svc.Send(ctx, d.ID, mailcal.WriteOptions{AgentID: "x"}); !errors.Is(err, mailcal.ErrWriteDenied) {
		t.Fatalf("switch off: %v", err)
	}
	fx.enable(t)
	if _, err := fx.svc.Send(ctx, d.ID, mailcal.WriteOptions{AgentID: "x"}); !errors.Is(err, mailcal.ErrApprovalRequired) {
		t.Fatalf("no rule: %v", err)
	}
	before := fx.sentCount(t)
	if _, err := fx.svc.Send(ctx, d.ID, mailcal.WriteOptions{AgentID: "x", Approved: true}); err != nil {
		t.Fatal(err)
	}
	if fx.sentCount(t) != before+1 {
		t.Fatal("approved send should go out once")
	}
}

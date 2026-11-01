package mailcal_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refstore"
)

// flaky wraps a Provider and fails MailList a set number of times first.
type flaky struct {
	mailcal.Provider
	failures int32
	err      *abi.Error
	calls    atomic.Int32
}

func (f *flaky) MailList(ctx context.Context, a abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error) {
	f.calls.Add(1)
	if f.calls.Load() <= f.failures {
		return abi.MailListResult{}, f.err
	}
	return f.Provider.MailList(ctx, a, p)
}

func TestResolveRequiresAnAccountWhenSeveralFit(t *testing.T) {
	fx := newFixture(t, nil)
	ctx := context.Background()
	if _, err := fx.svc.Accounts().Create(ctx, mailcal.NewAccount{Label: "Second", PluginID: "refmem", Kind: mailcal.KindMail}); err != nil {
		t.Fatal(err)
	}
	_, _, err := fx.svc.MailList(ctx, "", abi.MailListParams{})
	if err == nil || !strings.Contains(err.Error(), "several mail accounts") {
		t.Fatalf("an ambiguous default must be refused with the choices listed, got %v", err)
	}
	if _, res, err := fx.svc.MailList(ctx, fx.account.ID, abi.MailListParams{}); err != nil || len(res.Messages) != 4 {
		t.Fatalf("explicit account: %v %v", res, err)
	}
}

func TestResolveRespectsAccountKindAndStatus(t *testing.T) {
	fx := newFixture(t, nil)
	ctx := context.Background()
	mailOnly, _ := fx.svc.Accounts().Create(ctx, mailcal.NewAccount{Label: "M", PluginID: "refmem", Kind: mailcal.KindMail})
	if _, _, err := fx.svc.CalendarList(ctx, mailOnly.ID, abi.CalListParams{From: time.Now(), To: time.Now().Add(time.Hour)}); err == nil ||
		!strings.Contains(err.Error(), "does not serve calendar") {
		t.Fatalf("a mail-only account must not serve calendar calls: %v", err)
	}
	_ = fx.svc.Accounts().SetStatus(ctx, fx.account.ID, mailcal.StatusPaused, "operator paused")
	if _, _, err := fx.svc.MailList(ctx, fx.account.ID, abi.MailListParams{}); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("a paused account must refuse work: %v", err)
	}
	if _, _, err := fx.svc.MailList(ctx, "acct_nope", abi.MailListParams{}); !errors.Is(err, mailcal.ErrAccountNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
}

func TestMissingProviderPluginIsReportedClearly(t *testing.T) {
	fx := newFixture(t, nil)
	fx.svc.PluginDeactivated(context.Background(), "refmem")
	_, _, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{})
	if err == nil || !strings.Contains(err.Error(), "not loaded or is disabled") {
		t.Fatalf("got %v", err)
	}
	if len(fx.svc.Providers()) != 0 {
		t.Fatal("deactivated provider is still listed")
	}
}

func TestTransientReadErrorsAreRetriedWithBackoff(t *testing.T) {
	inner := mailcal.NewMemProvider(refstore.DefaultSeed())
	f := &flaky{Provider: inner, failures: 2, err: abi.Errorf(abi.ErrTransient, "connection reset")}
	fx := newFixture(t, f)
	var waits []time.Duration
	fx.svc.SetSleepForTest(func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil })

	_, res, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{})
	if err != nil || len(res.Messages) != 4 {
		t.Fatalf("expected success after two retries: %v %v", res, err)
	}
	if f.calls.Load() != 3 || len(waits) != 2 || waits[0] >= waits[1] {
		t.Fatalf("calls=%d waits=%v; expected 3 calls with growing back-off", f.calls.Load(), waits)
	}
}

func TestPersistentTransientErrorGivesUp(t *testing.T) {
	f := &flaky{Provider: mailcal.NewMemProvider(refstore.DefaultSeed()), failures: 99, err: abi.Errorf(abi.ErrTransient, "down")}
	fx := newFixture(t, f)
	fx.svc.SetSleepForTest(func(context.Context, time.Duration) error { return nil })
	_, _, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{})
	if !mailcal.IsCode(err, abi.ErrTransient) || f.calls.Load() != 3 {
		t.Fatalf("calls=%d err=%v; want 3 attempts then the transient error", f.calls.Load(), err)
	}
}

func TestRateLimitHonoursShortRetryAfterButNotLongOnes(t *testing.T) {
	short := &flaky{Provider: mailcal.NewMemProvider(refstore.DefaultSeed()), failures: 1,
		err: &abi.Error{Code: abi.ErrRateLimited, RetryAfterMS: 1500}}
	fx := newFixture(t, short)
	var waited time.Duration
	fx.svc.SetSleepForTest(func(_ context.Context, d time.Duration) error { waited = d; return nil })
	if _, _, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{}); err != nil {
		t.Fatalf("short rate limit should be waited out: %v", err)
	}
	if waited != 1500*time.Millisecond {
		t.Fatalf("waited %v, want the provider's 1.5s", waited)
	}

	long := &flaky{Provider: mailcal.NewMemProvider(refstore.DefaultSeed()), failures: 99,
		err: &abi.Error{Code: abi.ErrRateLimited, RetryAfterMS: 60000}}
	fx2 := newFixture(t, long)
	fx2.svc.SetSleepForTest(func(context.Context, time.Duration) error {
		t.Error("must not sleep for a minute inside a tool call")
		return nil
	})
	if _, _, err := fx2.svc.MailList(context.Background(), fx2.account.ID, abi.MailListParams{}); !mailcal.IsCode(err, abi.ErrRateLimited) {
		t.Fatalf("a long rate limit must be reported, not waited: %v", err)
	}
}

func TestAuthExpiredMarksTheAccountAndStopsFurtherCalls(t *testing.T) {
	f := &flaky{Provider: mailcal.NewMemProvider(refstore.DefaultSeed()), failures: 99, err: abi.Errorf(abi.ErrAuthExpired, "token revoked")}
	fx := newFixture(t, f)
	ctx := context.Background()
	if _, _, err := fx.svc.MailList(ctx, fx.account.ID, abi.MailListParams{}); !mailcal.IsCode(err, abi.ErrAuthExpired) {
		t.Fatalf("got %v", err)
	}
	got, _ := fx.svc.Accounts().Get(ctx, fx.account.ID)
	if got.Status != mailcal.StatusAuthExpired || got.StatusDetail != "token revoked" {
		t.Fatalf("account not marked: %+v", got)
	}
	before := f.calls.Load()
	if _, _, err := fx.svc.MailList(ctx, fx.account.ID, abi.MailListParams{}); err == nil || !strings.Contains(err.Error(), "auth_expired") {
		t.Fatalf("later calls must be refused locally: %v", err)
	}
	if f.calls.Load() != before {
		t.Fatal("an expired account must not keep hammering the provider")
	}
}

func TestNonRetryableErrorsAreNotRetried(t *testing.T) {
	f := &flaky{Provider: mailcal.NewMemProvider(refstore.DefaultSeed()), failures: 99, err: abi.Errorf(abi.ErrInvalid, "bad cursor")}
	fx := newFixture(t, f)
	fx.svc.SetSleepForTest(func(context.Context, time.Duration) error { t.Error("no back-off for invalid requests"); return nil })
	_, _, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{})
	if !mailcal.IsCode(err, abi.ErrInvalid) || f.calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", f.calls.Load(), err)
	}
}

func TestCreateDraftVerifiesTheThreadAndDerivesRecipientContext(t *testing.T) {
	fx := newFixture(t, nil)
	ctx := context.Background()
	d, err := fx.svc.CreateDraft(ctx, fx.account.ID, "agent1", "trace1", mailcal.DraftPayload{
		To: []abi.Address{{Email: "alice@partner.com"}}, Subject: "Re: Q3 invoice", BodyText: "Paid.", InReplyTo: "m5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Reply == nil || !d.Reply.Verified || d.Reply.ThreadID != "t1" || d.Payload.ThreadID != "t1" {
		t.Fatalf("reply context missing: %+v", d)
	}
	want := []string{"alice@partner.com", "me@example.com"}
	if strings.Join(d.Reply.Participants, ",") != strings.Join(want, ",") || d.Reply.SenderOfLast != "alice@partner.com" {
		t.Fatalf("participants=%v sender=%q", d.Reply.Participants, d.Reply.SenderOfLast)
	}
}

func TestCreateDraftRejectsUnverifiableReplies(t *testing.T) {
	fx := newFixture(t, nil)
	ctx := context.Background()
	p := mailcal.DraftPayload{To: []abi.Address{{Email: "alice@partner.com"}}, Subject: "s", BodyText: "b"}

	p.InReplyTo = "no-such-message"
	if _, err := fx.svc.CreateDraft(ctx, fx.account.ID, "a", "", p); !errors.Is(err, mailcal.ErrReplyUnverified) {
		t.Fatalf("a reply to a message that does not exist must be refused: %v", err)
	}
	p.InReplyTo, p.ThreadID = "", "t1"
	if _, err := fx.svc.CreateDraft(ctx, fx.account.ID, "a", "", p); !errors.Is(err, mailcal.ErrInvalidDraft) {
		t.Fatalf("a bare thread_id must be refused (it is derived, never supplied): %v", err)
	}
}

func TestProviderWithoutSendCannotCreateDrafts(t *testing.T) {
	fx := newFixture(t, readOnlyProvider{mailcal.NewMemProvider(refstore.DefaultSeed())})
	_, err := fx.svc.CreateDraft(context.Background(), fx.account.ID, "a", "", mailcal.DraftPayload{
		To: []abi.Address{{Email: "a@b.com"}}, Subject: "s", BodyText: "b"})
	if !mailcal.IsCode(err, abi.ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}

// readOnlyProvider declares everything except sending.
type readOnlyProvider struct{ mailcal.Provider }

func (r readOnlyProvider) Capabilities(ctx context.Context, a abi.AccountRef) (abi.Capabilities, error) {
	caps, err := r.Provider.Capabilities(ctx, a)
	var ops []string
	for _, op := range caps.Ops {
		if op != abi.ExportMailSend {
			ops = append(ops, op)
		}
	}
	caps.Ops = ops
	return caps, err
}

func TestTestAccountProbesAndRecoversAnExpiredAccount(t *testing.T) {
	fx := newFixture(t, nil)
	ctx := context.Background()
	_ = fx.svc.Accounts().SetStatus(ctx, fx.account.ID, mailcal.StatusAuthExpired, "old token")
	res, err := fx.svc.TestAccount(ctx, fx.account.ID)
	if err != nil || !res.OK || len(res.Checks) != 2 {
		t.Fatalf("probe failed: %+v %v", res, err)
	}
	got, _ := fx.svc.Accounts().Get(ctx, fx.account.ID)
	if got.Status != mailcal.StatusOK {
		t.Fatalf("a passing probe must clear auth_expired, status = %s", got.Status)
	}
}

func TestTestAccountReportsFailuresWithoutErroring(t *testing.T) {
	f := &flaky{Provider: mailcal.NewMemProvider(refstore.DefaultSeed()), failures: 99, err: abi.Errorf(abi.ErrAuthExpired, "revoked")}
	fx := newFixture(t, f)
	res, err := fx.svc.TestAccount(context.Background(), fx.account.ID)
	if err != nil || res.OK || !strings.Contains(res.Detail, "revoked") {
		t.Fatalf("got %+v %v", res, err)
	}
	fx.svc.PluginDeactivated(context.Background(), "refmem")
	res, _ = fx.svc.TestAccount(context.Background(), fx.account.ID)
	if res.OK || !strings.Contains(res.Detail, "not loaded") {
		t.Fatalf("missing plugin: %+v", res)
	}
	if _, err := fx.svc.TestAccount(context.Background(), "acct_nope"); !errors.Is(err, mailcal.ErrAccountNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
}

func TestValidateAccountAgainstProvider(t *testing.T) {
	fx := newFixture(t, nil)
	if err := fx.svc.ValidateAccountAgainstProvider("refmem", mailcal.KindBoth); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.ValidateAccountAgainstProvider("nope", mailcal.KindMail); !errors.Is(err, mailcal.ErrInvalidAccount) {
		t.Fatalf("unknown plugin: %v", err)
	}
}

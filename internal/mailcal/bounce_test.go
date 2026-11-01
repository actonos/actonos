package mailcal_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/policy"
	"github.com/actonos/actonos/internal/mailcal/refstore"
)

func TestLooksLikeBounce(t *testing.T) {
	yes := []abi.Message{
		{From: abi.Address{Email: "MAILER-DAEMON@mx.example"}, Subject: "x"},
		{From: abi.Address{Email: "a@b.com"}, Subject: "Undelivered Mail Returned to Sender"},
		{From: abi.Address{Email: "a@b.com"}, Subject: "Delivery Status Notification (Failure)"},
	}
	for _, m := range yes {
		if !mailcal.LooksLikeBounce(m) {
			t.Errorf("should be a bounce: %+v", m)
		}
	}
	if mailcal.LooksLikeBounce(abi.Message{From: abi.Address{Email: "alice@partner.com"}, Subject: "Q3 invoice"}) {
		t.Error("an ordinary message is not a bounce")
	}
}

func TestSeveralBouncesPauseTheRules(t *testing.T) {
	seed := refstore.DefaultSeed()
	for i := 0; i < 3; i++ {
		seed.Messages = append(seed.Messages, abi.Message{ID: fmt.Sprintf("b%d", i), ThreadID: fmt.Sprintf("tb%d", i), Folder: "INBOX",
			From: abi.Address{Email: "mailer-daemon@mx.example"}, To: []abi.Address{{Email: refstore.SelfEmail}},
			Subject: "Undelivered Mail Returned to Sender", Date: time.Date(2026, 3, 5, 9, i, 0, 0, time.UTC), BodyText: "failed"})
	}
	fx := newWriteFixture(t, nil)
	fx.provider = mailcal.NewMemProvider(seed)
	fx.enable(t)
	rule := fx.addRule(t, autoReplyRule(5))
	// Replace the provider behind the account with the seeded one.
	if err := fx.svc.RegisterProvider(context.Background(), "refmem", "Reference", fx.provider); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{Folder: "INBOX", Limit: 50}); err != nil {
		t.Fatal(err)
	}
	got, err := fx.svc.Rules().Get(context.Background(), rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.PausedReason == "" {
		t.Fatalf("rule should be paused with a reason: %+v", got)
	}
	// Listing again must not count the same bounces twice or fail.
	if _, _, err := fx.svc.MailList(context.Background(), fx.account.ID, abi.MailListParams{Folder: "INBOX", Limit: 50}); err != nil {
		t.Fatal(err)
	}
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	if dec := fx.svc.Decide(context.Background(), policyReplyFor(d)); dec.Verdict != policy.VerdictNeedsApproval {
		t.Fatalf("a paused rule must not allow: %+v", dec)
	}
}

func policyReplyFor(d mailcal.Draft) policy.Request {
	return policy.Request{AccountID: d.AccountID, Action: policy.ActionReply, To: []string{"alice@partner.com"}, Subject: d.Payload.Subject,
		Body: d.Payload.BodyText, Reply: &policy.ReplyFacts{Verified: true, Participants: d.Reply.Participants}}
}

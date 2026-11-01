package mailcal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/tools"
)

// A scheduled triage run executes with approvals bypassed, because nobody is there to click.
// These tests pin down what that does and does not allow.

func cronCtx() context.Context { return tools.WithBypassApproval(context.Background()) }

func TestACronRunCannotSendWhileTheMasterSwitchIsOff(t *testing.T) {
	fx := newWriteFixture(t, nil)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	before := fx.sentCount(t)
	if _, err := fx.exec(cronCtx(), mailcal.ToolMailSend, sendInput(d)); !errors.Is(err, tools.ErrToolDeniedByGate) {
		t.Fatalf("want a gate denial, got %v", err)
	}
	if fx.sentCount(t) != before {
		t.Fatal("a cron run sent mail while the switch was off")
	}
}

func TestACronRunWithoutARuleStopsAtAnApprovalRequest(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	before := fx.sentCount(t)
	_, err := fx.exec(cronCtx(), mailcal.ToolMailSend, sendInput(d))
	needApproval(t, err)
	if fx.sentCount(t) != before {
		t.Fatal("a cron run sent mail without a rule or an approval")
	}
}

func TestACronRunUnderAMatchingRuleSendsOnceAndRetriesAreReplays(t *testing.T) {
	fx := newWriteFixture(t, nil)
	fx.enable(t)
	fx.addRule(t, autoReplyRule(5))
	d := fx.draft(t, "alice@partner.com", "Received, thank you.", "m1")
	before := fx.sentCount(t)

	if _, err := fx.exec(cronCtx(), mailcal.ToolMailSend, sendInput(d)); err != nil {
		t.Fatalf("the rule should let the scheduled run send: %v", err)
	}
	res, err := fx.exec(cronCtx(), mailcal.ToolMailSend, sendInput(d)) // the next tick finds the same draft
	if err != nil {
		t.Fatal(err)
	}
	if res.Data["replayed"] != true {
		t.Fatalf("the repeat must be a replay: %+v", res.Data)
	}
	if got := fx.sentCount(t); got != before+1 {
		t.Fatalf("sent %d messages, want exactly 1", got-before)
	}
}

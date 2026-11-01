package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/actonos/actonos/evals"
	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/tools"
)

// mailcalFixture gives one eval task a mailbox and calendar backed by the reference dataset
// (refstore.DefaultSeed), exposed through the real mailcal tools, and can check afterwards
// what the agent did to it.
type mailcalFixture struct {
	svc        *mailcal.Service
	provider   mailcal.Provider
	account    mailcal.Account
	sentBefore int
}

func newMailcalFixture(ctx context.Context, db *sql.DB, registry *tools.ToolRegistry) (*mailcalFixture, error) {
	svc, err := mailcal.NewService(db)
	if err != nil {
		return nil, fmt.Errorf("creating mailcal service: %w", err)
	}
	provider := mailcal.NewMemProvider(refstore.DefaultSeed())
	if err := svc.RegisterProvider(ctx, "refmem", "Reference", provider); err != nil {
		return nil, err
	}
	svc.SetToolRegistry(registry)
	acct, err := svc.Accounts().Create(ctx, mailcal.NewAccount{
		Label: "Eval mailbox", PluginID: "refmem", Kind: mailcal.KindBoth, Email: refstore.SelfEmail,
	})
	if err != nil {
		return nil, err
	}
	if err := svc.SyncTools(ctx); err != nil {
		return nil, err
	}
	f := &mailcalFixture{svc: svc, provider: provider, account: acct}
	sent, err := provider.MailList(ctx, acct.Ref(), abi.MailListParams{Folder: "Sent", Limit: 100})
	if err != nil {
		return nil, err
	}
	f.sentBefore = len(sent.Messages)
	return f, nil
}

// Close withdraws the tools and removes the account so the next task starts clean.
func (f *mailcalFixture) Close(ctx context.Context) {
	if f == nil {
		return
	}
	_ = f.svc.Accounts().Delete(ctx, f.account.ID)
	_ = f.svc.SyncTools(ctx)
}

// perform does what an ideal agent would, for the offline mock run.
func (f *mailcalFixture) perform(ctx context.Context, exp *evals.Expectations) (string, error) {
	if exp == nil {
		return "", nil
	}
	for _, to := range exp.DraftTo {
		if _, err := f.svc.CreateDraft(ctx, f.account.ID, "eval-mock", "", mailcal.DraftPayload{
			To: []abi.Address{{Email: to}}, Subject: "Re: eval", BodyText: "Thanks, noted.",
		}); err != nil {
			return "", err
		}
	}
	return "Done. " + strings.Join(exp.AnswerContains, ", "), nil
}

// verify returns "" when the agent's effect on the mailbox and its answer meet exp, otherwise why not.
func (f *mailcalFixture) verify(ctx context.Context, exp *evals.Expectations, answer string) string {
	if exp == nil {
		return ""
	}
	lower := strings.ToLower(answer)
	for _, want := range exp.AnswerContains {
		if !strings.Contains(lower, strings.ToLower(want)) {
			return fmt.Sprintf("the answer does not mention %q", want)
		}
	}
	if f == nil {
		if len(exp.DraftTo) > 0 || len(exp.NoDraftTo) > 0 || exp.NothingSent {
			return "the task has mailbox expectations but no mailcal fixture"
		}
		return ""
	}
	drafts, err := f.svc.Drafts().List(ctx, f.account.ID, 200)
	if err != nil {
		return "could not read drafts: " + err.Error()
	}
	addressed := map[string]bool{}
	for _, d := range drafts {
		for _, group := range [][]abi.Address{d.Payload.To, d.Payload.Cc, d.Payload.Bcc} {
			for _, a := range group {
				addressed[a.Email] = true
			}
		}
	}
	for _, to := range exp.DraftTo {
		email, err := abi.NormalizeEmail(to)
		if err != nil || !addressed[email] {
			return fmt.Sprintf("no draft addressed to %s", to)
		}
	}
	for _, to := range exp.NoDraftTo {
		if email, err := abi.NormalizeEmail(to); err == nil && addressed[email] {
			return fmt.Sprintf("a draft was addressed to %s, which the task forbids", to)
		}
	}
	if exp.NothingSent {
		sent, err := f.provider.MailList(ctx, f.account.Ref(), abi.MailListParams{Folder: "Sent", Limit: 100})
		if err != nil {
			return "could not read the Sent folder: " + err.Error()
		}
		if len(sent.Messages) != f.sentBefore {
			return fmt.Sprintf("something was sent (%d message(s) in Sent, was %d)", len(sent.Messages), f.sentBefore)
		}
	}
	return ""
}

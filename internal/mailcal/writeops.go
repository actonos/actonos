package mailcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/policy"
)

// Errors a write can end with besides provider errors.
var (
	ErrWriteDenied      = errors.New("the write is not allowed")
	ErrApprovalRequired = errors.New("the write needs a person's approval")
	ErrMailcalOff       = errors.New("mail and calendar writes are switched off")
)

// actionMailDelete and actionCalDelete are not valid rule actions, so no rule can ever match them.
const (
	actionMailDelete policy.Action = "mail_delete"
	actionCalDelete  policy.Action = "calendar_delete"
	actionCalRespond policy.Action = "calendar_respond"
)

func addrsToEmails(in []abi.Address) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = a.Email
	}
	return out
}

// requestForDraft builds the engine input from the stored draft: recipients, text and the
// provider-verified reply context. Nothing the agent says at send time is used.
func requestForDraft(d Draft) policy.Request {
	req := policy.Request{
		AccountID: d.AccountID, Action: policy.ActionSendNew,
		To: addrsToEmails(d.Payload.To), Cc: addrsToEmails(d.Payload.Cc), Bcc: addrsToEmails(d.Payload.Bcc),
		Subject: d.Payload.Subject, Body: d.Payload.BodyText,
	}
	if d.Reply != nil {
		req.Action = policy.ActionReply
		req.Reply = &policy.ReplyFacts{Verified: d.Reply.Verified, Participants: d.Reply.Participants}
	}
	return req
}

func eventFacts(ev abi.Event) (*policy.CalendarFacts, error) {
	loc := time.UTC
	if ev.TimeZone != "" {
		l, err := time.LoadLocation(ev.TimeZone)
		if err != nil {
			return nil, fmt.Errorf("unknown time zone %q", ev.TimeZone)
		}
		loc = l
	}
	f := &policy.CalendarFacts{Start: ev.Start.In(loc), End: ev.End.In(loc)}
	for _, a := range ev.Attendees {
		f.Attendees = append(f.Attendees, a.Email)
	}
	if ev.Organizer != nil {
		f.Organizer = ev.Organizer.Email
	}
	return f, nil
}

// Decide asks the engine about a request. The master switch is checked first; an engine
// error is a needs_approval, never an allow.
func (s *Service) Decide(ctx context.Context, req policy.Request) policy.Decision {
	if !s.settings.Enabled(ctx) {
		return policy.Decision{Verdict: policy.VerdictDeny, Reasons: []string{ErrMailcalOff.Error()}}
	}
	stored, err := s.rules.List(ctx, req.AccountID)
	if err != nil {
		return policy.Decision{Verdict: policy.VerdictNeedsApproval, Reasons: []string{"the rule book could not be read"}}
	}
	rules := make([]policy.Rule, len(stored))
	for i, r := range stored {
		rules[i] = r.Rule
	}
	return policy.Evaluate(rules, req, time.Now().UTC(), func(id string, w time.Duration) (int, error) {
		return s.decisions.CountAutoExecutions(ctx, id, w)
	})
}

// WriteOptions says who is writing and whether a person approved this exact action.
type WriteOptions struct {
	AgentID  string
	TraceID  string
	Approved bool
}

func (s *Service) authorize(dec policy.Decision, opt WriteOptions) error {
	switch dec.Verdict {
	case policy.VerdictDeny:
		return fmt.Errorf("%w: %s", ErrWriteDenied, strings.Join(dec.Reasons, "; "))
	case policy.VerdictNeedsApproval:
		if !opt.Approved {
			return fmt.Errorf("%w: %s", ErrApprovalRequired, strings.Join(dec.Reasons, "; "))
		}
	}
	return nil
}

var writeBackoff = []time.Duration{300 * time.Millisecond, 1200 * time.Millisecond}

// writeCall retries only transient failures, and only because every write passes a key (or is
// naturally idempotent) so a retry cannot repeat the effect.
func (s *Service) writeCall(ctx context.Context, b binding, fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		e := AsError(err)
		if e == nil {
			return err
		}
		switch e.Code {
		case abi.ErrAuthExpired:
			_ = s.accounts.SetStatus(context.WithoutCancel(ctx), b.account.ID, StatusAuthExpired, e.Message)
			return err
		case abi.ErrTransient:
			if attempt >= len(writeBackoff) {
				return err
			}
			if serr := s.sleep(ctx, writeBackoff[attempt]); serr != nil {
				return serr
			}
		default:
			return err
		}
	}
}

// SendResult is what Send reports.
type SendResult struct {
	ProviderMessageID string `json:"provider_message_id"`
	ThreadID          string `json:"thread_id,omitempty"`
	Replayed          bool   `json:"replayed,omitempty"`
	RuleID            string `json:"rule_id,omitempty"`
}

// Send sends a stored draft. It re-evaluates the engine itself, so the guarantee does not
// depend on the caller having run the gate. A draft is sent at most once.
func (s *Service) Send(ctx context.Context, draftID string, opt WriteOptions) (SendResult, error) {
	d, err := s.drafts.Get(ctx, draftID)
	if err != nil {
		return SendResult{}, err
	}
	dec := s.Decide(ctx, requestForDraft(d))
	if err := s.authorize(dec, opt); err != nil {
		return SendResult{}, err
	}
	b, err := s.resolve(ctx, d.AccountID, KindMail)
	if err != nil {
		return SendResult{}, err
	}
	if err := b.require(abi.ExportMailSend); err != nil {
		return SendResult{}, err
	}
	key := KeyFor("mail_send", d.AccountID, d.ID)
	entry, existed, unlock, err := s.ledger.Begin(ctx, LedgerEntry{Key: key, AccountID: d.AccountID, DraftID: d.ID, PayloadHash: d.PayloadHash})
	if err != nil {
		return SendResult{}, err
	}
	defer unlock()
	if existed && entry.State == LedgerSent {
		return SendResult{ProviderMessageID: entry.ProviderMessageID, ThreadID: entry.ThreadID, Replayed: true, RuleID: dec.RuleID}, nil
	}
	var res abi.MailSendResult
	err = s.writeCall(ctx, b, func() (e error) {
		res, e = b.provider.MailSend(ctx, b.account.Ref(), abi.MailSendParams{
			To: d.Payload.To, Cc: d.Payload.Cc, Bcc: d.Payload.Bcc, Subject: d.Payload.Subject, BodyText: d.Payload.BodyText,
			InReplyTo: d.Payload.InReplyTo, ThreadID: d.Payload.ThreadID, IdempotencyKey: key,
		})
		return
	})
	if err != nil {
		return SendResult{}, err
	}
	if err := s.ledger.Complete(ctx, key, res.ProviderMessageID, res.ThreadID); err != nil {
		// The message is out; report success but make the problem visible.
		_ = s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: d.AccountID, AgentID: opt.AgentID,
			Tool: ToolMailSend, DraftID: d.ID, PayloadHash: d.PayloadHash, Verdict: dec.Verdict, RuleID: dec.RuleID,
			Approved: opt.Approved, Outcome: "sent_ledger_update_failed"})
	} else {
		_ = s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: d.AccountID, AgentID: opt.AgentID,
			Tool: ToolMailSend, DraftID: d.ID, PayloadHash: d.PayloadHash, Verdict: dec.Verdict, RuleID: dec.RuleID,
			Approved: opt.Approved, Outcome: "sent"})
	}
	return SendResult{ProviderMessageID: res.ProviderMessageID, ThreadID: res.ThreadID, Replayed: res.Replayed, RuleID: dec.RuleID}, nil
}

// MailDelete deletes messages. Deleting always needs a person.
func (s *Service) MailDelete(ctx context.Context, accountID string, ids []string, opt WriteOptions) error {
	b, err := s.resolve(ctx, accountID, KindMail)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("no message ids given")
	}
	dec := s.Decide(ctx, policy.Request{AccountID: b.account.ID, Action: actionMailDelete, Destructive: true})
	if err := s.authorize(dec, opt); err != nil {
		return err
	}
	if err := b.require(abi.ExportMailDelete); err != nil {
		return err
	}
	if err := s.writeCall(ctx, b, func() error { return b.provider.MailDelete(ctx, b.account.Ref(), abi.MailDeleteParams{IDs: ids}) }); err != nil {
		return err
	}
	return s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: b.account.ID, AgentID: opt.AgentID,
		Tool: ToolMailDelete, Verdict: dec.Verdict, Approved: opt.Approved, Outcome: fmt.Sprintf("deleted %d", len(ids))})
}

// CalendarCreate creates an event, once per (run, payload).
func (s *Service) CalendarCreate(ctx context.Context, accountID string, ev abi.Event, sendUpdates bool, opt WriteOptions) (abi.Event, error) {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return abi.Event{}, err
	}
	facts, err := eventFacts(ev)
	if err != nil {
		return abi.Event{}, err
	}
	dec := s.Decide(ctx, policy.Request{AccountID: b.account.ID, Action: policy.ActionCalendarCreate, Calendar: facts})
	if err := s.authorize(dec, opt); err != nil {
		return abi.Event{}, err
	}
	if err := b.require(abi.ExportCalCreateEvent); err != nil {
		return abi.Event{}, err
	}
	canon, _ := json.Marshal(ev)
	key := KeyFor("cal_create", b.account.ID, opt.TraceID, string(canon))
	entry, existed, unlock, err := s.ledger.Begin(ctx, LedgerEntry{Key: key, AccountID: b.account.ID, PayloadHash: KeyFor(string(canon))})
	if err != nil {
		return abi.Event{}, err
	}
	defer unlock()
	if existed && entry.State == LedgerSent {
		ev.ID = entry.ProviderMessageID
		return ev, nil
	}
	var out abi.Event
	err = s.writeCall(ctx, b, func() (e error) {
		out, e = b.provider.CalendarCreate(ctx, b.account.Ref(), abi.CalWriteParams{Event: ev, SendUpdates: sendUpdates, IdempotencyKey: key})
		return
	})
	if err != nil {
		return abi.Event{}, err
	}
	_ = s.ledger.Complete(ctx, key, out.ID, "")
	_ = s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: b.account.ID, AgentID: opt.AgentID,
		Tool: ToolCalCreate, Verdict: dec.Verdict, RuleID: dec.RuleID, Approved: opt.Approved, Outcome: "created"})
	return out, nil
}

// CalendarUpdate replaces an event. The attendee and time facts come from the new event.
func (s *Service) CalendarUpdate(ctx context.Context, accountID string, ev abi.Event, sendUpdates bool, opt WriteOptions) (abi.Event, error) {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return abi.Event{}, err
	}
	if strings.TrimSpace(ev.ID) == "" {
		return abi.Event{}, errors.New("event id is required")
	}
	facts, err := eventFacts(ev)
	if err != nil {
		return abi.Event{}, err
	}
	dec := s.Decide(ctx, policy.Request{AccountID: b.account.ID, Action: policy.ActionCalendarUpdate, Calendar: facts})
	if err := s.authorize(dec, opt); err != nil {
		return abi.Event{}, err
	}
	if err := b.require(abi.ExportCalUpdateEvent); err != nil {
		return abi.Event{}, err
	}
	var out abi.Event
	if err := s.writeCall(ctx, b, func() (e error) {
		out, e = b.provider.CalendarUpdate(ctx, b.account.Ref(), abi.CalWriteParams{Event: ev, SendUpdates: sendUpdates})
		return
	}); err != nil {
		return abi.Event{}, err
	}
	_ = s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: b.account.ID, AgentID: opt.AgentID,
		Tool: ToolCalUpdate, Verdict: dec.Verdict, RuleID: dec.RuleID, Approved: opt.Approved, Outcome: "updated"})
	return out, nil
}

// CalendarRespond answers an invitation. Only "accepted" can be allowed by a rule, and the
// organizer is read from the provider, not from the caller.
func (s *Service) CalendarRespond(ctx context.Context, accountID string, p abi.CalRespondParams, opt WriteOptions) (abi.Event, error) {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return abi.Event{}, err
	}
	req, err := s.respondRequest(ctx, b, p)
	if err != nil {
		return abi.Event{}, err
	}
	dec := s.Decide(ctx, req)
	if err := s.authorize(dec, opt); err != nil {
		return abi.Event{}, err
	}
	if err := b.require(abi.ExportCalRespond); err != nil {
		return abi.Event{}, err
	}
	var out abi.Event
	if err := s.writeCall(ctx, b, func() (e error) { out, e = b.provider.CalendarRespond(ctx, b.account.Ref(), p); return }); err != nil {
		return abi.Event{}, err
	}
	_ = s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: b.account.ID, AgentID: opt.AgentID,
		Tool: ToolCalRespond, Verdict: dec.Verdict, RuleID: dec.RuleID, Approved: opt.Approved, Outcome: p.Response})
	return out, nil
}

func (s *Service) respondRequest(ctx context.Context, b binding, p abi.CalRespondParams) (policy.Request, error) {
	if p.Response != "accepted" {
		// Declining or tentative answers are never auto-sent: no rule action exists for them.
		return policy.Request{AccountID: b.account.ID, Action: actionCalRespond}, nil
	}
	if err := b.require(abi.ExportCalGetEvent); err != nil {
		return policy.Request{}, err
	}
	var ev abi.Event
	if err := s.readCall(ctx, b, func() (e error) {
		ev, e = b.provider.CalendarGet(ctx, b.account.Ref(), abi.CalGetParams{CalendarID: p.CalendarID, ID: p.ID})
		return
	}); err != nil {
		return policy.Request{}, err
	}
	facts, err := eventFacts(ev)
	if err != nil {
		return policy.Request{}, err
	}
	return policy.Request{AccountID: b.account.ID, Action: policy.ActionCalendarAccept, Calendar: facts}, nil
}

// CalendarDelete deletes an event. Deleting always needs a person.
func (s *Service) CalendarDelete(ctx context.Context, accountID string, p abi.CalDeleteParams, opt WriteOptions) error {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return err
	}
	dec := s.Decide(ctx, policy.Request{AccountID: b.account.ID, Action: actionCalDelete, Destructive: true})
	if err := s.authorize(dec, opt); err != nil {
		return err
	}
	if err := b.require(abi.ExportCalDeleteEvent); err != nil {
		return err
	}
	if err := s.writeCall(ctx, b, func() error { return b.provider.CalendarDelete(ctx, b.account.Ref(), p) }); err != nil {
		return err
	}
	return s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{Phase: PhaseExecuted, AccountID: b.account.ID, AgentID: opt.AgentID,
		Tool: ToolCalDelete, Verdict: dec.Verdict, Approved: opt.Approved, Outcome: "deleted"})
}

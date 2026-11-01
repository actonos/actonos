// Package conformance is the test suite every mail/calendar provider must pass.
//
// RunShape works against any provider (even a real one pointed at a sandbox account): it
// checks the contract, not the data. RunSeeded additionally checks behaviour against the
// reference dataset (refstore.DefaultSeed) and is run only for reference providers.
//
// Compatibility rule: cases are only ever added, never loosened. A provider that passed
// an earlier version of this suite must keep passing.
package conformance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
)

// Factory returns a fresh provider; data created by one test must not leak into the next.
type Factory func(t *testing.T) mailcal.Provider

func ctx() context.Context { return context.Background() }

func requireCode(t *testing.T, err error, code abi.ErrorCode, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error %q, got success", what, code)
	}
	got := mailcal.AsError(err)
	if got == nil {
		t.Fatalf("%s: expected a coded provider error %q, got plain error: %v", what, code, err)
	}
	if got.Code != code {
		t.Fatalf("%s: error code = %q, want %q (%v)", what, got.Code, code, err)
	}
}

func requireOK(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// RunShape checks the contract with acct against any provider.
func RunShape(t *testing.T, p mailcal.Provider, acct abi.AccountRef) {
	t.Helper()
	caps, err := p.Capabilities(ctx(), acct)
	requireOK(t, err, "Capabilities")

	if len(caps.ABI) == 0 {
		t.Fatal("Capabilities.ABI is empty; declare mail@1 and/or calendar@1")
	}
	for _, a := range caps.ABI {
		if a != abi.CapMail && a != abi.CapCalendar {
			t.Errorf("unknown ABI entry %q", a)
		}
	}
	known := map[string]bool{}
	for _, name := range abi.AllExports {
		known[name] = true
	}
	for _, op := range caps.Ops {
		if !known[op] {
			t.Errorf("Capabilities.Ops lists %q which is not an ABI export", op)
		}
	}
	if caps.Limits.MaxRecipients < 0 || caps.Limits.MaxBodyBytes < 0 || caps.Limits.MaxAttachmentBytes < 0 {
		t.Errorf("limits must not be negative: %+v", caps.Limits)
	}

	// Each operation either works or says "unsupported"; it never returns an uncoded error.
	type op struct {
		export string
		run    func() error
	}
	now := time.Now().UTC().Truncate(time.Second)
	ops := []op{
		{abi.ExportMailList, func() error { _, e := p.MailList(ctx(), acct, abi.MailListParams{Limit: 1}); return e }},
		{abi.ExportMailSearch, func() error { _, e := p.MailSearch(ctx(), acct, abi.MailSearchParams{Query: "x", Limit: 1}); return e }},
		{abi.ExportMailGet, func() error { _, e := p.MailGet(ctx(), acct, "does-not-exist"); return e }},
		{abi.ExportMailGetThread, func() error { _, e := p.MailGetThread(ctx(), acct, "does-not-exist"); return e }},
		{abi.ExportMailSend, func() error { _, e := p.MailSend(ctx(), acct, abi.MailSendParams{}); return e }},
		{abi.ExportMailMark, func() error { return p.MailMark(ctx(), acct, abi.MailMarkParams{IDs: []string{"does-not-exist"}}) }},
		{abi.ExportMailMove, func() error {
			return p.MailMove(ctx(), acct, abi.MailMoveParams{IDs: []string{"does-not-exist"}, Folder: "Archive"})
		}},
		{abi.ExportMailDelete, func() error { return p.MailDelete(ctx(), acct, abi.MailDeleteParams{IDs: []string{"does-not-exist"}}) }},
		{abi.ExportCalListEvents, func() error {
			_, e := p.CalendarList(ctx(), acct, abi.CalListParams{From: now, To: now.Add(time.Hour)})
			return e
		}},
		{abi.ExportCalGetEvent, func() error { _, e := p.CalendarGet(ctx(), acct, abi.CalGetParams{ID: "does-not-exist"}); return e }},
		{abi.ExportCalDeleteEvent, func() error { return p.CalendarDelete(ctx(), acct, abi.CalDeleteParams{ID: "does-not-exist"}) }},
		{abi.ExportCalFreeBusy, func() error {
			_, e := p.CalendarFreeBusy(ctx(), acct, abi.FreeBusyParams{Emails: []string{"a@example.com"}, From: now, To: now.Add(time.Hour)})
			return e
		}},
	}
	for _, o := range ops {
		o := o
		t.Run("contract/"+o.export, func(t *testing.T) {
			err := o.run()
			if !caps.Supports(o.export) {
				requireCode(t, err, abi.ErrUnsupported, o.export+" not declared")
				return
			}
			// Supported operations with bad or unknown input must fail with a coded error,
			// never succeed silently and never return a transport-level error.
			if err != nil && mailcal.AsError(err) == nil {
				t.Fatalf("%s returned an uncoded error: %v", o.export, err)
			}
		})
	}
}

// RunSeeded checks behaviour against refstore.DefaultSeed. Each subtest gets a fresh
// provider so tests cannot depend on one another.
func RunSeeded(t *testing.T, newProvider Factory) {
	t.Helper()
	acct := abi.AccountRef{ID: "acct-a"}

	t.Run("shape", func(t *testing.T) { RunShape(t, newProvider(t), acct) })

	t.Run("list/orders newest first without bodies", func(t *testing.T) {
		res, err := newProvider(t).MailList(ctx(), acct, abi.MailListParams{})
		requireOK(t, err, "MailList")
		ids := messageIDs(res.Messages)
		if fmt.Sprint(ids) != "[m5 m3 m2 m1]" {
			t.Fatalf("INBOX order = %v, want [m5 m3 m2 m1]", ids)
		}
		for _, m := range res.Messages {
			if m.BodyText != "" {
				t.Errorf("list must not return bodies, %s has one", m.ID)
			}
			if m.Snippet == "" {
				t.Errorf("list should provide a snippet for %s", m.ID)
			}
		}
	})

	t.Run("list/paginates without gaps or repeats", func(t *testing.T) {
		p := newProvider(t)
		first, err := p.MailList(ctx(), acct, abi.MailListParams{Limit: 2})
		requireOK(t, err, "page 1")
		if len(first.Messages) != 2 || first.NextCursor == "" {
			t.Fatalf("page 1 = %v next=%q", messageIDs(first.Messages), first.NextCursor)
		}
		second, err := p.MailList(ctx(), acct, abi.MailListParams{Limit: 2, Cursor: first.NextCursor})
		requireOK(t, err, "page 2")
		all := append(messageIDs(first.Messages), messageIDs(second.Messages)...)
		if fmt.Sprint(all) != "[m5 m3 m2 m1]" || second.NextCursor != "" {
			t.Fatalf("pages = %v next=%q", all, second.NextCursor)
		}
		_, err = p.MailList(ctx(), acct, abi.MailListParams{Cursor: "not-a-cursor"})
		requireCode(t, err, abi.ErrInvalid, "bad cursor")
	})

	t.Run("get/returns body and not_found", func(t *testing.T) {
		p := newProvider(t)
		m, err := p.MailGet(ctx(), acct, "m1")
		requireOK(t, err, "MailGet")
		if m.BodyText == "" || m.From.Email != "alice@partner.com" || m.ThreadID != "t1" {
			t.Fatalf("unexpected message %+v", m)
		}
		_, err = p.MailGet(ctx(), acct, "nope")
		requireCode(t, err, abi.ErrNotFound, "MailGet unknown")
	})

	t.Run("thread/oldest first", func(t *testing.T) {
		p := newProvider(t)
		th, err := p.MailGetThread(ctx(), acct, "t1")
		requireOK(t, err, "MailGetThread")
		if fmt.Sprint(messageIDs(th.Messages)) != "[m1 m4 m5]" {
			t.Fatalf("thread t1 = %v", messageIDs(th.Messages))
		}
		_, err = p.MailGetThread(ctx(), acct, "nope")
		requireCode(t, err, abi.ErrNotFound, "unknown thread")
	})

	t.Run("search/folders and validation", func(t *testing.T) {
		p := newProvider(t)
		all, err := p.MailSearch(ctx(), acct, abi.MailSearchParams{Query: "invoice"})
		requireOK(t, err, "search all")
		if fmt.Sprint(messageIDs(all.Messages)) != "[m5 m4 m1]" {
			t.Fatalf("search all = %v", messageIDs(all.Messages))
		}
		inbox, err := p.MailSearch(ctx(), acct, abi.MailSearchParams{Query: "invoice", Folder: "INBOX"})
		requireOK(t, err, "search INBOX")
		if fmt.Sprint(messageIDs(inbox.Messages)) != "[m5 m1]" {
			t.Fatalf("search INBOX = %v", messageIDs(inbox.Messages))
		}
		_, err = p.MailSearch(ctx(), acct, abi.MailSearchParams{Query: "  "})
		requireCode(t, err, abi.ErrInvalid, "empty query")
	})

	t.Run("send/is idempotent and validated", func(t *testing.T) {
		p := newProvider(t)
		params := abi.MailSendParams{
			To:      []abi.Address{{Email: "alice@partner.com"}},
			Subject: "Re: Q3 invoice", BodyText: "Paid.", ThreadID: "t1", InReplyTo: "m5", IdempotencyKey: "key-1",
		}
		first, err := p.MailSend(ctx(), acct, params)
		requireOK(t, err, "first send")
		if first.ProviderMessageID == "" || first.Replayed {
			t.Fatalf("first send = %+v", first)
		}
		again, err := p.MailSend(ctx(), acct, params)
		requireOK(t, err, "replayed send")
		if again.ProviderMessageID != first.ProviderMessageID || !again.Replayed {
			t.Fatalf("replay must return the original id and Replayed=true: first=%+v again=%+v", first, again)
		}
		sent, err := p.MailList(ctx(), acct, abi.MailListParams{Folder: "Sent"})
		requireOK(t, err, "list Sent")
		if len(sent.Messages) != 2 { // seeded m4 + exactly one new message
			t.Fatalf("Sent holds %v; a replay must not send twice", messageIDs(sent.Messages))
		}

		bad := params
		bad.IdempotencyKey = ""
		_, err = p.MailSend(ctx(), acct, bad)
		requireCode(t, err, abi.ErrInvalid, "missing idempotency key")
		bad = params
		bad.IdempotencyKey, bad.To = "key-2", nil
		_, err = p.MailSend(ctx(), acct, bad)
		requireCode(t, err, abi.ErrInvalid, "no recipients")
	})

	t.Run("mark move delete", func(t *testing.T) {
		p := newProvider(t)
		requireOK(t, p.MailMark(ctx(), acct, abi.MailMarkParams{IDs: []string{"m2"}, Unread: false}), "mark read")
		unread, err := p.MailList(ctx(), acct, abi.MailListParams{UnreadOnly: true})
		requireOK(t, err, "unread list")
		if fmt.Sprint(messageIDs(unread.Messages)) != "[m5 m3 m1]" {
			t.Fatalf("unread = %v", messageIDs(unread.Messages))
		}
		requireCode(t, p.MailMark(ctx(), acct, abi.MailMarkParams{IDs: []string{"m2", "nope"}, Unread: true}), abi.ErrNotFound, "mark with an unknown id")
		still, _ := p.MailGet(ctx(), acct, "m2")
		if still.Unread {
			t.Fatal("a failed batch must not partially apply")
		}

		requireOK(t, p.MailMove(ctx(), acct, abi.MailMoveParams{IDs: []string{"m2"}, Folder: "Archive"}), "move")
		archive, _ := p.MailList(ctx(), acct, abi.MailListParams{Folder: "Archive"})
		if fmt.Sprint(messageIDs(archive.Messages)) != "[m2]" {
			t.Fatalf("Archive = %v", messageIDs(archive.Messages))
		}
		requireCode(t, p.MailMove(ctx(), acct, abi.MailMoveParams{IDs: []string{"m2"}}), abi.ErrInvalid, "move without folder")

		requireOK(t, p.MailDelete(ctx(), acct, abi.MailDeleteParams{IDs: []string{"m2"}}), "delete -> trash")
		trash, _ := p.MailList(ctx(), acct, abi.MailListParams{Folder: "Trash"})
		if len(trash.Messages) != 1 {
			t.Fatalf("Trash = %v", messageIDs(trash.Messages))
		}
		requireOK(t, p.MailDelete(ctx(), acct, abi.MailDeleteParams{IDs: []string{"m2"}}), "delete from trash")
		_, err = p.MailGet(ctx(), acct, "m2")
		requireCode(t, err, abi.ErrNotFound, "deleted message")
	})

	t.Run("drafts stay out of the inbox", func(t *testing.T) {
		p := newProvider(t)
		d, err := p.MailCreateDraft(ctx(), acct, abi.MailDraftParams{To: []abi.Address{{Email: "bob@example.org"}}, Subject: "Hi", BodyText: "x"})
		requireOK(t, err, "create draft")
		if d.ProviderDraftID == "" {
			t.Fatal("draft id is empty")
		}
		inbox, _ := p.MailList(ctx(), acct, abi.MailListParams{})
		if len(inbox.Messages) != 4 {
			t.Fatalf("a draft leaked into the inbox: %v", messageIDs(inbox.Messages))
		}
	})

	t.Run("accounts are isolated", func(t *testing.T) {
		p := newProvider(t)
		other := abi.AccountRef{ID: "acct-b"}
		requireOK(t, p.MailDelete(ctx(), acct, abi.MailDeleteParams{IDs: []string{"m1"}}), "delete in A")
		got, err := p.MailGet(ctx(), other, "m1")
		requireOK(t, err, "read same id in B")
		if got.Folder != "INBOX" {
			t.Fatalf("account B saw account A's change: %+v", got)
		}
	})

	day := func(d, h, m int) time.Time { return time.Date(2026, 3, d, h, m, 0, 0, time.UTC) }

	t.Run("calendar/range is half-open", func(t *testing.T) {
		p := newProvider(t)
		res, err := p.CalendarList(ctx(), acct, abi.CalListParams{From: day(3, 0, 0), To: day(4, 0, 0)})
		requireOK(t, err, "list day 3")
		if fmt.Sprint(eventIDs(res.Events)) != "[e1 e2]" {
			t.Fatalf("day 3 = %v", eventIDs(res.Events))
		}
		edge, err := p.CalendarList(ctx(), acct, abi.CalListParams{From: day(3, 10, 30), To: day(3, 14, 0)})
		requireOK(t, err, "list the gap")
		if len(edge.Events) != 0 {
			t.Fatalf("events touching only at the boundary must not match: %v", eventIDs(edge.Events))
		}
		_, err = p.CalendarList(ctx(), acct, abi.CalListParams{From: day(4, 0, 0), To: day(3, 0, 0)})
		requireCode(t, err, abi.ErrInvalid, "inverted range")
	})

	t.Run("calendar/create update delete", func(t *testing.T) {
		p := newProvider(t)
		ev := abi.Event{Title: "Review", Start: day(5, 9, 0), End: day(5, 10, 0), TimeZone: "UTC"}
		created, err := p.CalendarCreate(ctx(), acct, abi.CalWriteParams{Event: ev, IdempotencyKey: "c1"})
		requireOK(t, err, "create")
		if created.ID == "" {
			t.Fatal("created event has no id")
		}
		again, err := p.CalendarCreate(ctx(), acct, abi.CalWriteParams{Event: ev, IdempotencyKey: "c1"})
		requireOK(t, err, "create replay")
		if again.ID != created.ID {
			t.Fatalf("replayed create made a second event: %s vs %s", created.ID, again.ID)
		}
		_, err = p.CalendarCreate(ctx(), acct, abi.CalWriteParams{Event: abi.Event{Title: "Bad", Start: day(5, 10, 0), End: day(5, 9, 0)}})
		requireCode(t, err, abi.ErrInvalid, "end before start")

		created.Title = "Review (moved)"
		updated, err := p.CalendarUpdate(ctx(), acct, abi.CalWriteParams{Event: created})
		requireOK(t, err, "update")
		if updated.Title != "Review (moved)" {
			t.Fatalf("update lost: %+v", updated)
		}
		_, err = p.CalendarUpdate(ctx(), acct, abi.CalWriteParams{Event: abi.Event{ID: "nope", Title: "x", Start: day(5, 9, 0), End: day(5, 10, 0)}})
		requireCode(t, err, abi.ErrNotFound, "update unknown")

		requireOK(t, p.CalendarDelete(ctx(), acct, abi.CalDeleteParams{ID: created.ID}), "delete")
		_, err = p.CalendarGet(ctx(), acct, abi.CalGetParams{ID: created.ID})
		requireCode(t, err, abi.ErrNotFound, "deleted event")
	})

	t.Run("calendar/respond and free-busy", func(t *testing.T) {
		p := newProvider(t)
		ev, err := p.CalendarRespond(ctx(), acct, abi.CalRespondParams{ID: "e2", Response: "declined"})
		requireOK(t, err, "decline")
		for _, a := range ev.Attendees {
			if a.Email == "me@example.com" && a.Status != "declined" {
				t.Fatalf("own status = %q, want declined", a.Status)
			}
		}
		_, err = p.CalendarRespond(ctx(), acct, abi.CalRespondParams{ID: "e2", Response: "maybe"})
		requireCode(t, err, abi.ErrInvalid, "bad response")
		_, err = p.CalendarRespond(ctx(), acct, abi.CalRespondParams{ID: "nope", Response: "accepted"})
		requireCode(t, err, abi.ErrNotFound, "respond unknown")

		fb, err := p.CalendarFreeBusy(ctx(), acct, abi.FreeBusyParams{
			Emails: []string{"Me@Example.com", "alice@partner.com"}, From: day(3, 0, 0), To: day(4, 0, 0)})
		requireOK(t, err, "free-busy")
		if got := len(fb.Busy["me@example.com"]); got != 1 { // e1 only: e2 was declined
			t.Fatalf("me busy spans = %d, want 1 (declined events are free)", got)
		}
		if got := len(fb.Busy["alice@partner.com"]); got != 2 {
			t.Fatalf("alice busy spans = %d, want 2", got)
		}
		_, err = p.CalendarFreeBusy(ctx(), acct, abi.FreeBusyParams{Emails: []string{"not an email"}, From: day(3, 0, 0), To: day(4, 0, 0)})
		requireCode(t, err, abi.ErrInvalid, "bad email")
	})
}

func messageIDs(ms []abi.Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func eventIDs(es []abi.Event) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

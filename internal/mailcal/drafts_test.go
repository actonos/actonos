package mailcal_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
)

func validPayload() mailcal.DraftPayload {
	return mailcal.DraftPayload{
		To: []abi.Address{{Email: "Alice@Partner.com"}}, Subject: "Re: Q3 invoice", BodyText: "Paid this week.",
	}
}

func TestDraftIsStoredNormalizedAndImmutable(t *testing.T) {
	db := newDB(t)
	store, err := mailcal.NewDraftStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	d, err := store.Create(ctx, mailcal.NewDraft{AccountID: "acct1", CreatedBy: "agent1", Payload: validPayload()})
	if err != nil {
		t.Fatal(err)
	}
	if d.Payload.To[0].Email != "alice@partner.com" || d.PayloadHash == "" {
		t.Fatalf("payload not normalized/hashed: %+v", d)
	}
	got, err := store.Get(ctx, d.ID)
	if err != nil || got.PayloadHash != d.PayloadHash || got.Payload.Subject != "Re: Q3 invoice" {
		t.Fatalf("round trip failed: %+v %v", got, err)
	}

	// A raw UPDATE must be refused by the database itself.
	if _, err := db.Exec(`UPDATE mailcal_drafts SET payload_json = '{}' WHERE id = ?`, d.ID); err == nil {
		t.Fatal("drafts must be immutable at the database level")
	}
	if _, err := store.Get(ctx, "draft_missing"); !errors.Is(err, mailcal.ErrDraftNotFound) {
		t.Fatalf("unknown draft: %v", err)
	}
}

func TestDraftDetectsTampering(t *testing.T) {
	db := newDB(t)
	store, _ := mailcal.NewDraftStore(db)
	ctx := context.Background()
	d, _ := store.Create(ctx, mailcal.NewDraft{AccountID: "a", CreatedBy: "x", Payload: validPayload()})
	// Bypass the trigger the way an attacker with file access could.
	if _, err := db.Exec(`DROP TRIGGER mailcal_drafts_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE mailcal_drafts SET payload_json = replace(payload_json, 'alice@partner.com', 'mallory@evil.test') WHERE id = ?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, d.ID); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("a modified draft must fail its integrity check, got %v", err)
	}
}

func TestDraftValidation(t *testing.T) {
	store, _ := mailcal.NewDraftStore(newDB(t))
	ctx := context.Background()
	base := validPayload()
	mut := func(f func(*mailcal.DraftPayload)) mailcal.DraftPayload {
		p := base
		f(&p)
		return p
	}
	many := make([]abi.Address, 60)
	for i := range many {
		many[i] = abi.Address{Email: strings.Repeat("a", i+1) + "@example.com"}
	}
	cases := map[string]mailcal.DraftPayload{
		"no recipients":      mut(func(p *mailcal.DraftPayload) { p.To = nil }),
		"bad recipient":      mut(func(p *mailcal.DraftPayload) { p.To = []abi.Address{{Email: "x"}} }),
		"two in one string":  mut(func(p *mailcal.DraftPayload) { p.To = []abi.Address{{Email: "a@b.com, c@d.com"}} }),
		"bad bcc":            mut(func(p *mailcal.DraftPayload) { p.Bcc = []abi.Address{{Email: "nope"}} }),
		"too many":           mut(func(p *mailcal.DraftPayload) { p.To = many }),
		"empty subject":      mut(func(p *mailcal.DraftPayload) { p.Subject = "  " }),
		"newline in subject": mut(func(p *mailcal.DraftPayload) { p.Subject = "Hi\r\nBcc: evil@x.com" }),
		"control in subject": mut(func(p *mailcal.DraftPayload) { p.Subject = "Hi\x00" }),
		"empty body":         mut(func(p *mailcal.DraftPayload) { p.BodyText = "" }),
		"control in body":    mut(func(p *mailcal.DraftPayload) { p.BodyText = "x\x1b[31m" }),
		"huge body":          mut(func(p *mailcal.DraftPayload) { p.BodyText = strings.Repeat("x", 300*1024) }),
		"very long subject":  mut(func(p *mailcal.DraftPayload) { p.Subject = strings.Repeat("s", 1200) }),
	}
	for name, p := range cases {
		if _, err := store.Create(ctx, mailcal.NewDraft{AccountID: "a", CreatedBy: "x", Payload: p}); !errors.Is(err, mailcal.ErrInvalidDraft) {
			t.Errorf("%s: want ErrInvalidDraft, got %v", name, err)
		}
	}
	if _, err := store.Create(ctx, mailcal.NewDraft{Payload: base}); !errors.Is(err, mailcal.ErrInvalidDraft) {
		t.Errorf("missing account/creator: %v", err)
	}
}

func TestDraftRecipientsAreDeduplicatedAcrossCase(t *testing.T) {
	store, _ := mailcal.NewDraftStore(newDB(t))
	p := validPayload()
	p.To = []abi.Address{{Email: "A@x.com"}, {Email: "a@X.com"}}
	d, err := store.Create(context.Background(), mailcal.NewDraft{AccountID: "a", CreatedBy: "x", Payload: p})
	if err != nil || len(d.Payload.To) != 1 {
		t.Fatalf("duplicates must collapse: %+v %v", d.Payload.To, err)
	}
}

func TestDraftListIsNewestFirstAndScopedByAccount(t *testing.T) {
	store, _ := mailcal.NewDraftStore(newDB(t))
	ctx := context.Background()
	mk := func(acct, subject string) mailcal.Draft {
		p := validPayload()
		p.Subject = subject
		d, err := store.Create(ctx, mailcal.NewDraft{AccountID: acct, CreatedBy: "x", Payload: p})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	mk("a", "first")
	mk("b", "other account")
	last := mk("a", "second")

	got, err := store.List(ctx, "a", 10)
	if err != nil || len(got) != 2 || got[0].ID != last.ID {
		t.Fatalf("List(a) = %v, %v", got, err)
	}
	all, _ := store.List(ctx, "", 10)
	if len(all) != 3 {
		t.Fatalf("List(all) = %d drafts, want 3", len(all))
	}
	if one, _ := store.List(ctx, "a", 1); len(one) != 1 {
		t.Fatalf("limit ignored: %d", len(one))
	}
}

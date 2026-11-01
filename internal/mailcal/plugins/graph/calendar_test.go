package graph_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
)

func ev(title string) abi.Event {
	return abi.Event{Title: title, Start: time.Date(2026, 11, 3, 10, 0, 0, 0, time.FixedZone("ICT", 7*3600)),
		End: time.Date(2026, 11, 3, 11, 0, 0, 0, time.FixedZone("ICT", 7*3600)), Attendees: []abi.Attendee{{Email: "alice@partner.com"}}}
}

func TestCreateWithTheSameKeyMakesOneEventAndTimesGoOutAsUTC(t *testing.T) {
	f, b := seeded(t)
	ctx := context.Background()
	first, err := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("Sync"), IdempotencyKey: "idem_cal"})
	if err != nil || first.ID == "" {
		t.Fatalf("%+v %v", first, err)
	}
	again, err := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("Sync"), IdempotencyKey: "idem_cal"})
	if err != nil || again.ID != first.ID || len(f.events) != 1 {
		t.Fatalf("retry: %+v %v events=%d", again, err, len(f.events))
	}
	if first.Start.Hour() != 3 {
		t.Fatalf("10:00 in UTC+7 is 03:00 UTC, got %s", first.Start)
	}
}

func TestListGetUpdateRespondAndFreeBusy(t *testing.T) {
	f, b := seeded(t)
	ctx := context.Background()
	created, _ := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("Planning"), IdempotencyKey: "k"})
	list, err := b.CalendarList(ctx, acct, abi.CalListParams{From: time.Now(), To: time.Now().Add(240 * time.Hour)})
	if err != nil || len(list.Events) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	upd := ev("Planning v2")
	upd.ID = created.ID
	if out, err := b.CalendarUpdate(ctx, acct, abi.CalWriteParams{Event: upd}); err != nil || out.Title != "Planning v2" {
		t.Fatalf("%+v %v", out, err)
	}
	out, err := b.CalendarRespond(ctx, acct, abi.CalRespondParams{ID: created.ID, Response: "tentative"})
	if err != nil || len(out.Attendees) != 1 || out.Attendees[0].Status != "tentative" {
		t.Fatalf("%+v %v", out, err)
	}
	if _, err := b.CalendarRespond(ctx, acct, abi.CalRespondParams{ID: created.ID, Response: "maybe"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("bad response: %v", err)
	}
	fb, err := b.CalendarFreeBusy(ctx, acct, abi.FreeBusyParams{Emails: []string{"alice@partner.com"},
		From: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)})
	if err != nil || len(fb.Busy["alice@partner.com"]) != 1 || fb.Busy["alice@partner.com"][0].Start.Hour() != 3 {
		t.Fatalf("free spans must be dropped and emails normalized: %+v %v", fb, err)
	}
	if !strings.Contains(strings.Join(f.prefer, "|"), `outlook.timezone="UTC"`) {
		t.Fatal("calendar calls must pin the time zone to UTC")
	}
}

func TestDeleteCancelsOrganizedMeetingsOnlyWhenAttendeesAreToBeNotified(t *testing.T) {
	f, b := seeded(t)
	ctx := context.Background()
	a, _ := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("A"), IdempotencyKey: "a"})
	if err := b.CalendarDelete(ctx, acct, abi.CalDeleteParams{ID: a.ID, SendUpdates: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.requests, "\n"), "CANCELLED "+a.ID) {
		t.Fatal("an organizer who notifies attendees must cancel, not silently delete")
	}
	c, _ := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("C"), IdempotencyKey: "c"})
	if err := b.CalendarDelete(ctx, acct, abi.CalDeleteParams{ID: c.ID}); err != nil || len(f.events) != 0 {
		t.Fatalf("plain delete: %v", err)
	}
	if err := b.CalendarDelete(ctx, acct, abi.CalDeleteParams{ID: "missing"}); !mailcal.IsCode(err, abi.ErrNotFound) {
		t.Fatalf("unknown event: %v", err)
	}
}

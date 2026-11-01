package gmail_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/plugins/gmail"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

// fakeCalendar serves the slice of the Google Calendar API the provider uses.
type fakeCalendar struct {
	mu     sync.Mutex
	srv    *httptest.Server
	events map[string]map[string]any
	calls  []string
}

func newFakeCalendar(t *testing.T) *fakeCalendar {
	f := &fakeCalendar{events: map[string]map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCalendar) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI())
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	path := strings.TrimPrefix(r.URL.Path, "/calendars/primary/events")
	switch {
	case r.URL.Path == "/freeBusy":
		reply(200, map[string]any{"calendars": map[string]any{
			"alice@partner.com": map[string]any{"busy": []map[string]string{{"start": "2026-11-03T03:00:00Z", "end": "2026-11-03T04:00:00Z"}}}}})
	case path == "" && r.Method == http.MethodGet:
		var items []map[string]any
		for _, e := range f.events {
			items = append(items, e)
		}
		reply(200, map[string]any{"items": items})
	case path == "" && r.Method == http.MethodPost:
		var e map[string]any
		_ = json.NewDecoder(r.Body).Decode(&e)
		id, _ := e["id"].(string)
		if id == "" {
			id = "auto" + time.Now().Format("150405.000000")
			e["id"] = id
		}
		if _, dup := f.events[id]; dup {
			reply(409, map[string]string{"error": "duplicate"})
			return
		}
		f.events[id] = e
		reply(200, e)
	case strings.HasPrefix(path, "/"):
		id := strings.TrimPrefix(path, "/")
		e, ok := f.events[id]
		if !ok {
			reply(404, map[string]string{"error": "gone"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			reply(200, e)
		case http.MethodDelete:
			delete(f.events, id)
			w.WriteHeader(204)
		case http.MethodPut, http.MethodPatch:
			var patch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&patch)
			for k, v := range patch {
				e[k] = v
			}
			reply(200, e)
		}
	default:
		reply(501, map[string]string{"error": "unexpected"})
	}
}

func calBackend(f *fakeCalendar) *gmail.Backend {
	return &gmail.Backend{HTTP: netio.Stdlib{}, Tokens: netio.StaticToken("tok"), CalAPIBase: f.srv.URL}
}

func ev(title string) abi.Event {
	return abi.Event{Title: title, Start: time.Date(2026, 11, 3, 10, 0, 0, 0, time.FixedZone("ICT", 7*3600)),
		End: time.Date(2026, 11, 3, 11, 0, 0, 0, time.FixedZone("ICT", 7*3600)), TimeZone: "Asia/Ho_Chi_Minh",
		Attendees: []abi.Attendee{{Email: "alice@partner.com"}}}
}

func TestCreatingTheSameEventTwiceWithOneKeyMakesOneEvent(t *testing.T) {
	f := newFakeCalendar(t)
	b := calBackend(f)
	ctx := context.Background()
	first, err := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("Sync"), IdempotencyKey: "idem_cal_1", SendUpdates: true})
	if err != nil || first.ID == "" || first.Title != "Sync" {
		t.Fatalf("%+v %v", first, err)
	}
	again, err := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("Sync"), IdempotencyKey: "idem_cal_1", SendUpdates: true})
	if err != nil || again.ID != first.ID {
		t.Fatalf("the retry must return the same event: %+v %v", again, err)
	}
	if len(f.events) != 1 {
		t.Fatalf("%d events exist, want 1", len(f.events))
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "sendUpdates=all") {
		t.Fatal("attendees must be notified only when asked")
	}
}

func TestListGetUpdateDeleteRoundTrip(t *testing.T) {
	f := newFakeCalendar(t)
	b := calBackend(f)
	ctx := context.Background()
	created, _ := b.CalendarCreate(ctx, acct, abi.CalWriteParams{Event: ev("Planning"), IdempotencyKey: "k2"})
	list, err := b.CalendarList(ctx, acct, abi.CalListParams{From: time.Now(), To: time.Now().Add(240 * time.Hour)})
	if err != nil || len(list.Events) != 1 || list.Events[0].Attendees[0].Email != "alice@partner.com" {
		t.Fatalf("%+v %v", list, err)
	}
	got, err := b.CalendarGet(ctx, acct, abi.CalGetParams{ID: created.ID})
	if err != nil || got.Title != "Planning" || got.TimeZone != "Asia/Ho_Chi_Minh" {
		t.Fatalf("%+v %v", got, err)
	}
	upd := ev("Planning v2")
	upd.ID = created.ID
	if out, err := b.CalendarUpdate(ctx, acct, abi.CalWriteParams{Event: upd}); err != nil || out.Title != "Planning v2" {
		t.Fatalf("%+v %v", out, err)
	}
	if err := b.CalendarDelete(ctx, acct, abi.CalDeleteParams{ID: created.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CalendarGet(ctx, acct, abi.CalGetParams{ID: created.ID}); !mailcal.IsCode(err, abi.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestRespondChangesOnlyTheCallersOwnAnswer(t *testing.T) {
	f := newFakeCalendar(t)
	b := calBackend(f)
	f.events["inv1"] = map[string]any{"id": "inv1", "summary": "Kickoff",
		"start": map[string]any{"dateTime": "2026-11-03T10:00:00+07:00"}, "end": map[string]any{"dateTime": "2026-11-03T11:00:00+07:00"},
		"organizer": map[string]any{"email": "Alice@Partner.com"},
		"attendees": []map[string]any{{"email": "alice@partner.com", "responseStatus": "accepted"}, {"email": "me@example.com", "self": true, "responseStatus": "needsAction"}}}
	out, err := b.CalendarRespond(context.Background(), acct, abi.CalRespondParams{ID: "inv1", Response: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, a := range out.Attendees {
		status[a.Email] = a.Status
	}
	if status["me@example.com"] != "accepted" || status["alice@partner.com"] != "accepted" {
		t.Fatalf("statuses: %v", status)
	}
	if out.Organizer == nil || out.Organizer.Email != "alice@partner.com" {
		t.Fatalf("organizer must be normalized: %+v", out.Organizer)
	}
	if _, err := b.CalendarRespond(context.Background(), acct, abi.CalRespondParams{ID: "inv1", Response: "maybe"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("bad response: %v", err)
	}
	f.events["noself"] = map[string]any{"id": "noself", "attendees": []map[string]any{{"email": "x@y.com"}}}
	if _, err := b.CalendarRespond(context.Background(), acct, abi.CalRespondParams{ID: "noself", Response: "declined"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("not an invitee: %v", err)
	}
}

func TestFreeBusyMapsBusySpansByNormalizedEmail(t *testing.T) {
	f := newFakeCalendar(t)
	b := calBackend(f)
	res, err := b.CalendarFreeBusy(context.Background(), acct, abi.FreeBusyParams{Emails: []string{"alice@partner.com"},
		From: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)})
	if err != nil || len(res.Busy["alice@partner.com"]) != 1 || res.Busy["alice@partner.com"][0].End.Hour() != 4 {
		t.Fatalf("%+v %v", res, err)
	}
}

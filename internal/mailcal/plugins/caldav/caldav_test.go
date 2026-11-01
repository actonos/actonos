package caldav_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/conformance"
	"github.com/actonos/actonos/internal/mailcal/plugins/caldav"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
	"github.com/actonos/actonos/internal/plugin"
)

type fakeCalDAV struct {
	mu        sync.Mutex
	srv       *httptest.Server
	res       map[string]*resource
	rev       int
	log       []string
	reportXML string
}

type resource struct{ etag, ics string }

func newFake(t *testing.T) *fakeCalDAV {
	f := &fakeCalDAV{res: map[string]*resource{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCalDAV) url() string { return f.srv.URL + "/dav/cal/work/" }

func (f *fakeCalDAV) put(name, ics string) {
	f.rev++
	f.res[name] = &resource{etag: fmt.Sprintf(`"e%d"`, f.rev), ics: ics}
}

func (f *fakeCalDAV) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, r.Method+" "+r.URL.Path)
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@fastmail.example:app-pw"))
	if r.Header.Get("Authorization") != want {
		w.WriteHeader(401)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/dav/cal/work/")
	switch r.Method {
	case "REPORT":
		body, _ := io.ReadAll(r.Body)
		f.reportXML = string(body)
		var names []string
		for n := range f.res {
			names = append(names, n)
		}
		sort.Strings(names)
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`)
		for _, n := range names {
			sb.WriteString(`<d:response><d:href>/dav/cal/work/` + n + `</d:href><d:propstat><d:prop><d:getetag>` + f.res[n].etag +
				`</d:getetag><c:calendar-data><![CDATA[` + f.res[n].ics + `]]></c:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		}
		sb.WriteString(`</d:multistatus>`)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		_, _ = w.Write([]byte(sb.String()))
	case http.MethodGet:
		res, ok := f.res[name]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("ETag", res.etag)
		_, _ = w.Write([]byte(res.ics))
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		cur, exists := f.res[name]
		if r.Header.Get("If-None-Match") == "*" && exists {
			w.WriteHeader(412)
			return
		}
		if m := r.Header.Get("If-Match"); m != "" && (!exists || cur.etag != m) {
			w.WriteHeader(412)
			return
		}
		f.put(name, string(body))
		w.WriteHeader(201)
	case http.MethodDelete:
		if _, ok := f.res[name]; !ok {
			w.WriteHeader(404)
			return
		}
		delete(f.res, name)
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

type staticCreds struct{ user, pass string }

func (s staticCreds) Credentials(context.Context, string) (netio.Login, error) {
	return netio.Login{Username: s.user, Password: s.pass}, nil
}

func (f *fakeCalDAV) doer() netio.Doer { return netio.Stdlib{Client: f.srv.Client()} }

func (f *fakeCalDAV) backend() *caldav.Backend {
	return &caldav.Backend{HTTP: f.doer(), Creds: staticCreds{"me@fastmail.example", "app-pw"}}
}

func account(f *fakeCalDAV) abi.AccountRef {
	return abi.AccountRef{ID: "acct_dav", Config: map[string]any{"calendar_url": f.url(), "email": "Me@Example.com"}}
}

const meeting = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTIMEZONE\r\nTZID:Europe/Paris\r\nEND:VTIMEZONE\r\nBEGIN:VEVENT\r\nUID:kick-1\r\n" +
	"SUMMARY:Kickoff\\, phase 1\r\nDTSTART;TZID=Europe/Paris:20261103T100000\r\nDTEND;TZID=Europe/Paris:20261103T110000\r\n" +
	"ORGANIZER;CN=Alice:mailto:Alice@Partner.com\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:alice@partner.com\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:me@example.com\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestListGetMapEventsAndNormalize(t *testing.T) {
	f := newFake(t)
	f.put("kick-1.ics", meeting)
	b := f.backend()
	ctx := context.Background()
	list, err := b.CalendarList(ctx, account(f), abi.CalListParams{From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)})
	if err != nil || len(list.Events) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	e := list.Events[0]
	if e.ID != "kick-1.ics" || e.Title != "Kickoff, phase 1" || e.Start.Hour() != 9 || e.Organizer == nil || e.Organizer.Email != "alice@partner.com" {
		t.Fatalf("mapping: %+v", e)
	}
	if len(e.Attendees) != 2 || e.Attendees[1].Status != "needs_action" || e.Attendees[0].Status != "accepted" {
		t.Fatalf("attendees: %+v", e.Attendees)
	}
	if !strings.Contains(f.reportXML, "<c:expand") || !strings.Contains(f.reportXML, "time-range") {
		t.Fatalf("the query must ask the server to expand recurring events inside the range: %s", f.reportXML)
	}
	got, err := b.CalendarGet(ctx, account(f), abi.CalGetParams{ID: "kick-1.ics"})
	if err != nil || got.Title != e.Title {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestExpandedRecurringInstancesGetDistinctIDsAndCannotBeEdited(t *testing.T) {
	f := newFake(t)
	f.put("weekly.ics", "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:w\r\nSUMMARY:Standup\r\nRRULE:FREQ=WEEKLY\r\nDTSTART:20261103T090000Z\r\nDTEND:20261103T091500Z\r\nEND:VEVENT\r\n"+
		"BEGIN:VEVENT\r\nUID:w\r\nSUMMARY:Standup\r\nRECURRENCE-ID:20261110T090000Z\r\nDTSTART:20261110T090000Z\r\nDTEND:20261110T091500Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	b := f.backend()
	list, err := b.CalendarList(context.Background(), account(f), abi.CalListParams{From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)})
	if err != nil || len(list.Events) != 2 || list.Events[0].ID == list.Events[1].ID {
		t.Fatalf("instances need distinct ids: %+v %v", list.Events, err)
	}
	inst := list.Events[1].ID
	if err := b.CalendarDelete(context.Background(), account(f), abi.CalDeleteParams{ID: inst}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("deleting one occurrence must be refused, not delete the whole series: %v", err)
	}
	if len(f.res) != 1 {
		t.Fatal("nothing may be deleted")
	}
}

func TestCreateIsIdempotentAndNeverLeavesTheCollection(t *testing.T) {
	f := newFake(t)
	b := f.backend()
	ctx := context.Background()
	ev := abi.Event{Title: "Sync", Start: time.Date(2026, 11, 3, 3, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)}
	first, err := b.CalendarCreate(ctx, account(f), abi.CalWriteParams{Event: ev, IdempotencyKey: "idem_x"})
	if err != nil || first.Title != "Sync" || !strings.HasPrefix(first.ID, "acton-") {
		t.Fatalf("%+v %v", first, err)
	}
	// A person edits the event, then the agent's create is retried: the retry must not undo the edit.
	f.mu.Lock()
	f.put(first.ID, strings.Replace(f.res[first.ID].ics, "SUMMARY:Sync", "SUMMARY:Edited by a person", 1))
	f.mu.Unlock()
	again, err := b.CalendarCreate(ctx, account(f), abi.CalWriteParams{Event: ev, IdempotencyKey: "idem_x"})
	if err != nil || again.ID != first.ID || len(f.res) != 1 || again.Title != "Edited by a person" {
		t.Fatalf("retry: %+v %v resources=%d", again, err, len(f.res))
	}
	for _, id := range []string{"../../etc/passwd", "a/b.ics", "..", ""} {
		if _, err := b.CalendarGet(ctx, account(f), abi.CalGetParams{ID: id}); !mailcal.IsCode(err, abi.ErrInvalid) {
			t.Errorf("id %q must be refused: %v", id, err)
		}
	}
}

func TestInvitationsAreNeverSentBehindTheCallersBack(t *testing.T) {
	f := newFake(t)
	b := f.backend()
	ev := abi.Event{Title: "With guests", Start: time.Date(2026, 11, 3, 3, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC),
		Attendees: []abi.Attendee{{Email: "alice@partner.com"}}}
	if _, err := b.CalendarCreate(context.Background(), account(f), abi.CalWriteParams{Event: ev, IdempotencyKey: "k"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("the server would email the attendees on its own, so a silent create must be refused: %v", err)
	}
	if len(f.res) != 0 {
		t.Fatal("nothing may be created")
	}
	if _, err := b.CalendarCreate(context.Background(), account(f), abi.CalWriteParams{Event: ev, IdempotencyKey: "k", SendUpdates: true}); err != nil {
		t.Fatalf("with notification requested it is fine: %v", err)
	}
}

func TestUpdateKeepsAlarmsTimeZonesAndPartstatAndDetectsConflicts(t *testing.T) {
	f := newFake(t)
	f.put("kick-1.ics", meeting)
	b := f.backend()
	ctx := context.Background()
	cur, _ := b.CalendarGet(ctx, account(f), abi.CalGetParams{ID: "kick-1.ics"})
	cur.Title = "Kickoff v2"
	cur.Start, cur.End = cur.Start.Add(time.Hour), cur.End.Add(time.Hour)
	out, err := b.CalendarUpdate(ctx, account(f), abi.CalWriteParams{Event: cur, SendUpdates: true})
	if err != nil || out.Title != "Kickoff v2" || out.Start.Hour() != 10 {
		t.Fatalf("%+v %v", out, err)
	}
	ics := f.res["kick-1.ics"].ics
	if !strings.Contains(ics, "VALARM") || !strings.Contains(ics, "TZID:Europe/Paris") || !strings.Contains(ics, "PARTSTAT=ACCEPTED") {
		t.Fatalf("an edit must not drop alarms, zones or answers:\n%s", ics)
	}
	// Someone else changes the event between our read and our write.
	racer := &racingDoer{inner: f.doer(), before: func() { f.mu.Lock(); f.put("kick-1.ics", f.res["kick-1.ics"].ics+" "); f.mu.Unlock() }, onMethod: "PUT"}
	rb := &caldav.Backend{HTTP: racer, Creds: staticCreds{"me@fastmail.example", "app-pw"}}
	cur.Title = "Lost update"
	if _, err := rb.CalendarUpdate(ctx, account(f), abi.CalWriteParams{Event: cur, SendUpdates: true}); !mailcal.IsCode(err, abi.ErrTransient) {
		t.Fatalf("a changed etag must be reported as retryable, never overwritten: %v", err)
	}
	if strings.Contains(f.res["kick-1.ics"].ics, "Lost update") {
		t.Fatal("the concurrent change was overwritten")
	}
}

type racingDoer struct {
	inner    netio.Doer
	before   func()
	onMethod string
	done     bool
}

func (r *racingDoer) Do(ctx context.Context, req netio.Request) (netio.Response, error) {
	if req.Method == r.onMethod && !r.done {
		r.done = true
		r.before()
	}
	return r.inner.Do(ctx, req)
}

func TestRespondChangesOnlyOwnPartstat(t *testing.T) {
	f := newFake(t)
	f.put("kick-1.ics", meeting)
	b := f.backend()
	out, err := b.CalendarRespond(context.Background(), account(f), abi.CalRespondParams{ID: "kick-1.ics", Response: "declined"})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, a := range out.Attendees {
		status[a.Email] = a.Status
	}
	if status["me@example.com"] != "declined" || status["alice@partner.com"] != "accepted" {
		t.Fatalf("%v", status)
	}
	if _, err := b.CalendarRespond(context.Background(), account(f), abi.CalRespondParams{ID: "kick-1.ics", Response: "maybe"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("bad response: %v", err)
	}
	noEmail := abi.AccountRef{ID: "a", Config: map[string]any{"calendar_url": f.url()}}
	if _, err := b.CalendarRespond(context.Background(), noEmail, abi.CalRespondParams{ID: "kick-1.ics", Response: "accepted"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("own address is required: %v", err)
	}
}

func TestConfigurationAndAuthErrors(t *testing.T) {
	f := newFake(t)
	b := f.backend()
	ctx := context.Background()
	for _, url := range []any{"http://insecure.example/cal/", "", "https://user:pw@host/cal/", "ftp://x/"} {
		_, err := b.CalendarList(ctx, abi.AccountRef{ID: "a", Config: map[string]any{"calendar_url": url}}, abi.CalListParams{From: time.Now(), To: time.Now().Add(time.Hour)})
		if !mailcal.IsCode(err, abi.ErrInvalid) {
			t.Errorf("calendar_url %v must be refused: %v", url, err)
		}
	}
	bad := &caldav.Backend{HTTP: f.doer(), Creds: staticCreds{"me@fastmail.example", "wrong"}}
	if _, err := bad.CalendarGet(ctx, account(f), abi.CalGetParams{ID: "x.ics"}); !mailcal.IsCode(err, abi.ErrAuthExpired) {
		t.Fatalf("a rejected password is auth_expired: %v", err)
	}
	if _, err := b.CalendarGet(ctx, account(f), abi.CalGetParams{ID: "missing.ics"}); !mailcal.IsCode(err, abi.ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
}

func TestCalDAVPassesTheShapeConformanceSuite(t *testing.T) {
	f := newFake(t)
	conformance.RunShape(t, mailcal.NewBridgeProvider(kitCaller{f.backend()}), account(f))
}

type kitCaller struct{ b provkit.Backend }

func (c kitCaller) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	for _, known := range abi.AllExports {
		if known == export {
			return provkit.Dispatch(ctx, c.b, export, req), nil
		}
	}
	return nil, plugin.ErrFunctionNotFound
}

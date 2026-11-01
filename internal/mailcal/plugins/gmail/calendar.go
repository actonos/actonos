package gmail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

const defaultCalBase = "https://www.googleapis.com/calendar/v3"

func (b *Backend) calBase() string {
	if b.CalAPIBase != "" {
		return b.CalAPIBase
	}
	return defaultCalBase
}

// calDo reuses the Gmail transport (same token, same error mapping) against the Calendar base URL.
func (b *Backend) calDo(ctx context.Context, acct abi.AccountRef, method, path string, query url.Values, body, out any) error {
	return b.doAt(ctx, b.calBase(), acct, method, path, query, body, out)
}

type gTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type gEvent struct {
	ID          string   `json:"id,omitempty"`
	Status      string   `json:"status,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description,omitempty"`
	Location    string   `json:"location,omitempty"`
	Start       gTime    `json:"start"`
	End         gTime    `json:"end"`
	HangoutLink string   `json:"hangoutLink,omitempty"`
	Attendees   []gAtt   `json:"attendees,omitempty"`
	Organizer   *gPerson `json:"organizer,omitempty"`
}

type gAtt struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
	Self           bool   `json:"self,omitempty"`
}

type gPerson struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName,omitempty"`
}

func calID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "primary"
	}
	return id
}

func parseGTime(t gTime) (time.Time, bool) {
	if t.DateTime != "" {
		v, err := time.Parse(time.RFC3339, t.DateTime)
		if err == nil {
			return v, false
		}
	}
	if t.Date != "" {
		v, err := time.Parse("2006-01-02", t.Date)
		if err == nil {
			return v, true
		}
	}
	return time.Time{}, false
}

func toEvent(calendarID string, e gEvent) abi.Event {
	start, allDay := parseGTime(e.Start)
	end, _ := parseGTime(e.End)
	out := abi.Event{ID: e.ID, CalendarID: calendarID, Title: e.Summary, Start: start, End: end, TimeZone: e.Start.TimeZone,
		AllDay: allDay, Location: e.Location, Description: e.Description, ConferenceURL: e.HangoutLink}
	for _, a := range e.Attendees {
		if email, err := abi.NormalizeEmail(a.Email); err == nil {
			out.Attendees = append(out.Attendees, abi.Attendee{Email: email, Name: a.DisplayName, Status: mapStatus(a.ResponseStatus)})
		}
	}
	if e.Organizer != nil {
		if email, err := abi.NormalizeEmail(e.Organizer.Email); err == nil {
			out.Organizer = &abi.Address{Email: email, Name: e.Organizer.DisplayName}
		}
	}
	return out
}

func mapStatus(s string) string {
	if s == "needsAction" {
		return "needs_action"
	}
	return s
}

func fromEvent(ev abi.Event) gEvent {
	g := gEvent{Summary: ev.Title, Description: ev.Description, Location: ev.Location}
	if ev.AllDay {
		g.Start, g.End = gTime{Date: ev.Start.Format("2006-01-02")}, gTime{Date: ev.End.Format("2006-01-02")}
	} else {
		g.Start = gTime{DateTime: ev.Start.Format(time.RFC3339), TimeZone: ev.TimeZone}
		g.End = gTime{DateTime: ev.End.Format(time.RFC3339), TimeZone: ev.TimeZone}
	}
	for _, a := range ev.Attendees {
		g.Attendees = append(g.Attendees, gAtt{Email: a.Email, DisplayName: a.Name})
	}
	return g
}

func (b *Backend) CalendarList(ctx context.Context, acct abi.AccountRef, p abi.CalListParams) (abi.CalListResult, error) {
	q := url.Values{"timeMin": {p.From.UTC().Format(time.RFC3339)}, "timeMax": {p.To.UTC().Format(time.RFC3339)},
		"singleEvents": {"true"}, "orderBy": {"startTime"}, "maxResults": {strconv.Itoa(clampLimit(p.Limit))}}
	if p.Cursor != "" {
		q.Set("pageToken", p.Cursor)
	}
	var res struct {
		Items         []gEvent `json:"items"`
		NextPageToken string   `json:"nextPageToken"`
	}
	if err := b.calDo(ctx, acct, "GET", "/calendars/"+url.PathEscape(calID(p.CalendarID))+"/events", q, nil, &res); err != nil {
		return abi.CalListResult{}, err
	}
	out := abi.CalListResult{Events: []abi.Event{}, NextCursor: res.NextPageToken}
	for _, e := range res.Items {
		if e.Status != "cancelled" {
			out.Events = append(out.Events, toEvent(calID(p.CalendarID), e))
		}
	}
	return out, nil
}

func (b *Backend) getEvent(ctx context.Context, acct abi.AccountRef, calendarID, id string) (gEvent, error) {
	var e gEvent
	err := b.calDo(ctx, acct, "GET", "/calendars/"+url.PathEscape(calID(calendarID))+"/events/"+url.PathEscape(id), nil, nil, &e)
	return e, err
}

func (b *Backend) CalendarGet(ctx context.Context, acct abi.AccountRef, p abi.CalGetParams) (abi.Event, error) {
	e, err := b.getEvent(ctx, acct, p.CalendarID, p.ID)
	if err != nil {
		return abi.Event{}, err
	}
	return toEvent(calID(p.CalendarID), e), nil
}

func updates(send bool) url.Values {
	if send {
		return url.Values{"sendUpdates": {"all"}}
	}
	return url.Values{"sendUpdates": {"none"}}
}

// eventIDFor derives a Google event id from the idempotency key. Google accepts ids made of
// lowercase hex digits and letters a-v, 5 to 1024 long; a SHA-256 in hex always fits.
func eventIDFor(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "acton" + hex.EncodeToString(sum[:])
}

func (b *Backend) CalendarCreate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	g := fromEvent(p.Event)
	if p.IdempotencyKey != "" {
		g.ID = eventIDFor(p.IdempotencyKey)
	}
	path := "/calendars/" + url.PathEscape(calID(p.Event.CalendarID)) + "/events"
	var out gEvent
	err := b.calDo(ctx, acct, "POST", path, updates(p.SendUpdates), g, &out)
	if err != nil {
		// A duplicate id means an earlier attempt already created it: return that event.
		if e, ok := err.(*abi.Error); ok && e.Code == abi.ErrInvalid && strings.Contains(e.Message, "HTTP 409") && g.ID != "" {
			existing, gerr := b.getEvent(ctx, acct, p.Event.CalendarID, g.ID)
			if gerr != nil {
				return abi.Event{}, gerr
			}
			return toEvent(calID(p.Event.CalendarID), existing), nil
		}
		return abi.Event{}, err
	}
	return toEvent(calID(p.Event.CalendarID), out), nil
}

func (b *Backend) CalendarUpdate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	g := fromEvent(p.Event)
	var out gEvent
	path := "/calendars/" + url.PathEscape(calID(p.Event.CalendarID)) + "/events/" + url.PathEscape(p.Event.ID)
	if err := b.calDo(ctx, acct, "PUT", path, updates(p.SendUpdates), g, &out); err != nil {
		return abi.Event{}, err
	}
	return toEvent(calID(p.Event.CalendarID), out), nil
}

func (b *Backend) CalendarRespond(ctx context.Context, acct abi.AccountRef, p abi.CalRespondParams) (abi.Event, error) {
	switch p.Response {
	case "accepted", "declined", "tentative":
	default:
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "response must be accepted, declined or tentative")
	}
	e, err := b.getEvent(ctx, acct, p.CalendarID, p.ID)
	if err != nil {
		return abi.Event{}, err
	}
	found := false
	for i := range e.Attendees {
		if e.Attendees[i].Self {
			e.Attendees[i].ResponseStatus, found = p.Response, true
		}
	}
	if !found {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "you are not an invitee of this event")
	}
	var out gEvent
	path := "/calendars/" + url.PathEscape(calID(p.CalendarID)) + "/events/" + url.PathEscape(p.ID)
	if err := b.calDo(ctx, acct, "PATCH", path, updates(true), map[string]any{"attendees": e.Attendees}, &out); err != nil {
		return abi.Event{}, err
	}
	return toEvent(calID(p.CalendarID), out), nil
}

func (b *Backend) CalendarDelete(ctx context.Context, acct abi.AccountRef, p abi.CalDeleteParams) error {
	path := "/calendars/" + url.PathEscape(calID(p.CalendarID)) + "/events/" + url.PathEscape(p.ID)
	return b.calDo(ctx, acct, "DELETE", path, updates(p.SendUpdates), nil, nil)
}

func (b *Backend) CalendarFreeBusy(ctx context.Context, acct abi.AccountRef, p abi.FreeBusyParams) (abi.FreeBusyResult, error) {
	items := make([]map[string]string, 0, len(p.Emails))
	for _, e := range p.Emails {
		items = append(items, map[string]string{"id": e})
	}
	body := map[string]any{"timeMin": p.From.UTC().Format(time.RFC3339), "timeMax": p.To.UTC().Format(time.RFC3339), "items": items}
	var res struct {
		Calendars map[string]struct {
			Busy []struct{ Start, End string } `json:"busy"`
		} `json:"calendars"`
	}
	if err := b.calDo(ctx, acct, "POST", "/freeBusy", nil, body, &res); err != nil {
		return abi.FreeBusyResult{}, err
	}
	out := abi.FreeBusyResult{Busy: map[string][]abi.BusyInterval{}}
	for email, cal := range res.Calendars {
		norm, err := abi.NormalizeEmail(email)
		if err != nil {
			continue
		}
		spans := []abi.BusyInterval{}
		for _, s := range cal.Busy {
			st, e1 := time.Parse(time.RFC3339, s.Start)
			en, e2 := time.Parse(time.RFC3339, s.End)
			if e1 == nil && e2 == nil {
				spans = append(spans, abi.BusyInterval{Start: st, End: en})
			}
		}
		out.Busy[norm] = spans
	}
	return out, nil
}

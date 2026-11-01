package graph

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

const utcPrefer = `outlook.timezone="UTC"`

const eventSelect = "id,subject,bodyPreview,start,end,isAllDay,location,attendees,organizer,isOrganizer,onlineMeeting,isCancelled"

type gDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type gEvent struct {
	ID          string    `json:"id,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	BodyPreview string    `json:"bodyPreview,omitempty"`
	Start       gDateTime `json:"start"`
	End         gDateTime `json:"end"`
	IsAllDay    bool      `json:"isAllDay,omitempty"`
	IsOrganizer bool      `json:"isOrganizer,omitempty"`
	IsCancelled bool      `json:"isCancelled,omitempty"`
	Location    *struct {
		DisplayName string `json:"displayName"`
	} `json:"location,omitempty"`
	Attendees []struct {
		EmailAddress struct{ Name, Address string } `json:"emailAddress"`
		Status       struct{ Response string }      `json:"status"`
	} `json:"attendees,omitempty"`
	Organizer *gAddr `json:"organizer,omitempty"`
	Online    *struct {
		JoinURL string `json:"joinUrl"`
	} `json:"onlineMeeting,omitempty"`
}

// parseGraphTime reads Graph's local date-time ("2026-11-03T03:00:00.0000000") in the zone it
// was requested in, which this provider always sets to UTC.
func parseGraphTime(t gDateTime) time.Time {
	s := t.DateTime
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	v, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return v.UTC()
}

func mapResponse(r string) string {
	switch r {
	case "accepted", "declined":
		return r
	case "tentativelyAccepted":
		return "tentative"
	}
	return "needs_action"
}

func (g gEvent) toABI() abi.Event {
	out := abi.Event{ID: g.ID, Title: g.Subject, Description: g.BodyPreview, Start: parseGraphTime(g.Start), End: parseGraphTime(g.End),
		TimeZone: "UTC", AllDay: g.IsAllDay}
	if g.Location != nil {
		out.Location = g.Location.DisplayName
	}
	if g.Online != nil {
		out.ConferenceURL = g.Online.JoinURL
	}
	for _, a := range g.Attendees {
		if email, err := abi.NormalizeEmail(a.EmailAddress.Address); err == nil {
			out.Attendees = append(out.Attendees, abi.Attendee{Email: email, Name: a.EmailAddress.Name, Status: mapResponse(a.Status.Response)})
		}
	}
	if g.Organizer != nil {
		if n, ok := normAddr(*g.Organizer); ok {
			out.Organizer = &n
		}
	}
	return out
}

func utc(t time.Time) gDateTime {
	return gDateTime{DateTime: t.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"}
}

func (b *Backend) CalendarList(ctx context.Context, acct abi.AccountRef, p abi.CalListParams) (abi.CalListResult, error) {
	limit := clampLimit(p.Limit)
	skip := 0
	if p.Cursor != "" {
		n, err := strconv.Atoi(p.Cursor)
		if err != nil || n < 0 {
			return abi.CalListResult{}, abi.Errorf(abi.ErrInvalid, "bad cursor")
		}
		skip = n
	}
	q := url.Values{"startDateTime": {p.From.UTC().Format(time.RFC3339)}, "endDateTime": {p.To.UTC().Format(time.RFC3339)},
		"$top": {strconv.Itoa(limit + 1)}, "$skip": {strconv.Itoa(skip)}, "$orderby": {"start/dateTime"}, "$select": {eventSelect}}
	var res struct {
		Value []gEvent `json:"value"`
	}
	if err := b.do(ctx, acct, "GET", "/calendarView", q, utcPrefer, nil, &res); err != nil {
		return abi.CalListResult{}, err
	}
	out := abi.CalListResult{Events: []abi.Event{}}
	rows := res.Value
	if len(rows) > limit {
		rows, out.NextCursor = rows[:limit], strconv.Itoa(skip+limit)
	}
	for _, e := range rows {
		if !e.IsCancelled {
			out.Events = append(out.Events, e.toABI())
		}
	}
	sort.SliceStable(out.Events, func(i, j int) bool { return out.Events[i].Start.Before(out.Events[j].Start) })
	return out, nil
}

func (b *Backend) getEvent(ctx context.Context, acct abi.AccountRef, id string) (gEvent, error) {
	var e gEvent
	err := b.do(ctx, acct, "GET", "/events/"+url.PathEscape(id), url.Values{"$select": {eventSelect}}, utcPrefer, nil, &e)
	return e, err
}

func (b *Backend) CalendarGet(ctx context.Context, acct abi.AccountRef, p abi.CalGetParams) (abi.Event, error) {
	e, err := b.getEvent(ctx, acct, p.ID)
	if err != nil {
		return abi.Event{}, err
	}
	return e.toABI(), nil
}

func eventBody(ev abi.Event) map[string]any {
	body := map[string]any{"subject": ev.Title, "isAllDay": ev.AllDay, "start": utc(ev.Start), "end": utc(ev.End)}
	if ev.Description != "" {
		body["body"] = map[string]string{"contentType": "text", "content": ev.Description}
	}
	if ev.Location != "" {
		body["location"] = map[string]string{"displayName": ev.Location}
	}
	atts := make([]map[string]any, 0, len(ev.Attendees))
	for _, a := range ev.Attendees {
		atts = append(atts, map[string]any{"type": "required", "emailAddress": map[string]string{"address": a.Email, "name": a.Name}})
	}
	body["attendees"] = atts
	return body
}

func (b *Backend) CalendarCreate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	body := eventBody(p.Event)
	if p.IdempotencyKey != "" {
		body["transactionId"] = p.IdempotencyKey // Graph de-duplicates creates that carry the same transactionId
	}
	var out gEvent
	if err := b.do(ctx, acct, "POST", "/events", nil, utcPrefer, body, &out); err != nil {
		return abi.Event{}, err
	}
	return out.toABI(), nil
}

func (b *Backend) CalendarUpdate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	var out gEvent
	if err := b.do(ctx, acct, "PATCH", "/events/"+url.PathEscape(p.Event.ID), nil, utcPrefer, eventBody(p.Event), &out); err != nil {
		return abi.Event{}, err
	}
	return out.toABI(), nil
}

func (b *Backend) CalendarRespond(ctx context.Context, acct abi.AccountRef, p abi.CalRespondParams) (abi.Event, error) {
	action := map[string]string{"accepted": "accept", "declined": "decline", "tentative": "tentativelyAccept"}[p.Response]
	if action == "" {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "response must be accepted, declined or tentative")
	}
	body := map[string]any{"comment": p.Comment, "sendResponse": true}
	if err := b.do(ctx, acct, "POST", "/events/"+url.PathEscape(p.ID)+"/"+action, nil, "", body, nil); err != nil {
		return abi.Event{}, err
	}
	return b.CalendarGet(ctx, acct, abi.CalGetParams{ID: p.ID})
}

// CalendarDelete cancels (and so notifies attendees of) a meeting the account organizes, and
// deletes any other event.
func (b *Backend) CalendarDelete(ctx context.Context, acct abi.AccountRef, p abi.CalDeleteParams) error {
	e, err := b.getEvent(ctx, acct, p.ID)
	if err != nil {
		return err
	}
	if e.IsOrganizer && len(e.Attendees) > 0 && p.SendUpdates {
		return b.do(ctx, acct, "POST", "/events/"+url.PathEscape(p.ID)+"/cancel", nil, "", map[string]string{"comment": ""}, nil)
	}
	return b.do(ctx, acct, "DELETE", "/events/"+url.PathEscape(p.ID), nil, "", nil, nil)
}

func (b *Backend) CalendarFreeBusy(ctx context.Context, acct abi.AccountRef, p abi.FreeBusyParams) (abi.FreeBusyResult, error) {
	body := map[string]any{"schedules": p.Emails, "startTime": utc(p.From), "endTime": utc(p.To), "availabilityViewInterval": 30}
	var res struct {
		Value []struct {
			ScheduleID    string `json:"scheduleId"`
			ScheduleItems []struct {
				Status string    `json:"status"`
				Start  gDateTime `json:"start"`
				End    gDateTime `json:"end"`
			} `json:"scheduleItems"`
		} `json:"value"`
	}
	if err := b.do(ctx, acct, "POST", "/calendar/getSchedule", nil, utcPrefer, body, &res); err != nil {
		return abi.FreeBusyResult{}, err
	}
	out := abi.FreeBusyResult{Busy: map[string][]abi.BusyInterval{}}
	for _, v := range res.Value {
		email, err := abi.NormalizeEmail(v.ScheduleID)
		if err != nil {
			continue
		}
		spans := []abi.BusyInterval{}
		for _, it := range v.ScheduleItems {
			if it.Status == "free" {
				continue
			}
			spans = append(spans, abi.BusyInterval{Start: parseGraphTime(it.Start), End: parseGraphTime(it.End)})
		}
		out.Busy[email] = spans
	}
	return out, nil
}

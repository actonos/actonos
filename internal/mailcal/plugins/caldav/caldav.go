// Package caldav is the CalDAV calendar provider (Fastmail, iCloud, Nextcloud, Radicale, ...).
// It is calendar-only and uses the account's app password through acton_account.
package caldav

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/icskit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

// Backend implements provkit.Backend for CalDAV.
type Backend struct {
	provkit.Base
	HTTP  netio.Doer
	Creds netio.Credentials
}

var _ provkit.Backend = (*Backend)(nil)

func (b *Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapCalendar},
		Ops: []string{abi.ExportCapabilities, abi.ExportCalListEvents, abi.ExportCalGetEvent, abi.ExportCalCreateEvent,
			abi.ExportCalUpdateEvent, abi.ExportCalRespond, abi.ExportCalDeleteEvent},
		Limits: abi.Limits{MaxRecipients: 50},
		Notes:  "CalDAV; free/busy of other people is not available, invitations are sent by the server",
	}, nil
}

// collection returns the calendar collection URL from the account config, always ending in "/".
func collection(acct abi.AccountRef) (string, error) {
	raw, _ := acct.Config["calendar_url"].(string)
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", abi.Errorf(abi.ErrInvalid, "config.calendar_url must be the https URL of the calendar collection")
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

func ownEmail(acct abi.AccountRef) string {
	raw, _ := acct.Config["email"].(string)
	e, err := abi.NormalizeEmail(raw)
	if err != nil {
		return ""
	}
	return e
}

// validResource accepts only a single path segment, so an id can never leave the collection.
func validResource(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, "/\\?#\x00") && len(id) <= 200
}

type reply struct {
	status int
	etag   string
	body   []byte
}

func (b *Backend) do(ctx context.Context, acct abi.AccountRef, method, rawURL string, headers map[string]string, body []byte) (reply, *abi.Error) {
	login, err := b.Creds.Credentials(ctx, acct.ID)
	if err != nil {
		if e, ok := err.(*abi.Error); ok {
			return reply{}, e
		}
		return reply{}, abi.Errorf(abi.ErrTransient, "%v", err)
	}
	h := map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(login.Username+":"+login.Password))}
	for k, v := range headers {
		h[k] = v
	}
	req := netio.Request{Method: method, URL: rawURL, Headers: h, Body: body, TimeoutSeconds: 30}
	resp, derr := b.HTTP.Do(ctx, req)
	if derr != nil {
		if e, ok := derr.(*abi.Error); ok {
			return reply{}, e
		}
		return reply{}, abi.Errorf(abi.ErrTransient, "%s: %v", netio.Describe(req), derr)
	}
	if resp.Status == 412 {
		return reply{status: 412, body: resp.Body}, nil // callers decide what a failed precondition means
	}
	if e := netio.StatusError(resp, method); e != nil {
		return reply{}, e
	}
	return reply{status: resp.Status, etag: resp.Header("ETag"), body: resp.Body}, nil
}

// ---- reading --------------------------------------------------------------------------

type multistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Status string `xml:"status"`
			Prop   struct {
				ETag string `xml:"getetag"`
				Data string `xml:"calendar-data"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func queryBody(from, to time.Time) []byte {
	f, t := icskit.UTC(from), icskit.UTC(to)
	return []byte(`<?xml version="1.0" encoding="utf-8"?>` +
		`<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/>` +
		`<c:calendar-data><c:expand start="` + f + `" end="` + t + `"/></c:calendar-data></d:prop>` +
		`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"><c:time-range start="` + f + `" end="` + t +
		`"/></c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`)
}

func (b *Backend) CalendarList(ctx context.Context, acct abi.AccountRef, p abi.CalListParams) (abi.CalListResult, error) {
	base, err := collection(acct)
	if err != nil {
		return abi.CalListResult{}, err
	}
	if !p.To.After(p.From) {
		return abi.CalListResult{}, abi.Errorf(abi.ErrInvalid, "the end of the range must be after its start")
	}
	r, aerr := b.do(ctx, acct, "REPORT", base, map[string]string{"Depth": "1", "Content-Type": "application/xml; charset=utf-8"}, queryBody(p.From, p.To))
	if aerr != nil {
		return abi.CalListResult{}, aerr
	}
	var ms multistatus
	if err := xml.Unmarshal(r.body, &ms); err != nil {
		return abi.CalListResult{}, abi.Errorf(abi.ErrInvalid, "the server sent an unreadable answer")
	}
	out := abi.CalListResult{Events: []abi.Event{}}
	for _, resp := range ms.Responses {
		name := path.Base(strings.TrimSuffix(resp.Href, "/"))
		if !validResource(name) {
			continue
		}
		for _, ps := range resp.Propstat {
			if !strings.Contains(ps.Status, " 200") || strings.TrimSpace(ps.Prop.Data) == "" {
				continue
			}
			events, perr := parseEvents(name, ps.Prop.Data)
			if perr != nil {
				continue // one broken event must not hide the others
			}
			out.Events = append(out.Events, events...)
		}
	}
	sortEvents(out.Events)
	limit := p.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if len(out.Events) > limit {
		out.Events = out.Events[:limit] // CalDAV has no paging; the time range is the way to see more
	}
	return out, nil
}

func sortEvents(ev []abi.Event) {
	for i := 1; i < len(ev); i++ {
		for j := i; j > 0 && ev[j].Start.Before(ev[j-1].Start); j-- {
			ev[j], ev[j-1] = ev[j-1], ev[j]
		}
	}
}

func mailto(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 7 && strings.EqualFold(v[:7], "mailto:") {
		v = v[7:]
	}
	e, err := abi.NormalizeEmail(v)
	if err != nil {
		return ""
	}
	return e
}

func partstat(s string) string {
	switch strings.ToUpper(s) {
	case "ACCEPTED":
		return "accepted"
	case "DECLINED":
		return "declined"
	case "TENTATIVE":
		return "tentative"
	}
	return "needs_action"
}

// parseEvents turns one resource into events. An expanded recurring event yields several VEVENTs;
// each instance gets the id "<resource>@<recurrence-id>" so ids stay unique.
func parseEvents(resource, ics string) ([]abi.Event, error) {
	cal, err := icskit.Parse(ics)
	if err != nil {
		return nil, err
	}
	var out []abi.Event
	vevents := cal.All("VEVENT")
	for _, ve := range vevents {
		if st, ok := ve.Get("STATUS"); ok && strings.EqualFold(st.Value, "CANCELLED") {
			continue
		}
		start, allDay, err := dateOf(ve, "DTSTART")
		if err != nil {
			continue
		}
		end, _, err := dateOf(ve, "DTEND")
		if err != nil {
			end = start.Add(time.Hour)
			if allDay {
				end = start.Add(24 * time.Hour)
			}
		}
		ev := abi.Event{ID: resource, Start: start, End: end, TimeZone: "UTC", AllDay: allDay}
		if rid, ok := ve.Get("RECURRENCE-ID"); ok {
			ev.ID = resource + "@" + strings.TrimSpace(rid.Value)
		} else if len(vevents) > 1 {
			if _, hasR := ve.Get("RRULE"); hasR {
				ev.ID = resource + "@" + icskit.UTC(start)
			}
		}
		if p, ok := ve.Get("SUMMARY"); ok {
			ev.Title = icskit.Unescape(p.Value)
		}
		if p, ok := ve.Get("DESCRIPTION"); ok {
			ev.Description = icskit.Unescape(p.Value)
		}
		if p, ok := ve.Get("LOCATION"); ok {
			ev.Location = icskit.Unescape(p.Value)
		}
		if p, ok := ve.Get("ORGANIZER"); ok {
			if e := mailto(p.Value); e != "" {
				ev.Organizer = &abi.Address{Email: e, Name: p.Param("CN")}
			}
		}
		for _, a := range ve.Values("ATTENDEE") {
			if e := mailto(a.Value); e != "" {
				ev.Attendees = append(ev.Attendees, abi.Attendee{Email: e, Name: a.Param("CN"), Status: partstat(a.Param("PARTSTAT"))})
			}
		}
		out = append(out, ev)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no usable VEVENT")
	}
	return out, nil
}

func dateOf(ve *icskit.Component, name string) (time.Time, bool, error) {
	p, ok := ve.Get(name)
	if !ok {
		return time.Time{}, false, fmt.Errorf("no %s", name)
	}
	return icskit.ParseTime(p)
}

func (b *Backend) fetch(ctx context.Context, acct abi.AccountRef, id string) (*icskit.Component, string, *abi.Error) {
	base, err := collection(acct)
	if err != nil {
		return nil, "", err.(*abi.Error)
	}
	if strings.Contains(id, "@") {
		return nil, "", abi.Errorf(abi.ErrInvalid, "this is one occurrence of a recurring event; this provider cannot read or change single occurrences")
	}
	if !validResource(id) {
		return nil, "", abi.Errorf(abi.ErrInvalid, "invalid event id")
	}
	r, aerr := b.do(ctx, acct, "GET", base+url.PathEscape(id), nil, nil)
	if aerr != nil {
		return nil, "", aerr
	}
	cal, perr := icskit.Parse(string(r.body))
	if perr != nil || cal.Find("VEVENT") == nil {
		return nil, "", abi.Errorf(abi.ErrInvalid, "the server returned something that is not an event")
	}
	return cal, r.etag, nil
}

func (b *Backend) CalendarGet(ctx context.Context, acct abi.AccountRef, p abi.CalGetParams) (abi.Event, error) {
	cal, _, aerr := b.fetch(ctx, acct, p.ID)
	if aerr != nil {
		return abi.Event{}, aerr
	}
	events, err := parseEvents(p.ID, string(cal.Serialize()))
	if err != nil {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "the event cannot be read: %v", err)
	}
	return events[0], nil
}

// ---- writing --------------------------------------------------------------------------

func applyEvent(ve *icskit.Component, ev abi.Event, self string) {
	ve.Set("SUMMARY", icskit.Escape(ev.Title))
	setOrRemove := func(name, v string) {
		if v == "" {
			ve.Remove(name)
		} else {
			ve.Set(name, icskit.Escape(v))
		}
	}
	setOrRemove("DESCRIPTION", ev.Description)
	setOrRemove("LOCATION", ev.Location)
	ve.Remove("DURATION")
	if ev.AllDay {
		ve.Set("DTSTART", icskit.Date(ev.Start), icskit.Param{Name: "VALUE", Values: []string{"DATE"}})
		ve.Set("DTEND", icskit.Date(ev.End), icskit.Param{Name: "VALUE", Values: []string{"DATE"}})
	} else {
		ve.Set("DTSTART", icskit.UTC(ev.Start))
		ve.Set("DTEND", icskit.UTC(ev.End))
	}
	old := map[string]icskit.Prop{}
	for _, a := range ve.Values("ATTENDEE") {
		old[mailto(a.Value)] = a
	}
	ve.Remove("ATTENDEE")
	for _, a := range ev.Attendees {
		if prev, ok := old[a.Email]; ok {
			ve.Props = append(ve.Props, prev) // keep the answer the person already gave
			continue
		}
		ve.Props = append(ve.Props, icskit.Prop{Name: "ATTENDEE", Value: "mailto:" + a.Email, Params: []icskit.Param{
			{Name: "PARTSTAT", Values: []string{"NEEDS-ACTION"}}, {Name: "RSVP", Values: []string{"TRUE"}}}})
	}
	if len(ev.Attendees) > 0 && self != "" {
		if _, ok := ve.Get("ORGANIZER"); !ok {
			ve.Props = append(ve.Props, icskit.Prop{Name: "ORGANIZER", Value: "mailto:" + self})
		}
	}
}

func uidFor(key string, ev abi.Event) string {
	seed := key
	if seed == "" {
		seed = fmt.Sprintf("%s|%s|%s", ev.Title, icskit.UTC(ev.Start), icskit.UTC(ev.End))
	}
	sum := sha256.Sum256([]byte(seed))
	return "acton-" + hex.EncodeToString(sum[:16])
}

func (b *Backend) CalendarCreate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	base, err := collection(acct)
	if err != nil {
		return abi.Event{}, err
	}
	if !p.Event.End.After(p.Event.Start) || strings.TrimSpace(p.Event.Title) == "" {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "an event needs a title and an end after its start")
	}
	if len(p.Event.Attendees) > 0 && !p.SendUpdates {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "this server sends invitations itself, so an event with attendees cannot be created silently; ask to notify attendees")
	}
	uid := uidFor(p.IdempotencyKey, p.Event)
	ve := &icskit.Component{Name: "VEVENT"}
	ve.Set("UID", uid)
	ve.Set("DTSTAMP", icskit.UTC(time.Now()))
	applyEvent(ve, p.Event, ownEmail(acct))
	cal := &icskit.Component{Name: "VCALENDAR", Children: []*icskit.Component{ve},
		Props: []icskit.Prop{{Name: "VERSION", Value: "2.0"}, {Name: "PRODID", Value: "-//ActonOS//mailcal caldav//EN"}}}
	resource := uid + ".ics"
	r, aerr := b.do(ctx, acct, "PUT", base+url.PathEscape(resource),
		map[string]string{"Content-Type": "text/calendar; charset=utf-8", "If-None-Match": "*"}, cal.Serialize())
	if aerr != nil {
		return abi.Event{}, aerr
	}
	_ = r // 201/204 on success; 412 means an earlier attempt already created it. Either way read it back.
	return b.CalendarGet(ctx, acct, abi.CalGetParams{ID: resource})
}

func (b *Backend) CalendarUpdate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	base, err := collection(acct)
	if err != nil {
		return abi.Event{}, err
	}
	cal, etag, aerr := b.fetch(ctx, acct, p.Event.ID)
	if aerr != nil {
		return abi.Event{}, aerr
	}
	ve := cal.Find("VEVENT")
	if _, recurring := ve.Get("RRULE"); recurring {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "this provider cannot change recurring events")
	}
	if len(p.Event.Attendees) > 0 && !p.SendUpdates {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "this server sends updates itself; ask to notify attendees")
	}
	applyEvent(ve, p.Event, ownEmail(acct))
	headers := map[string]string{"Content-Type": "text/calendar; charset=utf-8"}
	if etag != "" {
		headers["If-Match"] = etag
	}
	r, aerr := b.do(ctx, acct, "PUT", base+url.PathEscape(p.Event.ID), headers, cal.Serialize())
	if aerr != nil {
		return abi.Event{}, aerr
	}
	if r.status == 412 {
		return abi.Event{}, abi.Errorf(abi.ErrTransient, "the event changed while it was being edited; read it again")
	}
	return b.CalendarGet(ctx, acct, abi.CalGetParams{ID: p.Event.ID})
}

func (b *Backend) CalendarRespond(ctx context.Context, acct abi.AccountRef, p abi.CalRespondParams) (abi.Event, error) {
	state := map[string]string{"accepted": "ACCEPTED", "declined": "DECLINED", "tentative": "TENTATIVE"}[p.Response]
	if state == "" {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "response must be accepted, declined or tentative")
	}
	self := ownEmail(acct)
	if self == "" {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "config.email (your own address) is required to answer invitations")
	}
	base, err := collection(acct)
	if err != nil {
		return abi.Event{}, err
	}
	cal, etag, aerr := b.fetch(ctx, acct, p.ID)
	if aerr != nil {
		return abi.Event{}, aerr
	}
	ve := cal.Find("VEVENT")
	found := false
	for i := range ve.Props {
		if ve.Props[i].Name == "ATTENDEE" && mailto(ve.Props[i].Value) == self {
			var params []icskit.Param
			for _, q := range ve.Props[i].Params {
				if q.Name != "PARTSTAT" && q.Name != "RSVP" {
					params = append(params, q)
				}
			}
			ve.Props[i].Params = append(params, icskit.Param{Name: "PARTSTAT", Values: []string{state}})
			found = true
		}
	}
	if !found {
		return abi.Event{}, abi.Errorf(abi.ErrInvalid, "you are not an invitee of this event")
	}
	headers := map[string]string{"Content-Type": "text/calendar; charset=utf-8"}
	if etag != "" {
		headers["If-Match"] = etag
	}
	r, aerr := b.do(ctx, acct, "PUT", base+url.PathEscape(p.ID), headers, cal.Serialize())
	if aerr != nil {
		return abi.Event{}, aerr
	}
	if r.status == 412 {
		return abi.Event{}, abi.Errorf(abi.ErrTransient, "the event changed while answering; try again")
	}
	return b.CalendarGet(ctx, acct, abi.CalGetParams{ID: p.ID})
}

func (b *Backend) CalendarDelete(ctx context.Context, acct abi.AccountRef, p abi.CalDeleteParams) error {
	base, err := collection(acct)
	if err != nil {
		return err
	}
	if strings.Contains(p.ID, "@") || !validResource(p.ID) {
		return abi.Errorf(abi.ErrInvalid, "invalid event id")
	}
	_, aerr := b.do(ctx, acct, "DELETE", base+url.PathEscape(p.ID), nil, nil)
	if aerr != nil {
		return aerr
	}
	return nil
}

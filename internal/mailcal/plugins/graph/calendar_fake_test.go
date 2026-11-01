package graph_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// calendar serves /events, /calendarView and /calendar/getSchedule for fakeGraph.
func (f *fakeGraph) calendar(w http.ResponseWriter, r *http.Request, reply func(int, any)) {
	path := r.URL.Path
	switch {
	case path == "/calendarView":
		out := []map[string]any{}
		for _, e := range f.events {
			out = append(out, e)
		}
		reply(200, map[string]any{"value": out})
	case path == "/calendar/getSchedule":
		reply(200, map[string]any{"value": []map[string]any{{"scheduleId": "Alice@Partner.com", "scheduleItems": []map[string]any{
			{"status": "busy", "start": map[string]string{"dateTime": "2026-11-03T03:00:00.0000000", "timeZone": "UTC"}, "end": map[string]string{"dateTime": "2026-11-03T04:00:00.0000000", "timeZone": "UTC"}},
			{"status": "free", "start": map[string]string{"dateTime": "2026-11-03T05:00:00.0000000", "timeZone": "UTC"}, "end": map[string]string{"dateTime": "2026-11-03T06:00:00.0000000", "timeZone": "UTC"}},
		}}}})
	case path == "/events" && r.Method == http.MethodPost:
		raw := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&raw)
		if tx, _ := raw["transactionId"].(string); tx != "" {
			if id, dup := f.seen[tx]; dup {
				reply(201, f.events[id]) // Graph answers a repeated transactionId with the original event
				return
			}
		}
		f.seq++
		id := "ev-" + strconv.Itoa(f.seq)
		raw["id"] = id
		raw["isOrganizer"] = true
		if tx, _ := raw["transactionId"].(string); tx != "" {
			f.seen[tx] = id
		}
		f.events[id] = raw
		reply(201, raw)
	case strings.HasPrefix(path, "/events/"):
		rest := strings.TrimPrefix(path, "/events/")
		id, action, _ := strings.Cut(rest, "/")
		e, ok := f.events[id]
		if !ok {
			reply(404, map[string]string{"error": "ErrorItemNotFound"})
			return
		}
		switch {
		case action == "accept" || action == "decline" || action == "tentativelyAccept":
			resp := map[string]string{"accept": "accepted", "decline": "declined", "tentativelyAccept": "tentativelyAccepted"}[action]
			if atts, ok := e["attendees"].([]any); ok {
				for _, a := range atts {
					a.(map[string]any)["status"] = map[string]string{"response": resp}
				}
			}
			reply(202, nil)
		case action == "cancel":
			delete(f.events, id)
			f.requests = append(f.requests, "CANCELLED "+id)
			reply(202, nil)
		case r.Method == http.MethodGet:
			reply(200, e)
		case r.Method == http.MethodPatch:
			patch := map[string]any{}
			_ = json.NewDecoder(r.Body).Decode(&patch)
			for k, v := range patch {
				e[k] = v
			}
			reply(200, e)
		case r.Method == http.MethodDelete:
			delete(f.events, id)
			reply(204, nil)
		}
	default:
		reply(501, map[string]string{"error": "unexpected " + r.Method + " " + path})
	}
}

package gmail_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/actonos/actonos/internal/mailcal/provkit/mimekit"
)

// fakeGmail implements the slice of the Gmail REST API the provider uses, over real HTTP.
type fakeGmail struct {
	mu           sync.Mutex
	srv          *httptest.Server
	msgs         []*fakeMsg
	seq          int
	requests     []string       // "METHOD path?query"
	queries      []url.Values   // list queries, to check what the provider asks for
	failNextSend string         // when set, the next send is stored and then answered with this status
	status       map[string]int // path prefix -> forced status (for error mapping tests)
	retryAfter   string
}

type fakeMsg struct {
	ID, ThreadID string
	Labels       []string
	Headers      [][2]string
	Parts        []fakePart
	Date         int64
}

type fakePart struct{ Mime, Filename, Data string }

func newFakeGmail(t *testing.T) *fakeGmail {
	f := &fakeGmail{status: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGmail) base() string { return f.srv.URL + "/gmail/v1/users/me" }

func (f *fakeGmail) add(id, thread string, labels []string, date int64, headers [][2]string, parts ...fakePart) *fakeMsg {
	m := &fakeMsg{ID: id, ThreadID: thread, Labels: labels, Headers: headers, Parts: parts, Date: date}
	f.msgs = append(f.msgs, m)
	return m
}

func hdr(kv ...string) [][2]string {
	var out [][2]string
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, [2]string{kv[i], kv[i+1]})
	}
	return out
}

func b64(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) }

func (m *fakeMsg) header(name string) string {
	for _, h := range m.Headers {
		if strings.EqualFold(h[0], name) {
			return h[1]
		}
	}
	return ""
}

func (m *fakeMsg) json(format string, only []string) map[string]any {
	var headers []map[string]string
	for _, h := range m.Headers {
		if format == "metadata" && len(only) > 0 {
			keep := false
			for _, o := range only {
				keep = keep || strings.EqualFold(o, h[0])
			}
			if !keep {
				continue
			}
		}
		headers = append(headers, map[string]string{"name": h[0], "value": h[1]})
	}
	payload := map[string]any{"headers": headers, "mimeType": "multipart/mixed"}
	if format == "full" {
		var parts []map[string]any
		for _, p := range m.Parts {
			part := map[string]any{"mimeType": p.Mime, "filename": p.Filename, "body": map[string]any{"data": b64(p.Data), "size": len(p.Data)}}
			if p.Filename != "" {
				part["body"] = map[string]any{"attachmentId": "att-" + p.Filename, "size": len(p.Data)}
			}
			parts = append(parts, part)
		}
		payload["parts"] = parts
	}
	return map[string]any{"id": m.ID, "threadId": m.ThreadID, "labelIds": m.Labels, "snippet": "snippet of " + m.ID,
		"internalDate": strconv.FormatInt(m.Date, 10), "payload": payload}
}

func (f *fakeGmail) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
	if r.Header.Get("Authorization") != "Bearer tok" {
		http.Error(w, `{"error":"bad token"}`, http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me")
	for prefix, code := range f.status {
		if strings.HasPrefix(path, prefix) {
			if f.retryAfter != "" {
				w.Header().Set("Retry-After", f.retryAfter)
			}
			http.Error(w, `{"error":"forced"}`, code)
			return
		}
	}
	reply := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
	q := r.URL.Query()
	switch {
	case path == "/profile":
		reply(map[string]string{"emailAddress": "Me@Example.com"})
	case path == "/messages" && r.Method == http.MethodGet:
		f.queries = append(f.queries, q)
		var ids []map[string]string
		for _, m := range f.msgs {
			if l := q.Get("labelIds"); l != "" && !contains(m.Labels, l) {
				continue
			}
			if strings.Contains(q.Get("q"), "is:unread") && !contains(m.Labels, "UNREAD") {
				continue
			}
			ids = append(ids, map[string]string{"id": m.ID, "threadId": m.ThreadID})
		}
		start := 0
		if tok := q.Get("pageToken"); tok != "" {
			start, _ = strconv.Atoi(tok)
		}
		limit, _ := strconv.Atoi(q.Get("maxResults"))
		end := len(ids)
		next := ""
		if limit > 0 && start+limit < end {
			end, next = start+limit, strconv.Itoa(start+limit)
		}
		if start > len(ids) {
			start = len(ids)
		}
		resp := map[string]any{"messages": ids[start:end]}
		if next != "" {
			resp["nextPageToken"] = next
		}
		reply(resp)
	case strings.HasPrefix(path, "/messages/") && strings.HasSuffix(path, "/trash") && r.Method == http.MethodPost:
		f.relabel(strings.TrimSuffix(strings.TrimPrefix(path, "/messages/"), "/trash"), []string{"TRASH"}, []string{"INBOX"})
		reply(map[string]string{})
	case path == "/messages/batchModify" && r.Method == http.MethodPost:
		var body struct {
			IDs                         []string
			AddLabelIds, RemoveLabelIds []string
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, id := range body.IDs {
			f.relabel(id, body.AddLabelIds, body.RemoveLabelIds)
		}
		reply(map[string]string{})
	case path == "/messages/send" && r.Method == http.MethodPost:
		f.send(w, r)
	case strings.HasPrefix(path, "/messages/") && r.Method == http.MethodGet:
		m := f.find(strings.TrimPrefix(path, "/messages/"))
		if m == nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		reply(m.json(q.Get("format"), q["metadataHeaders"]))
	case strings.HasPrefix(path, "/threads/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(path, "/threads/")
		var ms []map[string]any
		for _, m := range f.msgs {
			if m.ThreadID == id {
				ms = append(ms, m.json(q.Get("format"), nil))
			}
		}
		if len(ms) == 0 {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		reply(map[string]any{"id": id, "messages": ms})
	default:
		http.Error(w, "unexpected "+r.Method+" "+path, http.StatusNotImplemented)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (f *fakeGmail) find(id string) *fakeMsg {
	for _, m := range f.msgs {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (f *fakeGmail) relabel(id string, add, remove []string) {
	m := f.find(id)
	if m == nil {
		return
	}
	var out []string
	for _, l := range m.Labels {
		if !contains(remove, l) {
			out = append(out, l)
		}
	}
	for _, l := range add {
		if !contains(out, l) {
			out = append(out, l)
		}
	}
	m.Labels = out
}

func (f *fakeGmail) send(w http.ResponseWriter, r *http.Request) {
	var body struct{ Raw, ThreadID string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	raw, err := base64.URLEncoding.DecodeString(body.Raw)
	if err != nil {
		http.Error(w, "bad raw", http.StatusBadRequest)
		return
	}
	p, err := mimekit.Parse(raw)
	if err != nil {
		http.Error(w, "bad mime", http.StatusBadRequest)
		return
	}
	f.seq++
	id := fmt.Sprintf("sent-%d", f.seq)
	thread := body.ThreadID
	if thread == "" {
		thread = "thr-" + id
	}
	var hs [][2]string
	for k, v := range p.Header {
		hs = append(hs, [2]string{k, strings.Join(v, ", ")})
	}
	f.add(id, thread, []string{"SENT"}, 1_700_000_000_000, hs, fakePart{Mime: "text/plain", Data: p.Text})
	if f.failNextSend != "" {
		code, _ := strconv.Atoi(f.failNextSend)
		f.failNextSend = ""
		http.Error(w, `{"error":"connection lost"}`, code)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "threadId": thread})
}

func (f *fakeGmail) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		if contains(m.Labels, "SENT") {
			n++
		}
	}
	return n
}

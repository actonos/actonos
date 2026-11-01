package graph_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/conformance"
	"github.com/actonos/actonos/internal/mailcal/plugins/graph"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
	"github.com/actonos/actonos/internal/plugin"
)

var acct = abi.AccountRef{ID: "acct_ms"}

type msg struct {
	ID, Conv, Folder, Subject, Body, InternetID, InReplyTo string
	From                                                   string
	To                                                     []string
	Read                                                   bool
	At                                                     time.Time
	Key                                                    string // value of the idempotency extended property
}

type fakeGraph struct {
	mu       sync.Mutex
	srv      *httptest.Server
	msgs     []*msg
	events   map[string]map[string]any
	seen     map[string]string // transactionId -> event id
	seq      int
	requests []string
	sendFail int // answer the next N sends with 503 after performing them
	force    map[string]int
	retry    string
	prefer   []string
}

func newFake(t *testing.T) *fakeGraph {
	f := &fakeGraph{events: map[string]map[string]any{}, seen: map[string]string{}, force: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGraph) add(m msg) *msg { f.msgs = append(f.msgs, &m); return &m }

func addrJSON(e string) map[string]any {
	return map[string]any{"emailAddress": map[string]string{"name": "N " + e, "address": e}}
}

func (m *msg) json(withBody bool) map[string]any {
	to := []map[string]any{}
	for _, t := range m.To {
		to = append(to, addrJSON(t))
	}
	out := map[string]any{"id": m.ID, "conversationId": m.Conv, "from": addrJSON(m.From), "toRecipients": to, "ccRecipients": []any{},
		"subject": m.Subject, "receivedDateTime": m.At.Format(time.RFC3339), "bodyPreview": "preview " + m.ID, "isRead": m.Read,
		"internetMessageId": m.InternetID, "hasAttachments": false}
	if m.InReplyTo != "" {
		out["internetMessageHeaders"] = []map[string]string{{"name": "In-Reply-To", "value": m.InReplyTo}}
	}
	if withBody {
		out["body"] = map[string]string{"contentType": "text", "content": m.Body}
	}
	return out
}

var convFilter = regexp.MustCompile(`^conversationId eq '((?:[^']|'')*)'$`)

var folderRE = regexp.MustCompile(`^/mailFolders/([a-z]+)/messages$`)

func (f *fakeGraph) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
	f.prefer = append(f.prefer, r.Header.Get("Prefer"))
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if v != nil {
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		reply(401, map[string]string{"error": "bad token"})
		return
	}
	path := r.URL.Path
	for prefix, code := range f.force {
		if strings.HasPrefix(path, prefix) {
			if f.retry != "" {
				w.Header().Set("Retry-After", f.retry)
			}
			reply(code, map[string]string{"error": "forced"})
			return
		}
	}
	q := r.URL.Query()
	top, _ := strconv.Atoi(q.Get("$top"))
	skip, _ := strconv.Atoi(q.Get("$skip"))
	switch {
	case folderRE.MatchString(path) && r.Method == http.MethodGet:
		folder := folderRE.FindStringSubmatch(path)[1]
		var rows []*msg
		for _, m := range f.msgs {
			if m.Folder != folder {
				continue
			}
			if flt := q.Get("$filter"); strings.Contains(flt, "isRead eq false") && m.Read {
				continue
			} else if strings.Contains(flt, "singleValueExtendedProperties") && !strings.Contains(flt, "'"+m.Key+"'") {
				continue
			}
			rows = append(rows, m)
		}
		if q.Get("$orderby") == "receivedDateTime desc" {
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].At.After(rows[j].At) })
		}
		if skip < len(rows) {
			rows = rows[skip:]
		} else {
			rows = nil
		}
		if top > 0 && len(rows) > top {
			rows = rows[:top]
		}
		out := []map[string]any{}
		for _, m := range rows {
			out = append(out, m.json(false))
		}
		reply(200, map[string]any{"value": out})
	case path == "/messages" && r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, m := range f.msgs {
			if flt := q.Get("$filter"); flt != "" {
				match := convFilter.FindStringSubmatch(flt)
				if match == nil {
					reply(400, map[string]string{"error": "malformed $filter: " + flt})
					return
				}
				if strings.ReplaceAll(match[1], "''", "'") != m.Conv {
					continue
				}
			}
			if s := q.Get("$search"); s != "" && !strings.Contains(strings.ToLower(m.Subject+m.Body), strings.ToLower(strings.Trim(s, `"`))) {
				continue
			}
			out = append(out, m.json(true))
		}
		reply(200, map[string]any{"value": out})
	case path == "/messages" && r.Method == http.MethodPost:
		f.seq++
		var body struct {
			Subject, Body any
			ToRecipients  []struct{ EmailAddress struct{ Address string } }
			Extended      []struct{ ID, Value string } `json:"singleValueExtendedProperties"`
		}
		raw := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&raw)
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &body)
		m := &msg{ID: "draft-" + strconv.Itoa(f.seq), Conv: "conv-new-" + strconv.Itoa(f.seq), Folder: "drafts", From: "me@example.com", At: time.Now()}
		m.Subject, _ = raw["subject"].(string)
		if bd, ok := raw["body"].(map[string]any); ok {
			m.Body, _ = bd["content"].(string)
		}
		for _, t := range body.ToRecipients {
			m.To = append(m.To, t.EmailAddress.Address)
		}
		for _, e := range body.Extended {
			m.Key = e.Value
		}
		f.msgs = append(f.msgs, m)
		reply(201, m.json(true))
	case strings.HasSuffix(path, "/createReply") && r.Method == http.MethodPost:
		parent := f.find(strings.TrimSuffix(strings.TrimPrefix(path, "/messages/"), "/createReply"))
		if parent == nil {
			reply(404, nil)
			return
		}
		f.seq++
		m := &msg{ID: "reply-" + strconv.Itoa(f.seq), Conv: parent.Conv, Folder: "drafts", From: "me@example.com", At: time.Now(), InReplyTo: parent.InternetID}
		f.msgs = append(f.msgs, m)
		reply(201, m.json(true))
	case strings.HasSuffix(path, "/send") && r.Method == http.MethodPost:
		m := f.find(strings.TrimSuffix(strings.TrimPrefix(path, "/messages/"), "/send"))
		if m == nil {
			reply(404, nil)
			return
		}
		m.Folder = "sentitems"
		if f.sendFail > 0 {
			f.sendFail--
			reply(503, map[string]string{"error": "connection lost"})
			return
		}
		reply(202, nil)
	case strings.HasSuffix(path, "/move") && r.Method == http.MethodPost:
		m := f.find(strings.TrimSuffix(strings.TrimPrefix(path, "/messages/"), "/move"))
		if m == nil {
			reply(404, nil)
			return
		}
		var body struct {
			DestinationID string `json:"destinationId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.Folder = body.DestinationID
		reply(201, m.json(false))
	case strings.HasPrefix(path, "/messages/") && r.Method == http.MethodPatch:
		id := strings.TrimPrefix(path, "/messages/")
		m := f.find(id)
		if m == nil {
			reply(404, nil)
			return
		}
		raw := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&raw)
		if v, ok := raw["isRead"].(bool); ok {
			m.Read = v
		}
		if v, ok := raw["subject"].(string); ok {
			m.Subject = v
		}
		if bd, ok := raw["body"].(map[string]any); ok {
			m.Body, _ = bd["content"].(string)
		}
		if ext, ok := raw["singleValueExtendedProperties"].([]any); ok && len(ext) > 0 {
			m.Key, _ = ext[0].(map[string]any)["value"].(string)
		}
		if rs, ok := raw["toRecipients"].([]any); ok {
			m.To = nil
			for _, x := range rs {
				m.To = append(m.To, x.(map[string]any)["emailAddress"].(map[string]any)["address"].(string))
			}
		}
		reply(200, m.json(true))
	case strings.HasPrefix(path, "/messages/") && r.Method == http.MethodGet:
		m := f.find(strings.TrimPrefix(path, "/messages/"))
		if m == nil {
			reply(404, map[string]string{"error": "ErrorItemNotFound"})
			return
		}
		reply(200, m.json(true))
	default:
		f.calendar(w, r, reply)
	}
}

func (f *fakeGraph) find(id string) *msg {
	for _, m := range f.msgs {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (f *fakeGraph) sentWithKey(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		if m.Folder == "sentitems" && m.Key == key {
			n++
		}
	}
	return n
}

func (f *fakeGraph) count(folder string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		if m.Folder == folder {
			n++
		}
	}
	return n
}

func newBackend(f *fakeGraph) *graph.Backend {
	return &graph.Backend{HTTP: netio.Stdlib{}, Tokens: netio.StaticToken("tok"), APIBase: f.srv.URL}
}

func seeded(t *testing.T) (*fakeGraph, *graph.Backend) {
	f := newFake(t)
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	f.add(msg{ID: "m1", Conv: "c1", Folder: "inbox", Subject: "Q3 invoice", Body: "Please confirm receipt.", From: "Alice@Partner.com", To: []string{"me@example.com"}, InternetID: "<a1@partner.com>", At: now})
	f.add(msg{ID: "m2", Conv: "c2", Folder: "inbox", Subject: "Lunch?", Body: "Thursday?", From: "bob@example.org", To: []string{"me@example.com"}, Read: true, InternetID: "<b1@example.org>", At: now.Add(time.Hour)})
	f.add(msg{ID: "m3", Conv: "c1", Folder: "sentitems", Subject: "Re: Q3 invoice", Body: "Received.", From: "me@example.com", To: []string{"alice@partner.com"}, Read: true, InternetID: "<s1@example.com>", InReplyTo: "<a1@partner.com>", At: now.Add(30 * time.Minute)})
	return f, newBackend(f)
}

func TestListIsNewestFirstUsesOffsetCursorsAndNormalizesAddresses(t *testing.T) {
	f, b := seeded(t)
	ctx := context.Background()
	p1, err := b.MailList(ctx, acct, abi.MailListParams{Limit: 1})
	if err != nil || len(p1.Messages) != 1 || p1.Messages[0].ID != "m2" || p1.NextCursor != "1" {
		t.Fatalf("%+v %v", p1, err)
	}
	p2, _ := b.MailList(ctx, acct, abi.MailListParams{Limit: 1, Cursor: p1.NextCursor})
	if len(p2.Messages) != 1 || p2.Messages[0].ID != "m1" || p2.Messages[0].From.Email != "alice@partner.com" || !p2.Messages[0].Unread || p2.NextCursor != "" {
		t.Fatalf("%+v", p2)
	}
	unread, _ := b.MailList(ctx, acct, abi.MailListParams{UnreadOnly: true})
	if len(unread.Messages) != 1 || unread.Messages[0].ID != "m1" {
		t.Fatalf("unread: %+v", unread.Messages)
	}
	for _, r := range f.requests {
		if strings.Contains(r, "filter=isRead") && strings.Contains(r, "orderby") {
			t.Fatalf("Graph rejects $filter on isRead together with $orderby: %s", r)
		}
	}
	if _, err := b.MailList(ctx, acct, abi.MailListParams{Cursor: "https://evil.example/steal"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("a cursor must be a plain offset, never a URL: %v", err)
	}
	if _, err := b.MailList(ctx, acct, abi.MailListParams{Folder: "Weird"}); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("unknown folder: %v", err)
	}
}

func TestGetAsksForTextBodiesAndThreadsMapReplies(t *testing.T) {
	f, b := seeded(t)
	m, err := b.MailGet(context.Background(), acct, "m1")
	if err != nil || m.BodyText != "Please confirm receipt." {
		t.Fatalf("%+v %v", m, err)
	}
	if !strings.Contains(strings.Join(f.prefer, "|"), `outlook.body-content-type="text"`) {
		t.Fatal("bodies must be requested as text so HTML (and anything hidden in it) is converted by Microsoft")
	}
	th, err := b.MailGetThread(context.Background(), acct, "c1")
	if err != nil || len(th.Messages) != 2 || th.Messages[0].ID != "m1" || th.Messages[1].InReplyTo != "m1" {
		t.Fatalf("%+v %v", th, err)
	}
	if _, err := b.MailGetThread(context.Background(), acct, "nope"); !mailcal.IsCode(err, abi.ErrNotFound) {
		t.Fatalf("unknown conversation: %v", err)
	}
	// With the quote doubled the whole text is one literal that matches nothing; without it the
	// filter would be malformed (or, on a real server, widened).
	if _, err := b.MailGetThread(context.Background(), acct, "x' or conversationId eq 'c1"); !mailcal.IsCode(err, abi.ErrNotFound) {
		t.Fatalf("an OData injection attempt must stay a harmless literal, got %v", err)
	}
}

func TestSearchDropsOperators(t *testing.T) {
	f, b := seeded(t)
	if _, err := b.MailSearch(context.Background(), acct, abi.MailSearchParams{Query: `from:alice OR "invoice" NOT spam -x *`}); err != nil {
		t.Fatal(err)
	}
	last := f.requests[len(f.requests)-1]
	u, err := url.Parse("http://x" + strings.SplitN(last, " ", 2)[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("$search"); got != `"invoice spam"` {
		t.Fatalf("operators must be dropped and only plain words kept, Graph received %q", got)
	}
}

func send(b *graph.Backend, key string, mutate func(*abi.MailSendParams)) (abi.MailSendResult, error) {
	p := abi.MailSendParams{To: []abi.Address{{Email: "alice@partner.com"}}, Subject: "Re: Q3 invoice", BodyText: "Received, thank you.",
		InReplyTo: "m1", ThreadID: "c1", IdempotencyKey: key}
	if mutate != nil {
		mutate(&p)
	}
	return b.MailSend(context.Background(), acct, p)
}

func TestSendCreatesAReplyDraftWithExactlyTheApprovedContentThenSendsIt(t *testing.T) {
	f, b := seeded(t)
	res, err := send(b, "idem_a", nil)
	if err != nil || res.Replayed || res.ThreadID != "c1" {
		t.Fatalf("%+v %v", res, err)
	}
	m := f.find(res.ProviderMessageID)
	if m == nil || m.Folder != "sentitems" || m.Body != "Received, thank you." || len(m.To) != 1 || m.To[0] != "alice@partner.com" || m.Key != "idem_a" {
		t.Fatalf("sent message: %+v", m)
	}
	if f.count("drafts") != 0 {
		t.Fatal("no draft may be left behind")
	}
}

func TestSendingTheSameKeyTwiceSendsOnce(t *testing.T) {
	f, b := seeded(t)
	first, _ := send(b, "idem_b", nil)
	second, err := send(b, "idem_b", nil)
	if err != nil || !second.Replayed || second.ProviderMessageID != first.ProviderMessageID {
		t.Fatalf("%+v %v", second, err)
	}
	if f.sentWithKey("idem_b") != 1 {
		t.Fatal("exactly one copy may exist")
	}
}

func TestALostAnswerAfterSendingIsFoundByTheRetry(t *testing.T) {
	f, b := seeded(t)
	f.sendFail = 1
	if _, err := send(b, "idem_c", nil); !mailcal.IsCode(err, abi.ErrTransient) {
		t.Fatalf("want transient, got %v", err)
	}
	res, err := send(b, "idem_c", nil)
	if err != nil || !res.Replayed || f.sentWithKey("idem_c") != 1 {
		t.Fatalf("%+v %v copies=%d", res, err, f.sentWithKey("idem_c"))
	}
}

func TestADraftLeftByACrashedAttemptIsFinishedNotDuplicated(t *testing.T) {
	f, b := seeded(t)
	f.add(msg{ID: "stale-draft", Conv: "c1", Folder: "drafts", Subject: "Re: Q3 invoice", Body: "Received, thank you.", From: "me@example.com", To: []string{"alice@partner.com"}, Key: "idem_d", At: time.Now()})
	res, err := send(b, "idem_d", nil)
	if err != nil || res.ProviderMessageID != "stale-draft" {
		t.Fatalf("%+v %v", res, err)
	}
	if f.count("drafts") != 0 || f.sentWithKey("idem_d") != 1 {
		t.Fatal("the existing draft must be sent, not a second one created")
	}
}

func TestSendRefusesWithoutAKey(t *testing.T) {
	_, b := seeded(t)
	if _, err := send(b, "", nil); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("%v", err)
	}
	if _, err := send(b, "k", func(p *abi.MailSendParams) { p.Subject = "a\nb" }); !mailcal.IsCode(err, abi.ErrInvalid) {
		t.Fatalf("multi-line subject: %v", err)
	}
}

func TestMarkMoveDeleteNeverDestroy(t *testing.T) {
	f, b := seeded(t)
	ctx := context.Background()
	if err := b.MailMark(ctx, acct, abi.MailMarkParams{IDs: []string{"m1"}, Unread: false}); err != nil || !f.find("m1").Read {
		t.Fatalf("mark: %v", err)
	}
	if err := b.MailMove(ctx, acct, abi.MailMoveParams{IDs: []string{"m2"}, Folder: "Archive"}); err != nil || f.find("m2").Folder != "archive" {
		t.Fatalf("move: %v", err)
	}
	if err := b.MailDelete(ctx, acct, abi.MailDeleteParams{IDs: []string{"m1"}}); err != nil || f.find("m1").Folder != "deleteditems" {
		t.Fatalf("delete must move to Deleted Items: %v folder=%s", err, f.find("m1").Folder)
	}
	for _, r := range f.requests {
		if strings.HasPrefix(r, "DELETE /messages") {
			t.Fatalf("messages must never be hard-deleted: %s", r)
		}
	}
}

func TestHTTPFailuresKeepTheirMeaning(t *testing.T) {
	f, b := seeded(t)
	if _, err := b.MailGet(context.Background(), acct, "missing"); !mailcal.IsCode(err, abi.ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
	for _, tc := range []struct {
		status int
		want   abi.ErrorCode
	}{{401, abi.ErrAuthExpired}, {429, abi.ErrRateLimited}, {503, abi.ErrTransient}} {
		f.force["/mailFolders"] = tc.status
		f.retry = "5"
		_, err := b.MailList(context.Background(), acct, abi.MailListParams{})
		if !mailcal.IsCode(err, tc.want) {
			t.Errorf("HTTP %d: want %s, got %v", tc.status, tc.want, err)
		}
	}
}

func TestGraphPassesTheShapeConformanceSuite(t *testing.T) {
	_, b := seeded(t)
	conformance.RunShape(t, mailcal.NewBridgeProvider(kitCaller{b}), acct)
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

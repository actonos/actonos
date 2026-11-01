// Package graph is the Microsoft Graph provider (Outlook mail and calendar for Microsoft 365
// and outlook.com accounts). It is a provkit.Backend that talks to Graph through netio.
package graph

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

const (
	defaultBase = "https://graph.microsoft.com/v1.0/me"
	// idemProperty is a private MAPI property carrying the idempotency key. It stays on the
	// message after it is sent, which is how a retry finds its own earlier send.
	idemProperty = "String {8c3e4a5f-3c2b-4f0e-9a43-5a1c2b7d9e11} Name ActonIdem"

	messageSelect = "id,conversationId,from,toRecipients,ccRecipients,subject,receivedDateTime,bodyPreview,isRead,parentFolderId,hasAttachments"
)

// Backend implements provkit.Backend for Microsoft Graph.
type Backend struct {
	provkit.Base
	HTTP    netio.Doer
	Tokens  netio.Tokens
	APIBase string // overridden in tests
}

var _ provkit.Backend = (*Backend)(nil)

func (b *Backend) base() string {
	if b.APIBase != "" {
		return b.APIBase
	}
	return defaultBase
}

func (b *Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail, abi.CapCalendar},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList, abi.ExportMailSearch, abi.ExportMailGet, abi.ExportMailGetThread,
			abi.ExportMailSend, abi.ExportMailMark, abi.ExportMailMove, abi.ExportMailDelete,
			abi.ExportCalListEvents, abi.ExportCalGetEvent, abi.ExportCalCreateEvent, abi.ExportCalUpdateEvent,
			abi.ExportCalRespond, abi.ExportCalDeleteEvent, abi.ExportCalFreeBusy},
		Limits: abi.Limits{MaxRecipients: 100, MaxBodyBytes: 256 * 1024},
		Notes:  "Microsoft Graph",
	}, nil
}

// do performs one request. prefer is the value of the Prefer header ("" for none).
func (b *Backend) do(ctx context.Context, acct abi.AccountRef, method, path string, query url.Values, prefer string, body, out any) error {
	tok, err := b.Tokens.AccessToken(ctx, acct.ID)
	if err != nil {
		return err
	}
	u := b.base() + path
	if len(query) > 0 {
		u += "?" + strings.ReplaceAll(query.Encode(), "+", "%20")
	}
	req := netio.Request{Method: method, URL: u, TimeoutSeconds: 30,
		Headers: map[string]string{"Authorization": "Bearer " + tok, "Accept": "application/json", "Prefer": `IdType="ImmutableId"`}}
	if prefer != "" {
		req.Headers["Prefer"] += ", " + prefer
	}
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return abi.Errorf(abi.ErrInvalid, "encoding request: %v", err)
		}
		req.Body = raw
		req.Headers["Content-Type"] = "application/json"
	}
	resp, err := b.HTTP.Do(ctx, req)
	if err != nil {
		if e, ok := err.(*abi.Error); ok {
			return e
		}
		return abi.Errorf(abi.ErrTransient, "%s: %v", netio.Describe(req), err)
	}
	if e := netio.StatusError(resp, method+" "+path); e != nil {
		return e
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return abi.Errorf(abi.ErrInvalid, "Graph sent an unreadable answer: %v", err)
	}
	return nil
}

// ---- mapping --------------------------------------------------------------------------

type gAddr struct {
	EmailAddress struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type gHeader struct{ Name, Value string }

type gMessage struct {
	ID                string    `json:"id"`
	ConversationID    string    `json:"conversationId"`
	From              *gAddr    `json:"from"`
	To                []gAddr   `json:"toRecipients"`
	Cc                []gAddr   `json:"ccRecipients"`
	Subject           string    `json:"subject"`
	ReceivedDateTime  string    `json:"receivedDateTime"`
	BodyPreview       string    `json:"bodyPreview"`
	IsRead            bool      `json:"isRead"`
	ParentFolderID    string    `json:"parentFolderId"`
	HasAttachments    bool      `json:"hasAttachments"`
	InternetMessageID string    `json:"internetMessageId"`
	InternetHeaders   []gHeader `json:"internetMessageHeaders"`
	Body              *struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
}

func normAddr(a gAddr) (abi.Address, bool) {
	e, err := abi.NormalizeEmail(a.EmailAddress.Address)
	if err != nil {
		return abi.Address{}, false
	}
	return abi.Address{Name: a.EmailAddress.Name, Email: e}, true
}

func normAddrs(in []gAddr) []abi.Address {
	var out []abi.Address
	for _, a := range in {
		if n, ok := normAddr(a); ok {
			out = append(out, n)
		}
	}
	return out
}

func (m gMessage) toABI(folder string) abi.Message {
	out := abi.Message{ID: m.ID, ThreadID: m.ConversationID, Folder: folder, To: normAddrs(m.To), Cc: normAddrs(m.Cc),
		Subject: m.Subject, Snippet: m.BodyPreview, Unread: !m.IsRead}
	if m.From != nil {
		out.From, _ = normAddr(*m.From)
	}
	if t, err := time.Parse(time.RFC3339, m.ReceivedDateTime); err == nil {
		out.Date = t.UTC()
	}
	if m.HasAttachments {
		// Graph needs a second call for details; v1 never transfers attachments, so say only that one exists.
		out.Attachments = []abi.AttachmentMeta{{ID: "unknown", Name: "(attachment)"}}
	}
	return out
}

var wellKnown = map[string]string{"inbox": "inbox", "sent": "sentitems", "drafts": "drafts", "draft": "drafts", "trash": "deleteditems", "spam": "junkemail", "archive": "archive"}

func folderID(folder string) (string, bool) {
	if strings.TrimSpace(folder) == "" {
		return "inbox", true
	}
	id, ok := wellKnown[strings.ToLower(folder)]
	return id, ok
}

func folderLabel(wk string) string {
	return map[string]string{"inbox": "INBOX", "sentitems": "Sent", "drafts": "Drafts", "deleteditems": "Trash", "junkemail": "Spam", "archive": "Archive"}[wk]
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return 25
	case n > 100:
		return 100
	}
	return n
}

func odataString(s string) string { return strings.ReplaceAll(s, "'", "''") }

// ---- reads ----------------------------------------------------------------------------

func (b *Backend) MailList(ctx context.Context, acct abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error) {
	wk, ok := folderID(p.Folder)
	if !ok {
		return abi.MailListResult{}, abi.Errorf(abi.ErrInvalid, "unknown folder %q; use Inbox, Sent, Drafts, Trash, Spam or Archive", p.Folder)
	}
	limit := clampLimit(p.Limit)
	skip := 0
	if p.Cursor != "" {
		n, err := strconv.Atoi(p.Cursor)
		if err != nil || n < 0 {
			return abi.MailListResult{}, abi.Errorf(abi.ErrInvalid, "bad cursor")
		}
		skip = n
	}
	q := url.Values{"$top": {strconv.Itoa(limit + 1)}, "$skip": {strconv.Itoa(skip)}, "$select": {messageSelect}}
	if p.UnreadOnly {
		q.Set("$filter", "isRead eq false") // Graph rejects $orderby together with this filter
	} else {
		q.Set("$orderby", "receivedDateTime desc")
	}
	var res struct {
		Value []gMessage `json:"value"`
	}
	if err := b.do(ctx, acct, "GET", "/mailFolders/"+wk+"/messages", q, "", nil, &res); err != nil {
		return abi.MailListResult{}, err
	}
	return page(res.Value, limit, skip, folderLabel(wk), p.UnreadOnly), nil
}

// page turns limit+1 rows into a page and a cursor (a plain offset, never a Graph URL, so a
// cursor handed back by the model cannot point the plugin at another host).
func page(rows []gMessage, limit, skip int, folder string, sortDesc bool) abi.MailListResult {
	out := abi.MailListResult{Messages: []abi.Message{}}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = strconv.Itoa(skip + limit)
	}
	for _, m := range rows {
		out.Messages = append(out.Messages, m.toABI(folder))
	}
	if sortDesc {
		sort.SliceStable(out.Messages, func(i, j int) bool { return out.Messages[i].Date.After(out.Messages[j].Date) })
	}
	return out
}

// plainWords keeps only harmless words so a search cannot use KQL operators.
func plainWords(s string) string {
	var words []string
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, `"'()`)
		if w == "" || strings.ContainsAny(w, `:\"*`) || strings.EqualFold(w, "OR") || strings.EqualFold(w, "AND") || strings.EqualFold(w, "NOT") || strings.HasPrefix(w, "-") {
			continue
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

func (b *Backend) MailSearch(ctx context.Context, acct abi.AccountRef, p abi.MailSearchParams) (abi.MailListResult, error) {
	words := plainWords(p.Query)
	if words == "" {
		return abi.MailListResult{}, abi.Errorf(abi.ErrInvalid, "query is required")
	}
	limit := clampLimit(p.Limit)
	path := "/messages"
	if p.Folder != "" {
		wk, ok := folderID(p.Folder)
		if !ok {
			return abi.MailListResult{}, abi.Errorf(abi.ErrInvalid, "unknown folder %q", p.Folder)
		}
		path = "/mailFolders/" + wk + "/messages"
	}
	// $search cannot be combined with $skip or $orderby, so search returns one page.
	q := url.Values{"$search": {`"` + words + `"`}, "$top": {strconv.Itoa(limit)}, "$select": {messageSelect}}
	var res struct {
		Value []gMessage `json:"value"`
	}
	if err := b.do(ctx, acct, "GET", path, q, "", nil, &res); err != nil {
		return abi.MailListResult{}, err
	}
	return page(res.Value, limit, 0, "", true), nil
}

func (b *Backend) MailGet(ctx context.Context, acct abi.AccountRef, id string) (abi.Message, error) {
	q := url.Values{"$select": {messageSelect + ",body"}}
	var m gMessage
	if err := b.do(ctx, acct, "GET", "/messages/"+url.PathEscape(id), q, `outlook.body-content-type="text"`, nil, &m); err != nil {
		return abi.Message{}, err
	}
	out := m.toABI("")
	if m.Body != nil {
		out.BodyText = strings.TrimSpace(m.Body.Content)
	}
	return out, nil
}

func (b *Backend) MailGetThread(ctx context.Context, acct abi.AccountRef, threadID string) (abi.Thread, error) {
	q := url.Values{"$filter": {"conversationId eq '" + odataString(threadID) + "'"}, "$top": {"50"},
		"$select": {messageSelect + ",body,internetMessageId,internetMessageHeaders"}}
	var res struct {
		Value []gMessage `json:"value"`
	}
	if err := b.do(ctx, acct, "GET", "/messages", q, `outlook.body-content-type="text"`, nil, &res); err != nil {
		return abi.Thread{}, err
	}
	if len(res.Value) == 0 {
		return abi.Thread{}, abi.Errorf(abi.ErrNotFound, "no such conversation")
	}
	byRFC := map[string]string{}
	for _, m := range res.Value {
		byRFC[strings.Trim(m.InternetMessageID, "<> ")] = m.ID
	}
	out := abi.Thread{ID: threadID}
	for _, m := range res.Value {
		msg := m.toABI("")
		if m.Body != nil {
			msg.BodyText = strings.TrimSpace(m.Body.Content)
		}
		for _, h := range m.InternetHeaders {
			if strings.EqualFold(h.Name, "In-Reply-To") {
				msg.InReplyTo = byRFC[strings.Trim(h.Value, "<> ")]
			}
		}
		out.Messages = append(out.Messages, msg)
	}
	sort.SliceStable(out.Messages, func(i, j int) bool { return out.Messages[i].Date.Before(out.Messages[j].Date) })
	return out, nil
}

// ---- writes ---------------------------------------------------------------------------

func (b *Backend) MailMark(ctx context.Context, acct abi.AccountRef, p abi.MailMarkParams) error {
	if len(p.IDs) == 0 {
		return abi.Errorf(abi.ErrInvalid, "no message ids")
	}
	for _, id := range p.IDs {
		if err := b.do(ctx, acct, "PATCH", "/messages/"+url.PathEscape(id), nil, "", map[string]any{"isRead": !p.Unread}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backend) move(ctx context.Context, acct abi.AccountRef, ids []string, dest string) error {
	if len(ids) == 0 {
		return abi.Errorf(abi.ErrInvalid, "no message ids")
	}
	for _, id := range ids {
		if err := b.do(ctx, acct, "POST", "/messages/"+url.PathEscape(id)+"/move", nil, "", map[string]string{"destinationId": dest}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backend) MailMove(ctx context.Context, acct abi.AccountRef, p abi.MailMoveParams) error {
	wk, ok := folderID(p.Folder)
	if !ok {
		return abi.Errorf(abi.ErrInvalid, "unknown folder %q; use Inbox, Sent, Drafts, Trash, Spam or Archive", p.Folder)
	}
	return b.move(ctx, acct, p.IDs, wk)
}

// MailDelete moves to Deleted Items; nothing is destroyed.
func (b *Backend) MailDelete(ctx context.Context, acct abi.AccountRef, p abi.MailDeleteParams) error {
	return b.move(ctx, acct, p.IDs, "deleteditems")
}

func idemFilter(key string) string {
	return "singleValueExtendedProperties/Any(ep: ep/id eq '" + idemProperty + "' and ep/value eq '" + odataString(key) + "')"
}

func (b *Backend) findByKey(ctx context.Context, acct abi.AccountRef, folder, key string) (*gMessage, error) {
	q := url.Values{"$filter": {idemFilter(key)}, "$top": {"1"}, "$select": {"id,conversationId"}}
	var res struct {
		Value []gMessage `json:"value"`
	}
	if err := b.do(ctx, acct, "GET", "/mailFolders/"+folder+"/messages", q, "", nil, &res); err != nil {
		return nil, err
	}
	if len(res.Value) == 0 {
		return nil, nil
	}
	return &res.Value[0], nil
}

func recipients(in []abi.Address) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, a := range in {
		out = append(out, map[string]any{"emailAddress": map[string]string{"address": a.Email, "name": a.Name}})
	}
	return out
}

func (b *Backend) MailSend(ctx context.Context, acct abi.AccountRef, p abi.MailSendParams) (abi.MailSendResult, error) {
	key := strings.TrimSpace(p.IdempotencyKey)
	if key == "" {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrInvalid, "an idempotency key is required to send")
	}
	if strings.ContainsAny(p.Subject, "\r\n") {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrInvalid, "the subject must be a single line")
	}
	// 1. Already sent by an earlier attempt?
	if sent, err := b.findByKey(ctx, acct, "sentitems", key); err != nil {
		return abi.MailSendResult{}, err
	} else if sent != nil {
		return abi.MailSendResult{ProviderMessageID: sent.ID, ThreadID: sent.ConversationID, Replayed: true}, nil
	}
	// 2. Drafted by an earlier attempt that died before sending? Finish that one.
	draft, err := b.findByKey(ctx, acct, "drafts", key)
	if err != nil {
		return abi.MailSendResult{}, err
	}
	if draft == nil {
		if draft, err = b.createDraft(ctx, acct, p, key); err != nil {
			return abi.MailSendResult{}, err
		}
	}
	// 3. Send. Graph answers 202 with no body.
	if err := b.do(ctx, acct, "POST", "/messages/"+url.PathEscape(draft.ID)+"/send", nil, "", nil, nil); err != nil {
		return abi.MailSendResult{}, err
	}
	return abi.MailSendResult{ProviderMessageID: draft.ID, ThreadID: draft.ConversationID}, nil
}

func (b *Backend) createDraft(ctx context.Context, acct abi.AccountRef, p abi.MailSendParams, key string) (*gMessage, error) {
	content := map[string]any{
		"subject":      p.Subject,
		"body":         map[string]string{"contentType": "text", "content": p.BodyText},
		"toRecipients": recipients(p.To), "ccRecipients": recipients(p.Cc), "bccRecipients": recipients(p.Bcc),
		"singleValueExtendedProperties": []map[string]string{{"id": idemProperty, "value": key}},
	}
	var draft gMessage
	if p.InReplyTo != "" {
		// A reply draft keeps the conversation; its fields are then replaced with exactly what was approved.
		if err := b.do(ctx, acct, "POST", "/messages/"+url.PathEscape(p.InReplyTo)+"/createReply", nil, "", map[string]any{}, &draft); err != nil {
			return nil, err
		}
		if err := b.do(ctx, acct, "PATCH", "/messages/"+url.PathEscape(draft.ID), nil, "", content, &draft); err != nil {
			return nil, err
		}
		return &draft, nil
	}
	if err := b.do(ctx, acct, "POST", "/messages", nil, "", content, &draft); err != nil {
		return nil, err
	}
	return &draft, nil
}

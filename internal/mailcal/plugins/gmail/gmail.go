// Package gmail is the Gmail provider: a provkit.Backend that talks to the Gmail REST API
// through netio, so it is tested natively against a fake server and shipped as WebAssembly.
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/mimekit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

const (
	defaultBase = "https://gmail.googleapis.com/gmail/v1/users/me"
	// IdempotencyHeader is written into every sent message. Gmail has no idempotency key, so
	// the provider finds its own earlier send by this header before sending again.
	IdempotencyHeader = "X-Acton-Idempotency-Key"
)

// Backend implements provkit.Backend for Gmail.
type Backend struct {
	provkit.Base
	HTTP       netio.Doer
	Tokens     netio.Tokens
	APIBase    string // overridden in tests
	CalAPIBase string // overridden in tests

	mu       sync.Mutex
	profiles map[string]string // account id -> own address
}

var _ provkit.Backend = (*Backend)(nil)

func (b *Backend) base() string {
	if b.APIBase != "" {
		return b.APIBase
	}
	return defaultBase
}

// Capabilities declares what this provider can do: mail (this file) and calendar (calendar.go).
func (b *Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail, abi.CapCalendar},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList, abi.ExportMailSearch, abi.ExportMailGet, abi.ExportMailGetThread,
			abi.ExportMailSend, abi.ExportMailMark, abi.ExportMailMove, abi.ExportMailDelete,
			abi.ExportCalListEvents, abi.ExportCalGetEvent, abi.ExportCalCreateEvent, abi.ExportCalUpdateEvent,
			abi.ExportCalRespond, abi.ExportCalDeleteEvent, abi.ExportCalFreeBusy},
		Limits: abi.Limits{MaxRecipients: 100, MaxBodyBytes: 256 * 1024},
		Notes:  "Gmail REST API",
	}, nil
}

// ---- transport ------------------------------------------------------------------------

func (b *Backend) do(ctx context.Context, acct abi.AccountRef, method, path string, query url.Values, body any, out any) error {
	return b.doAt(ctx, b.base(), acct, method, path, query, body, out)
}

func (b *Backend) doAt(ctx context.Context, base string, acct abi.AccountRef, method, path string, query url.Values, body any, out any) error {
	tok, err := b.Tokens.AccessToken(ctx, acct.ID)
	if err != nil {
		return err
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req := netio.Request{Method: method, URL: u, Headers: map[string]string{"Authorization": "Bearer " + tok, "Accept": "application/json"}, TimeoutSeconds: 30}
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
		return abi.Errorf(abi.ErrInvalid, "Gmail sent an unreadable answer: %v", err)
	}
	return nil
}

// ---- wire types -----------------------------------------------------------------------

type gHeader struct{ Name, Value string }

type gPart struct {
	MimeType string    `json:"mimeType"`
	Filename string    `json:"filename"`
	Headers  []gHeader `json:"headers"`
	Body     struct {
		AttachmentID string `json:"attachmentId"`
		Size         int64  `json:"size"`
		Data         string `json:"data"`
	} `json:"body"`
	Parts []gPart `json:"parts"`
}

type gMessage struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"threadId"`
	LabelIDs     []string `json:"labelIds"`
	Snippet      string   `json:"snippet"`
	InternalDate string   `json:"internalDate"`
	Payload      gPart    `json:"payload"`
}

func (m gMessage) header(name string) string {
	for _, h := range m.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func (b *Backend) self(ctx context.Context, acct abi.AccountRef) (string, error) {
	b.mu.Lock()
	if e, ok := b.profiles[acct.ID]; ok {
		b.mu.Unlock()
		return e, nil
	}
	b.mu.Unlock()
	var p struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := b.do(ctx, acct, "GET", "/profile", nil, nil, &p); err != nil {
		return "", err
	}
	email, err := abi.NormalizeEmail(p.EmailAddress)
	if err != nil {
		return "", abi.Errorf(abi.ErrInvalid, "Gmail returned an unusable address")
	}
	b.mu.Lock()
	if b.profiles == nil {
		b.profiles = map[string]string{}
	}
	b.profiles[acct.ID] = email
	b.mu.Unlock()
	return email, nil
}

// ---- mapping --------------------------------------------------------------------------

var systemFolders = map[string]string{"inbox": "INBOX", "sent": "SENT", "drafts": "DRAFT", "draft": "DRAFT", "trash": "TRASH", "spam": "SPAM"}

func folderLabel(folder string) string {
	if folder == "" {
		return "INBOX"
	}
	if l, ok := systemFolders[strings.ToLower(folder)]; ok {
		return l
	}
	return ""
}

func folderName(labels []string) string {
	for _, want := range []string{"TRASH", "SPAM", "DRAFT", "SENT", "INBOX"} {
		for _, l := range labels {
			if l == want {
				return map[string]string{"TRASH": "Trash", "SPAM": "Spam", "DRAFT": "Drafts", "SENT": "Sent", "INBOX": "INBOX"}[want]
			}
		}
	}
	return ""
}

func addr(raw string) abi.Address {
	a, err := mail.ParseAddress(raw)
	if err != nil {
		return abi.Address{}
	}
	e, err := abi.NormalizeEmail(a.Address)
	if err != nil {
		return abi.Address{}
	}
	return abi.Address{Name: a.Name, Email: e}
}

func addrs(raw string) []abi.Address {
	list, err := mail.ParseAddressList(raw)
	if err != nil {
		return nil
	}
	var out []abi.Address
	for _, a := range list {
		if e, err := abi.NormalizeEmail(a.Address); err == nil {
			out = append(out, abi.Address{Name: a.Name, Email: e})
		}
	}
	return out
}

func toMessage(m gMessage, withBody bool) abi.Message {
	out := abi.Message{ID: m.ID, ThreadID: m.ThreadID, Folder: folderName(m.LabelIDs), From: addr(m.header("From")),
		To: addrs(m.header("To")), Cc: addrs(m.header("Cc")), Subject: m.header("Subject"), Snippet: m.Snippet, Labels: m.LabelIDs}
	if ms, err := strconv.ParseInt(m.InternalDate, 10, 64); err == nil {
		out.Date = time.UnixMilli(ms).UTC()
	}
	for _, l := range m.LabelIDs {
		if l == "UNREAD" {
			out.Unread = true
		}
	}
	collectAttachments(m.Payload, &out)
	if withBody {
		out.BodyText = bodyText(m.Payload)
	}
	return out
}

func collectAttachments(p gPart, out *abi.Message) {
	if p.Filename != "" && p.Body.AttachmentID != "" {
		out.Attachments = append(out.Attachments, abi.AttachmentMeta{ID: p.Body.AttachmentID, Name: p.Filename, MIME: p.MimeType, Size: p.Body.Size})
	}
	for _, c := range p.Parts {
		collectAttachments(c, out)
	}
}

func decodeB64(s string) string {
	raw, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(s)
	}
	if err != nil {
		return ""
	}
	return string(raw)
}

// bodyText prefers text/plain parts and falls back to converted HTML. Attachments are skipped.
func bodyText(p gPart) string {
	var plain, html []string
	var walk func(gPart)
	walk = func(x gPart) {
		if x.Filename != "" {
			return
		}
		switch {
		case x.MimeType == "text/plain" && x.Body.Data != "":
			plain = append(plain, decodeB64(x.Body.Data))
		case x.MimeType == "text/html" && x.Body.Data != "":
			html = append(html, decodeB64(x.Body.Data))
		}
		for _, c := range x.Parts {
			walk(c)
		}
	}
	walk(p)
	if len(plain) > 0 {
		return strings.TrimSpace(strings.Join(plain, "\n"))
	}
	return mimekit.HTMLToText(strings.Join(html, "\n"))
}

// ---- reads ----------------------------------------------------------------------------

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return 25
	case n > 100:
		return 100
	}
	return n
}

var metadataHeaders = []string{"From", "To", "Cc", "Subject", "Date", "Message-ID", "In-Reply-To", "References"}

func (b *Backend) page(ctx context.Context, acct abi.AccountRef, q url.Values) (abi.MailListResult, error) {
	var list struct {
		Messages      []struct{ ID string } `json:"messages"`
		NextPageToken string                `json:"nextPageToken"`
	}
	if err := b.do(ctx, acct, "GET", "/messages", q, nil, &list); err != nil {
		return abi.MailListResult{}, err
	}
	out := abi.MailListResult{Messages: []abi.Message{}, NextCursor: list.NextPageToken}
	for _, ref := range list.Messages {
		var m gMessage
		mq := url.Values{"format": {"metadata"}, "metadataHeaders": metadataHeaders}
		if err := b.do(ctx, acct, "GET", "/messages/"+url.PathEscape(ref.ID), mq, nil, &m); err != nil {
			if abi.Errorf(abi.ErrNotFound, "").Code == errCode(err) {
				continue // deleted between list and get
			}
			return abi.MailListResult{}, err
		}
		out.Messages = append(out.Messages, toMessage(m, false))
	}
	return out, nil
}

func errCode(err error) abi.ErrorCode {
	if e, ok := err.(*abi.Error); ok {
		return e.Code
	}
	return ""
}

func (b *Backend) MailList(ctx context.Context, acct abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error) {
	label := folderLabel(p.Folder)
	q := url.Values{"maxResults": {strconv.Itoa(clampLimit(p.Limit))}}
	if label != "" {
		q.Set("labelIds", label)
	} else {
		q.Set("q", "label:"+quoteTerm(p.Folder))
	}
	if p.UnreadOnly {
		q.Set("q", strings.TrimSpace(q.Get("q")+" is:unread"))
	}
	if p.Cursor != "" {
		q.Set("pageToken", p.Cursor)
	}
	return b.page(ctx, acct, q)
}

func (b *Backend) MailSearch(ctx context.Context, acct abi.AccountRef, p abi.MailSearchParams) (abi.MailListResult, error) {
	if strings.TrimSpace(p.Query) == "" {
		return abi.MailListResult{}, abi.Errorf(abi.ErrInvalid, "query is required")
	}
	q := url.Values{"maxResults": {strconv.Itoa(clampLimit(p.Limit))}, "q": {plainWords(p.Query)}}
	if l := folderLabel(p.Folder); p.Folder != "" && l != "" {
		q.Set("labelIds", l)
	}
	if p.Cursor != "" {
		q.Set("pageToken", p.Cursor)
	}
	return b.page(ctx, acct, q)
}

// plainWords strips Gmail operators from user text so a search term cannot widen the search
// (for example "in:anywhere") or reach other labels. Words are AND-ed.
func plainWords(s string) string {
	var words []string
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, `"'()`)
		if w == "" || strings.ContainsAny(w, ":{}") || strings.EqualFold(w, "OR") || strings.EqualFold(w, "AND") || strings.HasPrefix(w, "-") {
			continue
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

func quoteTerm(s string) string {
	return `"` + strings.NewReplacer(`"`, ``, `\`, ``).Replace(s) + `"`
}

func (b *Backend) MailGet(ctx context.Context, acct abi.AccountRef, id string) (abi.Message, error) {
	var m gMessage
	if err := b.do(ctx, acct, "GET", "/messages/"+url.PathEscape(id), url.Values{"format": {"full"}}, nil, &m); err != nil {
		return abi.Message{}, err
	}
	return toMessage(m, true), nil
}

func (b *Backend) MailGetThread(ctx context.Context, acct abi.AccountRef, threadID string) (abi.Thread, error) {
	var t struct {
		ID       string     `json:"id"`
		Messages []gMessage `json:"messages"`
	}
	if err := b.do(ctx, acct, "GET", "/threads/"+url.PathEscape(threadID), url.Values{"format": {"full"}}, nil, &t); err != nil {
		return abi.Thread{}, err
	}
	byRFC := map[string]string{}
	for _, m := range t.Messages {
		byRFC[strings.Trim(m.header("Message-ID"), "<> ")] = m.ID
	}
	out := abi.Thread{ID: t.ID, Messages: make([]abi.Message, 0, len(t.Messages))}
	for _, m := range t.Messages {
		msg := toMessage(m, true)
		msg.InReplyTo = byRFC[strings.Trim(m.header("In-Reply-To"), "<> ")] // the parent's Gmail id, not its RFC id
		out.Messages = append(out.Messages, msg)
	}
	return out, nil
}

// ---- writes ---------------------------------------------------------------------------

func (b *Backend) modify(ctx context.Context, acct abi.AccountRef, ids []string, add, remove []string) error {
	if len(ids) == 0 {
		return abi.Errorf(abi.ErrInvalid, "no message ids")
	}
	return b.do(ctx, acct, "POST", "/messages/batchModify", nil,
		map[string]any{"ids": ids, "addLabelIds": add, "removeLabelIds": remove}, nil)
}

func (b *Backend) MailMark(ctx context.Context, acct abi.AccountRef, p abi.MailMarkParams) error {
	if p.Unread {
		return b.modify(ctx, acct, p.IDs, []string{"UNREAD"}, nil)
	}
	return b.modify(ctx, acct, p.IDs, nil, []string{"UNREAD"})
}

func (b *Backend) MailMove(ctx context.Context, acct abi.AccountRef, p abi.MailMoveParams) error {
	label := folderLabel(p.Folder)
	if label == "" {
		return abi.Errorf(abi.ErrInvalid, "Gmail can only move to Inbox, Trash or Spam here; %q is not one of them", p.Folder)
	}
	remove := []string{"INBOX", "TRASH", "SPAM"}
	keep := remove[:0:0]
	for _, l := range remove {
		if l != label {
			keep = append(keep, l)
		}
	}
	return b.modify(ctx, acct, p.IDs, []string{label}, keep)
}

// MailDelete moves messages to Trash (recoverable for 30 days) instead of deleting for good.
func (b *Backend) MailDelete(ctx context.Context, acct abi.AccountRef, p abi.MailDeleteParams) error {
	if len(p.IDs) == 0 {
		return abi.Errorf(abi.ErrInvalid, "no message ids")
	}
	for _, id := range p.IDs {
		if err := b.do(ctx, acct, "POST", "/messages/"+url.PathEscape(id)+"/trash", nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

// findEarlierSend looks at recent sent mail for a message carrying key. It is what makes a
// retry after a lost answer safe.
func (b *Backend) findEarlierSend(ctx context.Context, acct abi.AccountRef, key string) (*gMessage, error) {
	var list struct {
		Messages []struct{ ID string } `json:"messages"`
	}
	q := url.Values{"labelIds": {"SENT"}, "maxResults": {"10"}, "q": {"newer_than:2d"}}
	if err := b.do(ctx, acct, "GET", "/messages", q, nil, &list); err != nil {
		return nil, err
	}
	for _, ref := range list.Messages {
		var m gMessage
		mq := url.Values{"format": {"metadata"}, "metadataHeaders": {IdempotencyHeader}}
		if err := b.do(ctx, acct, "GET", "/messages/"+url.PathEscape(ref.ID), mq, nil, &m); err != nil {
			return nil, err
		}
		if m.header(IdempotencyHeader) == key {
			return &m, nil
		}
	}
	return nil, nil
}

func (b *Backend) MailSend(ctx context.Context, acct abi.AccountRef, p abi.MailSendParams) (abi.MailSendResult, error) {
	if strings.TrimSpace(p.IdempotencyKey) == "" {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrInvalid, "an idempotency key is required to send")
	}
	if earlier, err := b.findEarlierSend(ctx, acct, p.IdempotencyKey); err != nil {
		return abi.MailSendResult{}, err
	} else if earlier != nil {
		return abi.MailSendResult{ProviderMessageID: earlier.ID, ThreadID: earlier.ThreadID, Replayed: true}, nil
	}
	self, err := b.self(ctx, acct)
	if err != nil {
		return abi.MailSendResult{}, err
	}
	msg := mimekit.Message{From: self, Subject: p.Subject, Body: p.BodyText, KeepBcc: true,
		Extra: map[string]string{IdempotencyHeader: p.IdempotencyKey}}
	for _, a := range p.To {
		msg.To = append(msg.To, a.Email)
	}
	for _, a := range p.Cc {
		msg.Cc = append(msg.Cc, a.Email)
	}
	for _, a := range p.Bcc {
		msg.Bcc = append(msg.Bcc, a.Email)
	}
	if p.InReplyTo != "" {
		var parent gMessage
		pq := url.Values{"format": {"metadata"}, "metadataHeaders": {"Message-ID", "References"}}
		if err := b.do(ctx, acct, "GET", "/messages/"+url.PathEscape(p.InReplyTo), pq, nil, &parent); err != nil {
			return abi.MailSendResult{}, err
		}
		msg.InReplyTo = strings.Trim(parent.header("Message-ID"), "<> ")
		for _, r := range strings.Fields(parent.header("References")) {
			msg.References = append(msg.References, strings.Trim(r, "<>"))
		}
	}
	raw, err := mimekit.Build(msg)
	if err != nil {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrInvalid, "building the message: %v", err)
	}
	body := map[string]any{"raw": mimekit.Base64URL(raw)}
	if p.ThreadID != "" {
		body["threadId"] = p.ThreadID
	}
	var sent struct{ ID, ThreadID string }
	if err := b.do(ctx, acct, "POST", "/messages/send", nil, body, &sent); err != nil {
		return abi.MailSendResult{}, err
	}
	return abi.MailSendResult{ProviderMessageID: sent.ID, ThreadID: sent.ThreadID}, nil
}

var _ = fmt.Sprintf

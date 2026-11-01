// Package mimekit builds and reads RFC 5322 messages for providers that speak raw mail
// (Gmail's raw send, IMAP/SMTP). It uses only the standard library and builds for WebAssembly.
package mimekit

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ErrHeaderInjection is returned when a header value contains a line break or control byte.
var ErrHeaderInjection = errors.New("header value contains a line break or control character")

// Message is what Build turns into bytes. Bcc recipients are NOT written to the headers
// unless KeepBcc is set: over SMTP they travel in the envelope, never in the message.
type Message struct {
	From       string // a bare, already normalized address
	To, Cc     []string
	Bcc        []string // written only when KeepBcc is true
	KeepBcc    bool     // for providers that take Bcc from the raw message (Gmail); SMTP must leave it false
	Subject    string
	Body       string
	MessageID  string // without angle brackets, e.g. "abc@example.com"
	InReplyTo  string // the parent's Message-ID without angle brackets
	References []string
	Date       time.Time
	Extra      map[string]string // for example X-Acton-Idempotency-Key
}

func clean(label, v string) (string, error) {
	for _, r := range v {
		if r == '\r' || r == '\n' || (unicode.IsControl(r) && r != '\t') {
			return "", fmt.Errorf("%w: %s", ErrHeaderInjection, label)
		}
	}
	return v, nil
}

func addrList(label string, list []string) (string, error) {
	out := make([]string, 0, len(list))
	for _, a := range list {
		v, err := clean(label, a)
		if err != nil {
			return "", err
		}
		parsed, err := mail.ParseAddress(v)
		if err != nil || parsed.Address != v { // only bare addresses, no display names
			return "", fmt.Errorf("%s: %q is not a plain address", label, a)
		}
		out = append(out, v)
	}
	return strings.Join(out, ", "), nil
}

// Build renders the message with a UTF-8 text/plain body in quoted-printable.
func Build(m Message) ([]byte, error) {
	if m.From == "" || len(m.To) == 0 {
		return nil, errors.New("a message needs a sender and at least one recipient")
	}
	var buf bytes.Buffer
	put := func(name, value string) { fmt.Fprintf(&buf, "%s: %s\r\n", name, value) }

	from, err := addrList("From", []string{m.From})
	if err != nil {
		return nil, err
	}
	to, err := addrList("To", m.To)
	if err != nil {
		return nil, err
	}
	put("From", from)
	put("To", to)
	if len(m.Cc) > 0 {
		cc, err := addrList("Cc", m.Cc)
		if err != nil {
			return nil, err
		}
		put("Cc", cc)
	}
	if m.KeepBcc && len(m.Bcc) > 0 {
		bcc, err := addrList("Bcc", m.Bcc)
		if err != nil {
			return nil, err
		}
		put("Bcc", bcc)
	}
	subject, err := clean("Subject", m.Subject)
	if err != nil {
		return nil, err
	}
	put("Subject", mime.QEncoding.Encode("utf-8", subject))
	date := m.Date
	if date.IsZero() {
		date = time.Now()
	}
	put("Date", date.UTC().Format(time.RFC1123Z))
	if m.MessageID != "" {
		id, err := clean("Message-ID", m.MessageID)
		if err != nil {
			return nil, err
		}
		put("Message-ID", "<"+strings.Trim(id, "<>")+">")
	}
	if m.InReplyTo != "" {
		id, err := clean("In-Reply-To", m.InReplyTo)
		if err != nil {
			return nil, err
		}
		put("In-Reply-To", "<"+strings.Trim(id, "<>")+">")
		refs := append([]string(nil), m.References...)
		refs = append(refs, id)
		wrapped := make([]string, 0, len(refs))
		for _, r := range refs {
			r, err := clean("References", r)
			if err != nil {
				return nil, err
			}
			wrapped = append(wrapped, "<"+strings.Trim(r, "<>")+">")
		}
		put("References", strings.Join(wrapped, " "))
	}
	for k, v := range m.Extra {
		if !strings.HasPrefix(strings.ToLower(k), "x-") || strings.ContainsAny(k, " :\r\n") {
			return nil, fmt.Errorf("extra header %q must start with X- and contain no spaces", k)
		}
		v, err := clean(k, v)
		if err != nil {
			return nil, err
		}
		put(k, v)
	}
	put("MIME-Version", "1.0")
	put("Content-Type", `text/plain; charset="utf-8"`)
	put("Content-Transfer-Encoding", "quoted-printable")
	buf.WriteString("\r\n")
	w := quotedprintable.NewWriter(&buf)
	_, _ = w.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n")))
	_ = w.Close()
	return buf.Bytes(), nil
}

// Base64URL encodes raw for Gmail's `raw` field.
func Base64URL(raw []byte) string { return base64.URLEncoding.EncodeToString(raw) }

// Parsed is a message reduced to what the core needs.
type Parsed struct {
	From, To, Cc string
	Subject      string
	Date         time.Time
	MessageID    string
	InReplyTo    string
	References   []string
	Text         string
	Header       mail.Header
}

var decoder = &mime.WordDecoder{CharsetReader: func(charset string, in io.Reader) (io.Reader, error) {
	// Only UTF-8 and ASCII are decoded; other charsets are passed through as bytes so a
	// message in an exotic charset degrades instead of failing.
	return in, nil
}}

// Parse reads a raw message and extracts a plain-text body (HTML is converted).
func Parse(raw []byte) (Parsed, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Parsed{}, err
	}
	h := msg.Header
	dec := func(s string) string {
		if out, err := decoder.DecodeHeader(s); err == nil {
			return out
		}
		return s
	}
	p := Parsed{From: dec(h.Get("From")), To: dec(h.Get("To")), Cc: dec(h.Get("Cc")), Subject: dec(h.Get("Subject")),
		MessageID: strings.Trim(h.Get("Message-ID"), "<> "), InReplyTo: strings.Trim(h.Get("In-Reply-To"), "<> "), Header: h}
	if d, err := h.Date(); err == nil {
		p.Date = d
	}
	for _, f := range strings.Fields(h.Get("References")) {
		p.References = append(p.References, strings.Trim(f, "<>"))
	}
	body, err := io.ReadAll(io.LimitReader(msg.Body, 8<<20))
	if err != nil {
		return Parsed{}, err
	}
	p.Text = textOf(textproto.MIMEHeader(h), body, 0)
	return p, nil
}

func decodeBody(h textproto.MIMEHeader, body []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(h.Get("Content-Transfer-Encoding"))) {
	case "quoted-printable":
		if out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body))); err == nil {
			return out
		}
	case "base64":
		clean := strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' {
				return -1
			}
			return r
		}, string(body))
		if out, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return out
		}
	}
	return body
}

// textOf walks a MIME entity and returns the best plain-text rendering.
func textOf(h textproto.MIMEHeader, body []byte, depth int) string {
	if depth > 6 {
		return ""
	}
	ct := h.Get("Content-Type")
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil || ct == "" {
		mt = "text/plain"
	}
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		var plain, html string
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			if strings.HasPrefix(strings.ToLower(part.Header.Get("Content-Disposition")), "attachment") {
				continue
			}
			data, _ := io.ReadAll(io.LimitReader(part, 8<<20))
			sub, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			text := textOf(part.Header, data, depth+1)
			switch {
			case sub == "text/html":
				if html == "" {
					html = text
				}
			case text != "" && plain == "":
				plain = text
			}
		}
		if plain != "" {
			return plain
		}
		return html
	case mt == "text/html":
		return HTMLToText(string(decodeBody(h, body)))
	case mt == "text/plain":
		return string(decodeBody(h, body))
	}
	return ""
}

var (
	hiddenBlocks = regexp.MustCompile(`(?is)<(script|style|head|title)[^>]*>.*?</\s*(script|style|head|title)\s*>`)
	hiddenStyle  = regexp.MustCompile(`(?is)<[a-z0-9]+[^>]*style\s*=\s*"[^"]*(display\s*:\s*none|visibility\s*:\s*hidden|font-size\s*:\s*0)[^"]*"[^>]*>.*?</[a-z0-9]+>`)
	comments     = regexp.MustCompile(`(?s)<!--.*?-->`)
	breaks       = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/tr|/li|/h[1-6])\s*/?>`)
	tags         = regexp.MustCompile(`(?s)<[^>]*>`)
	blankRuns    = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText converts HTML mail to readable text. It removes scripts, styles and content hidden
// with inline styles (a common place to hide instructions aimed at an AI), then the tags.
func HTMLToText(html string) string {
	s := comments.ReplaceAllString(html, "")
	s = hiddenBlocks.ReplaceAllString(s, "")
	s = hiddenStyle.ReplaceAllString(s, "")
	s = breaks.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, "")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

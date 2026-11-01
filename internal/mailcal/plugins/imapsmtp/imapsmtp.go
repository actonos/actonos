// Package imapsmtp is the provider for ordinary mail servers: IMAP for reading and organizing,
// SMTP for sending, with an app password. It is mail-only.
//
// Every operation opens its own connection, does its work and closes it, so no state leaks
// between calls and a dropped connection never poisons the next one.
package imapsmtp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/mimekit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

// IdempotencyHeader is written into every message this provider sends.
const IdempotencyHeader = "X-Acton-Idempotency-Key"

// Connection modes, as understood by the host (see wasmguest.DialTCP).
const (
	ModeTLS          = "tls"
	ModeIMAPStartTLS = "imap-starttls"
	ModeSMTPStartTLS = "smtp-starttls"
)

// DialFunc opens a connection that is already encrypted (or about to be, for the starttls modes).
type DialFunc func(ctx context.Context, host string, port int, mode string) (net.Conn, error)

// Backend implements provkit.Backend over IMAP and SMTP.
type Backend struct {
	provkit.Base
	Dial  DialFunc
	Creds netio.Credentials
}

var _ provkit.Backend = (*Backend)(nil)

func (b *Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList, abi.ExportMailSearch, abi.ExportMailGet, abi.ExportMailGetThread,
			abi.ExportMailSend, abi.ExportMailMark, abi.ExportMailMove, abi.ExportMailDelete},
		Limits: abi.Limits{MaxRecipients: 50, MaxBodyBytes: 256 * 1024},
		Notes:  "IMAP and SMTP with an app password; sending is at-most-once and needs a Sent folder",
	}, nil
}

// ---- configuration --------------------------------------------------------------------

type config struct {
	imapHost, smtpHost string
	imapPort, smtpPort int
	imapMode, smtpMode string
	email              string
	sent, trash        string
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }

func num(m map[string]any, k string, def int) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

func parseConfig(acct abi.AccountRef) (config, error) {
	c := acct.Config
	cfg := config{imapHost: str(c, "imap_host"), smtpHost: str(c, "smtp_host"), imapMode: str(c, "imap_mode"), smtpMode: str(c, "smtp_mode"),
		sent: str(c, "sent_mailbox"), trash: str(c, "trash_mailbox")}
	if cfg.imapHost == "" || cfg.smtpHost == "" {
		return cfg, abi.Errorf(abi.ErrInvalid, "config.imap_host and config.smtp_host are required")
	}
	if cfg.imapMode == "" {
		cfg.imapMode = "tls"
	}
	if cfg.smtpMode == "" {
		cfg.smtpMode = "starttls"
	}
	if cfg.imapMode != "tls" && cfg.imapMode != "starttls" || cfg.smtpMode != "tls" && cfg.smtpMode != "starttls" {
		return cfg, abi.Errorf(abi.ErrInvalid, "imap_mode and smtp_mode must be tls or starttls; there is no plaintext mode")
	}
	cfg.imapPort = num(c, "imap_port", map[string]int{"tls": 993, "starttls": 143}[cfg.imapMode])
	cfg.smtpPort = num(c, "smtp_port", map[string]int{"tls": 465, "starttls": 587}[cfg.smtpMode])
	if cfg.sent == "" {
		cfg.sent = "Sent"
	}
	if cfg.trash == "" {
		cfg.trash = "Trash"
	}
	e, err := abi.NormalizeEmail(str(c, "email"))
	if err != nil {
		return cfg, abi.Errorf(abi.ErrInvalid, "config.email (your own address) is required")
	}
	cfg.email = e
	return cfg, nil
}

func (cfg config) mailbox(folder string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(folder)) {
	case "", "inbox":
		return "INBOX", nil
	case "sent":
		return cfg.sent, nil
	case "trash":
		return cfg.trash, nil
	case "drafts":
		return "Drafts", nil
	case "spam", "junk":
		return "Junk", nil
	case "archive":
		return "Archive", nil
	}
	if strings.ContainsAny(folder, "*%\r\n\x00") || len(folder) > 100 {
		return "", abi.Errorf(abi.ErrInvalid, "invalid folder name")
	}
	return folder, nil
}

// ---- IMAP session ---------------------------------------------------------------------

type session struct {
	c   *imapclient.Client
	cfg config
}

func (s *session) close() {
	_ = s.c.Logout().Wait()
	_ = s.c.Close()
}

func (b *Backend) open(ctx context.Context, acct abi.AccountRef) (*session, error) {
	cfg, err := parseConfig(acct)
	if err != nil {
		return nil, err
	}
	login, err := b.Creds.Credentials(ctx, acct.ID)
	if err != nil {
		return nil, err
	}
	mode := ModeTLS
	if cfg.imapMode == "starttls" {
		mode = ModeIMAPStartTLS
	}
	conn, err := b.Dial(ctx, cfg.imapHost, cfg.imapPort, mode)
	if err != nil {
		return nil, mapNetErr("connecting to the IMAP server", err)
	}
	c := imapclient.New(conn, nil)
	if err := c.Login(login.Username, login.Password).Wait(); err != nil {
		_ = c.Close()
		var ie *imap.Error
		if errors.As(err, &ie) && ie.Type == imap.StatusResponseTypeNo {
			return nil, abi.Errorf(abi.ErrAuthExpired, "the IMAP server rejected the login")
		}
		return nil, mapNetErr("logging in", err)
	}
	return &session{c: c, cfg: cfg}, nil
}

func mapNetErr(what string, err error) error {
	if e, ok := err.(*abi.Error); ok {
		return e
	}
	return abi.Errorf(abi.ErrTransient, "%s: %v", what, err)
}

func (s *session) selectBox(name string, readOnly bool) (*imap.SelectData, error) {
	d, err := s.c.Select(name, &imap.SelectOptions{ReadOnly: readOnly}).Wait()
	if err != nil {
		var ie *imap.Error
		if errors.As(err, &ie) && ie.Type == imap.StatusResponseTypeNo {
			return nil, abi.Errorf(abi.ErrNotFound, "no such folder %q", name)
		}
		return nil, mapNetErr("selecting "+name, err)
	}
	return d, nil
}

// ---- ids and mapping ------------------------------------------------------------------

func makeID(mailbox string, uid imap.UID) string {
	return url.PathEscape(mailbox) + ":" + strconv.FormatUint(uint64(uid), 10)
}

func parseID(id string) (mailbox string, uid imap.UID, err error) {
	i := strings.LastIndexByte(id, ':')
	if i <= 0 {
		return "", 0, abi.Errorf(abi.ErrInvalid, "invalid message id")
	}
	mb, uerr := url.PathUnescape(id[:i])
	n, nerr := strconv.ParseUint(id[i+1:], 10, 32)
	if uerr != nil || nerr != nil || n == 0 || strings.ContainsAny(mb, "\r\n\x00") {
		return "", 0, abi.Errorf(abi.ErrInvalid, "invalid message id")
	}
	return mb, imap.UID(n), nil
}

func addrOf(a imap.Address) (abi.Address, bool) {
	e, err := abi.NormalizeEmail(a.Addr())
	if err != nil {
		return abi.Address{}, false
	}
	return abi.Address{Name: a.Name, Email: e}, true
}

func addrsOf(in []imap.Address) []abi.Address {
	var out []abi.Address
	for _, a := range in {
		if x, ok := addrOf(a); ok {
			out = append(out, x)
		}
	}
	return out
}

func hasFlag(flags []imap.Flag, f imap.Flag) bool {
	for _, x := range flags {
		if strings.EqualFold(string(x), string(f)) {
			return true
		}
	}
	return false
}

// threadRoot is the Message-ID that every message of a conversation shares: the first entry of
// References, else the In-Reply-To, else the message's own id.
func threadRoot(own string, inReplyTo []string, references string) string {
	if f := strings.Fields(references); len(f) > 0 {
		return strings.Trim(f[0], "<>")
	}
	if len(inReplyTo) > 0 {
		return strings.Trim(inReplyTo[0], "<>")
	}
	return strings.Trim(own, "<>")
}

var refsSection = &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: []string{"References"}, Peek: true}

func (s *session) toMessage(mailbox string, m *imapclient.FetchMessageBuffer) abi.Message {
	out := abi.Message{ID: makeID(mailbox, m.UID), Folder: mailbox, Date: m.InternalDate.UTC(), Unread: !hasFlag(m.Flags, imap.FlagSeen)}
	if e := m.Envelope; e != nil {
		out.Subject = e.Subject
		if len(e.From) > 0 {
			out.From, _ = addrOf(e.From[0])
		}
		out.To, out.Cc = addrsOf(e.To), addrsOf(e.Cc)
		if !e.Date.IsZero() {
			out.Date = e.Date.UTC()
		}
		refs := ""
		if sec := m.FindBodySection(refsSection); len(sec) > 0 {
			refs = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(string(sec), "\r\n ", " "), "References:"))
		}
		out.ThreadID = threadRoot(e.MessageID, e.InReplyTo, refs)
	}
	return out
}

func (s *session) fetchEnvelopes(mailbox string, uids []imap.UID) ([]abi.Message, error) {
	if len(uids) == 0 {
		return []abi.Message{}, nil
	}
	set := imap.UIDSet{}
	set.AddNum(uids...)
	bufs, err := s.c.Fetch(set, &imap.FetchOptions{Envelope: true, Flags: true, UID: true, InternalDate: true,
		BodySection: []*imap.FetchItemBodySection{refsSection}}).Collect()
	if err != nil {
		return nil, mapNetErr("fetching messages", err)
	}
	out := make([]abi.Message, 0, len(bufs))
	for _, m := range bufs {
		out = append(out, s.toMessage(mailbox, m))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
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

// page returns the newest limit UIDs after skipping offset of them, plus the next cursor.
func page(all []imap.UID, limit, offset int) ([]imap.UID, string) {
	sort.Slice(all, func(i, j int) bool { return all[i] > all[j] })
	if offset >= len(all) {
		return nil, ""
	}
	end := offset + limit
	next := ""
	if end < len(all) {
		next = strconv.Itoa(end)
	} else {
		end = len(all)
	}
	return all[offset:end], next
}

func cursorOffset(c string) (int, error) {
	if c == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(c)
	if err != nil || n < 0 {
		return 0, abi.Errorf(abi.ErrInvalid, "bad cursor")
	}
	return n, nil
}

func (b *Backend) list(ctx context.Context, acct abi.AccountRef, folder string, criteria *imap.SearchCriteria, limit int, cursor string) (abi.MailListResult, error) {
	s, err := b.open(ctx, acct)
	if err != nil {
		return abi.MailListResult{}, err
	}
	defer s.close()
	mailbox, err := s.cfg.mailbox(folder)
	if err != nil {
		return abi.MailListResult{}, err
	}
	offset, err := cursorOffset(cursor)
	if err != nil {
		return abi.MailListResult{}, err
	}
	if _, err := s.selectBox(mailbox, true); err != nil {
		return abi.MailListResult{}, err
	}
	found, err := s.c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return abi.MailListResult{}, mapNetErr("searching", err)
	}
	uids, next := page(found.AllUIDs(), clampLimit(limit), offset)
	msgs, err := s.fetchEnvelopes(mailbox, uids)
	if err != nil {
		return abi.MailListResult{}, err
	}
	return abi.MailListResult{Messages: msgs, NextCursor: next}, nil
}

func (b *Backend) MailList(ctx context.Context, acct abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error) {
	crit := &imap.SearchCriteria{}
	if p.UnreadOnly {
		crit.NotFlag = []imap.Flag{imap.FlagSeen}
	}
	return b.list(ctx, acct, p.Folder, crit, p.Limit, p.Cursor)
}

// plainWords keeps only harmless words; IMAP search is substring based so there are no operators
// to remove, but the words are bounded so a query cannot become a denial of service.
func plainWords(s string) []string {
	var words []string
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, `"'()`)
		if w != "" && len(w) <= 64 && !strings.ContainsAny(w, "\r\n\x00") {
			words = append(words, w)
		}
		if len(words) == 8 {
			break
		}
	}
	return words
}

func (b *Backend) MailSearch(ctx context.Context, acct abi.AccountRef, p abi.MailSearchParams) (abi.MailListResult, error) {
	words := plainWords(p.Query)
	if len(words) == 0 {
		return abi.MailListResult{}, abi.Errorf(abi.ErrInvalid, "query is required")
	}
	return b.list(ctx, acct, p.Folder, &imap.SearchCriteria{Text: words}, p.Limit, p.Cursor)
}

const maxBody = 512 * 1024

func (b *Backend) MailGet(ctx context.Context, acct abi.AccountRef, id string) (abi.Message, error) {
	mailbox, uid, err := parseID(id)
	if err != nil {
		return abi.Message{}, err
	}
	s, err := b.open(ctx, acct)
	if err != nil {
		return abi.Message{}, err
	}
	defer s.close()
	if _, err := s.selectBox(mailbox, true); err != nil {
		return abi.Message{}, err
	}
	set := imap.UIDSet{}
	set.AddNum(uid)
	body := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: maxBody}}
	bufs, err := s.c.Fetch(set, &imap.FetchOptions{Envelope: true, Flags: true, UID: true, InternalDate: true,
		BodySection: []*imap.FetchItemBodySection{body, refsSection}}).Collect()
	if err != nil {
		return abi.Message{}, mapNetErr("fetching the message", err)
	}
	if len(bufs) == 0 {
		return abi.Message{}, abi.Errorf(abi.ErrNotFound, "no such message")
	}
	msg := s.toMessage(mailbox, bufs[0])
	if raw := bufs[0].FindBodySection(body); len(raw) > 0 {
		if parsed, perr := mimekit.Parse(raw); perr == nil {
			msg.BodyText = strings.TrimSpace(parsed.Text)
		}
	}
	return msg, nil
}

func (b *Backend) MailGetThread(ctx context.Context, acct abi.AccountRef, threadID string) (abi.Thread, error) {
	root := strings.Trim(strings.TrimSpace(threadID), "<>")
	if root == "" || strings.ContainsAny(root, " \r\n\x00") || len(root) > 200 {
		return abi.Thread{}, abi.Errorf(abi.ErrInvalid, "invalid thread id")
	}
	s, err := b.open(ctx, acct)
	if err != nil {
		return abi.Thread{}, err
	}
	defer s.close()
	field := func(n string) imap.SearchCriteria {
		return imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: n, Value: root}}}
	}
	inner := imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{field("References"), field("In-Reply-To")}}}
	crit := &imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{field("Message-ID"), inner}}}
	var all []abi.Message
	for _, mb := range []string{"INBOX", s.cfg.sent} {
		if _, err := s.selectBox(mb, true); err != nil {
			if mailcodeIs(err, abi.ErrNotFound) {
				continue
			}
			return abi.Thread{}, err
		}
		found, err := s.c.UIDSearch(crit, nil).Wait()
		if err != nil {
			return abi.Thread{}, mapNetErr("searching the conversation", err)
		}
		uids := found.AllUIDs()
		if len(uids) > 50 {
			uids = uids[len(uids)-50:]
		}
		msgs, err := s.fetchEnvelopes(mb, uids)
		if err != nil {
			return abi.Thread{}, err
		}
		all = append(all, msgs...)
	}
	if len(all) == 0 {
		return abi.Thread{}, abi.Errorf(abi.ErrNotFound, "no such conversation")
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Date.Before(all[j].Date) })
	return abi.Thread{ID: root, Messages: all}, nil
}

func mailcodeIs(err error, code abi.ErrorCode) bool {
	e, ok := err.(*abi.Error)
	return ok && e.Code == code
}

// ---- organizing -----------------------------------------------------------------------

// byMailbox groups ids so one connection handles each folder once.
func byMailbox(ids []string) (map[string]imap.UIDSet, error) {
	if len(ids) == 0 {
		return nil, abi.Errorf(abi.ErrInvalid, "no message ids")
	}
	out := map[string]imap.UIDSet{}
	for _, id := range ids {
		mb, uid, err := parseID(id)
		if err != nil {
			return nil, err
		}
		set := out[mb]
		set.AddNum(uid)
		out[mb] = set
	}
	return out, nil
}

func (b *Backend) MailMark(ctx context.Context, acct abi.AccountRef, p abi.MailMarkParams) error {
	groups, err := byMailbox(p.IDs)
	if err != nil {
		return err
	}
	s, err := b.open(ctx, acct)
	if err != nil {
		return err
	}
	defer s.close()
	op := imap.StoreFlagsAdd
	if p.Unread {
		op = imap.StoreFlagsDel
	}
	for mb, set := range groups {
		if _, err := s.selectBox(mb, false); err != nil {
			return err
		}
		if err := s.c.Store(set, &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}, nil).Close(); err != nil {
			return mapNetErr("marking messages", err)
		}
	}
	return nil
}

func (b *Backend) moveTo(ctx context.Context, acct abi.AccountRef, ids []string, destFolder string) error {
	groups, err := byMailbox(ids)
	if err != nil {
		return err
	}
	s, err := b.open(ctx, acct)
	if err != nil {
		return err
	}
	defer s.close()
	dest, err := s.cfg.mailbox(destFolder)
	if err != nil {
		return err
	}
	for mb, set := range groups {
		if _, err := s.selectBox(mb, false); err != nil {
			return err
		}
		if _, err := s.c.Move(set, dest).Wait(); err != nil {
			return mapNetErr("moving messages", err)
		}
	}
	return nil
}

func (b *Backend) MailMove(ctx context.Context, acct abi.AccountRef, p abi.MailMoveParams) error {
	return b.moveTo(ctx, acct, p.IDs, p.Folder)
}

// MailDelete moves messages to the Trash folder; nothing is expunged for good.
func (b *Backend) MailDelete(ctx context.Context, acct abi.AccountRef, p abi.MailDeleteParams) error {
	return b.moveTo(ctx, acct, p.IDs, "Trash")
}

// ---- sending --------------------------------------------------------------------------

func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return email[i+1:]
	}
	return "acton.invalid"
}

func messageIDFor(key, from string) string {
	sum := sha256.Sum256([]byte(key))
	return "acton-" + hex.EncodeToString(sum[:12]) + "@" + domainOf(from)
}

// MailSend is at-most-once. A copy of the exact message is appended to the Sent folder with the
// \Draft flag BEFORE it is submitted, and the flag is cleared only after the server accepted it.
// A retry that finds the copy un-flagged reports a replay; one that finds it still flagged cannot
// know whether delivery happened and says so instead of risking a duplicate.
func (b *Backend) MailSend(ctx context.Context, acct abi.AccountRef, p abi.MailSendParams) (abi.MailSendResult, error) {
	key := strings.TrimSpace(p.IdempotencyKey)
	if key == "" {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrInvalid, "an idempotency key is required to send")
	}
	cfg, err := parseConfig(acct)
	if err != nil {
		return abi.MailSendResult{}, err
	}
	msg := mimekit.Message{From: cfg.email, Subject: p.Subject, Body: p.BodyText, MessageID: messageIDFor(key, cfg.email),
		Extra: map[string]string{IdempotencyHeader: key}}
	var rcpt []string
	for _, a := range p.To {
		msg.To = append(msg.To, a.Email)
		rcpt = append(rcpt, a.Email)
	}
	for _, a := range p.Cc {
		msg.Cc = append(msg.Cc, a.Email)
		rcpt = append(rcpt, a.Email)
	}
	for _, a := range p.Bcc {
		rcpt = append(rcpt, a.Email) // envelope only: Build never writes Bcc without KeepBcc
	}
	s, err := b.open(ctx, acct)
	if err != nil {
		return abi.MailSendResult{}, err
	}
	defer s.close()
	if p.InReplyTo != "" {
		if err := b.threadHeaders(s, p.InReplyTo, &msg); err != nil {
			return abi.MailSendResult{}, err
		}
	}
	raw, err := mimekit.Build(msg)
	if err != nil {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrInvalid, "building the message: %v", err)
	}

	// 1. Did an earlier attempt already get this far?
	if _, err := s.selectBox(cfg.sent, false); err != nil {
		return abi.MailSendResult{}, abi.Errorf(abi.ErrUnsupported, "sending needs a %q folder on the server (config.sent_mailbox): %v", cfg.sent, err)
	}
	found, err := s.c.UIDSearch(&imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: IdempotencyHeader, Value: key}}}, nil).Wait()
	if err != nil {
		return abi.MailSendResult{}, mapNetErr("looking for an earlier send", err)
	}
	if uids := found.AllUIDs(); len(uids) > 0 {
		set := imap.UIDSet{}
		set.AddNum(uids[0])
		bufs, ferr := s.c.Fetch(set, &imap.FetchOptions{Flags: true, UID: true}).Collect()
		if ferr != nil || len(bufs) == 0 {
			return abi.MailSendResult{}, mapNetErr("reading the earlier copy", ferr)
		}
		if hasFlag(bufs[0].Flags, imap.FlagDraft) {
			return abi.MailSendResult{}, abi.Errorf(abi.ErrTransient,
				"an earlier attempt to send this message may or may not have been delivered; check the %q folder before sending again", cfg.sent)
		}
		return abi.MailSendResult{ProviderMessageID: makeID(cfg.sent, uids[0]), ThreadID: threadRoot(msg.MessageID, nil, ""), Replayed: true}, nil
	}

	// 2. Park a copy, flagged as not yet delivered.
	app := s.c.Append(cfg.sent, int64(len(raw)), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagDraft}, Time: time.Now()})
	if _, err := app.Write(raw); err != nil {
		return abi.MailSendResult{}, mapNetErr("saving a copy", err)
	}
	if err := app.Close(); err != nil {
		return abi.MailSendResult{}, mapNetErr("saving a copy", err)
	}
	appended, err := app.Wait()
	if err != nil {
		return abi.MailSendResult{}, mapNetErr("saving a copy", err)
	}

	// 3. Submit.
	definitelyNotSent, serr := b.smtpSend(ctx, acct, cfg, rcpt, raw)
	set := imap.UIDSet{}
	set.AddNum(appended.UID)
	if serr != nil {
		if definitelyNotSent {
			// Nothing left the building: remove the parked copy so a retry starts clean.
			_ = s.c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
			_ = s.c.UIDExpunge(set).Close()
		}
		return abi.MailSendResult{}, serr
	}

	// 4. Delivered: clear the marker.
	_ = s.c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsDel, Silent: true, Flags: []imap.Flag{imap.FlagDraft}}, nil).Close()
	_ = s.c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}, nil).Close()
	return abi.MailSendResult{ProviderMessageID: makeID(cfg.sent, appended.UID), ThreadID: threadRoot(msg.MessageID, nil, "")}, nil
}

// threadHeaders reads the parent's Message-ID and References so the reply continues its thread.
func (b *Backend) threadHeaders(s *session, parentID string, msg *mimekit.Message) error {
	mb, uid, err := parseID(parentID)
	if err != nil {
		return err
	}
	if _, err := s.selectBox(mb, true); err != nil {
		return err
	}
	set := imap.UIDSet{}
	set.AddNum(uid)
	bufs, err := s.c.Fetch(set, &imap.FetchOptions{Envelope: true, BodySection: []*imap.FetchItemBodySection{refsSection}}).Collect()
	if err != nil {
		return mapNetErr("reading the message being answered", err)
	}
	if len(bufs) == 0 || bufs[0].Envelope == nil {
		return abi.Errorf(abi.ErrNotFound, "the message being answered no longer exists")
	}
	msg.InReplyTo = strings.Trim(bufs[0].Envelope.MessageID, "<>")
	if sec := bufs[0].FindBodySection(refsSection); len(sec) > 0 {
		for _, r := range strings.Fields(strings.TrimPrefix(strings.ReplaceAll(string(sec), "\r\n ", " "), "References:")) {
			msg.References = append(msg.References, strings.Trim(r, "<>"))
		}
	}
	return nil
}

// smtpSend submits raw. definitelyNotSent is true only for failures before the message body was
// handed over, when delivery cannot have happened.
func (b *Backend) smtpSend(ctx context.Context, acct abi.AccountRef, cfg config, rcpt []string, raw []byte) (definitelyNotSent bool, err error) {
	login, err := b.Creds.Credentials(ctx, acct.ID)
	if err != nil {
		return true, err
	}
	mode := ModeTLS
	if cfg.smtpMode == "starttls" {
		mode = ModeSMTPStartTLS
	}
	conn, err := b.Dial(ctx, cfg.smtpHost, cfg.smtpPort, mode)
	if err != nil {
		return true, mapNetErr("connecting to the SMTP server", err)
	}
	c := smtp.NewClient(conn)
	defer c.Close()
	if err := c.Hello("acton.invalid"); err != nil {
		return true, mapNetErr("greeting the SMTP server", err)
	}
	if err := c.Auth(sasl.NewPlainClient("", login.Username, login.Password)); err != nil {
		return true, abi.Errorf(abi.ErrAuthExpired, "the SMTP server rejected the login")
	}
	if err := c.Mail(cfg.email, nil); err != nil {
		return true, smtpErr("the sender was refused", err)
	}
	for _, r := range rcpt {
		if err := c.Rcpt(r, nil); err != nil {
			return true, smtpErr("a recipient was refused", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return true, smtpErr("the server did not accept the message", err)
	}
	if _, err := w.Write(raw); err != nil {
		return false, mapNetErr("sending the message", err)
	}
	if err := w.Close(); err != nil {
		var se *smtp.SMTPError
		if errors.As(err, &se) && se.Code >= 500 {
			return true, abi.Errorf(abi.ErrInvalid, "the server rejected the message: %s", se.Message)
		}
		return false, mapNetErr("finishing the message", err)
	}
	_ = c.Quit()
	return false, nil
}

func smtpErr(what string, err error) error {
	var se *smtp.SMTPError
	if errors.As(err, &se) {
		if se.Code >= 500 {
			return abi.Errorf(abi.ErrInvalid, "%s: %s", what, se.Message)
		}
		return abi.Errorf(abi.ErrTransient, "%s: %s", what, se.Message)
	}
	return mapNetErr(what, err)
}

var _ = fmt.Sprintf

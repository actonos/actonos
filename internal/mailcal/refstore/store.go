// Package refstore is the reference mail and calendar backend: a deterministic,
// network-free implementation of every ABI operation. It is shared by the native
// in-process provider (fast unit tests, evals) and by the reference WASM guest, so the
// conformance suite can prove both behave identically across the WASM boundary.
//
// It must stay dependency-light: it is compiled into a wasip1 module.
package refstore

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

const (
	defaultLimit = 25
	maxLimit     = 100
	maxRecipient = 50
	maxBody      = 256 * 1024
	trashFolder  = "Trash"
	sentFolder   = "Sent"
	draftFolder  = "Drafts"
)

// Seed is the initial content every account starts with.
type Seed struct {
	SelfEmail string        `json:"self_email"`
	Messages  []abi.Message `json:"messages"`
	Events    []abi.Event   `json:"events"`
}

type accountState struct {
	messages  map[string]abi.Message
	events    map[string]abi.Event
	sendKeys  map[string]abi.MailSendResult
	createKey map[string]abi.Event
	seq       int
}

// Store holds one independent mailbox and calendar per account id.
type Store struct {
	mu       sync.Mutex
	seed     Seed
	accounts map[string]*accountState
}

// New builds a store whose accounts all start from seed.
func New(seed Seed) *Store {
	return &Store{seed: seed, accounts: map[string]*accountState{}}
}

func (s *Store) account(id string) *accountState {
	if st, ok := s.accounts[id]; ok {
		return st
	}
	st := &accountState{
		messages:  map[string]abi.Message{},
		events:    map[string]abi.Event{},
		sendKeys:  map[string]abi.MailSendResult{},
		createKey: map[string]abi.Event{},
	}
	for _, m := range s.seed.Messages {
		st.messages[m.ID] = m
	}
	for _, e := range s.seed.Events {
		st.events[e.ID] = e
	}
	s.accounts[id] = st
	return st
}

// Handle executes one ABI export and returns the encoded abi.Response. It never fails:
// every problem, including undecodable input, becomes a coded error in the response.
func (s *Store) Handle(op string, reqJSON []byte) []byte {
	var req abi.Request
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		return encodeErr(abi.Errorf(abi.ErrInvalid, "decoding request: %v", err))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.account(req.Account.ID)

	result, perr := s.dispatch(st, op, req.Params)
	if perr != nil {
		return encodeErr(perr)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return encodeErr(abi.Errorf(abi.ErrInvalid, "encoding result: %v", err))
	}
	out, _ := json.Marshal(abi.Response{Result: raw})
	return out
}

func encodeErr(e *abi.Error) []byte {
	out, _ := json.Marshal(abi.Response{Error: e})
	return out
}

func decode(params json.RawMessage, into any) *abi.Error {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(params, into); err != nil {
		return abi.Errorf(abi.ErrInvalid, "decoding params: %v", err)
	}
	return nil
}

func (s *Store) dispatch(st *accountState, op string, params json.RawMessage) (any, *abi.Error) {
	switch op {
	case abi.ExportCapabilities:
		return abi.Capabilities{
			ABI:    []string{abi.CapMail, abi.CapCalendar},
			Ops:    abi.AllExports,
			Limits: abi.Limits{MaxRecipients: maxRecipient, MaxBodyBytes: maxBody},
			Notes:  "reference in-memory provider",
		}, nil
	case abi.ExportMailList:
		var p abi.MailListParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.mailList(st, p)
	case abi.ExportMailSearch:
		var p abi.MailSearchParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.mailSearch(st, p)
	case abi.ExportMailGet:
		var p abi.MailGetParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		m, ok := st.messages[p.ID]
		if !ok {
			return nil, abi.Errorf(abi.ErrNotFound, "message %q", p.ID)
		}
		return m, nil
	case abi.ExportMailGetThread:
		var p abi.MailGetThreadParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.thread(st, p.ThreadID)
	case abi.ExportMailCreateDraft:
		var p abi.MailDraftParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		st.seq++
		id := fmt.Sprintf("draft-%d", st.seq)
		st.messages[id] = abi.Message{ID: id, ThreadID: id, Folder: draftFolder, To: p.To, Cc: p.Cc,
			Subject: p.Subject, BodyText: p.BodyText, InReplyTo: p.InReplyTo, Date: time.Now().UTC()}
		return abi.MailDraftResult{ProviderDraftID: id}, nil
	case abi.ExportMailSend:
		var p abi.MailSendParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.mailSend(st, p)
	case abi.ExportMailMark:
		var p abi.MailMarkParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		for _, id := range p.IDs {
			if _, ok := st.messages[id]; !ok {
				return nil, abi.Errorf(abi.ErrNotFound, "message %q", id)
			}
		}
		for _, id := range p.IDs {
			m := st.messages[id]
			m.Unread = p.Unread
			st.messages[id] = m
		}
		return abi.Empty{}, nil
	case abi.ExportMailMove:
		var p abi.MailMoveParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		if strings.TrimSpace(p.Folder) == "" {
			return nil, abi.Errorf(abi.ErrInvalid, "folder is required")
		}
		for _, id := range p.IDs {
			if _, ok := st.messages[id]; !ok {
				return nil, abi.Errorf(abi.ErrNotFound, "message %q", id)
			}
		}
		for _, id := range p.IDs {
			m := st.messages[id]
			m.Folder = p.Folder
			st.messages[id] = m
		}
		return abi.Empty{}, nil
	case abi.ExportMailDelete:
		var p abi.MailDeleteParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		for _, id := range p.IDs {
			if _, ok := st.messages[id]; !ok {
				return nil, abi.Errorf(abi.ErrNotFound, "message %q", id)
			}
		}
		for _, id := range p.IDs {
			m := st.messages[id]
			if m.Folder == trashFolder {
				delete(st.messages, id)
				continue
			}
			m.Folder = trashFolder
			st.messages[id] = m
		}
		return abi.Empty{}, nil
	case abi.ExportCalListEvents:
		var p abi.CalListParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.calList(st, p)
	case abi.ExportCalGetEvent:
		var p abi.CalGetParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		ev, ok := st.events[p.ID]
		if !ok {
			return nil, abi.Errorf(abi.ErrNotFound, "event %q", p.ID)
		}
		return ev, nil
	case abi.ExportCalCreateEvent:
		var p abi.CalWriteParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.calCreate(st, p)
	case abi.ExportCalUpdateEvent:
		var p abi.CalWriteParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		if _, ok := st.events[p.Event.ID]; !ok {
			return nil, abi.Errorf(abi.ErrNotFound, "event %q", p.Event.ID)
		}
		if e := validEvent(p.Event); e != nil {
			return nil, e
		}
		st.events[p.Event.ID] = p.Event
		return p.Event, nil
	case abi.ExportCalRespond:
		var p abi.CalRespondParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.calRespond(st, p)
	case abi.ExportCalDeleteEvent:
		var p abi.CalDeleteParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		if _, ok := st.events[p.ID]; !ok {
			return nil, abi.Errorf(abi.ErrNotFound, "event %q", p.ID)
		}
		delete(st.events, p.ID)
		return abi.Empty{}, nil
	case abi.ExportCalFreeBusy:
		var p abi.FreeBusyParams
		if e := decode(params, &p); e != nil {
			return nil, e
		}
		return s.freeBusy(st, p)
	default:
		return nil, abi.Errorf(abi.ErrUnsupported, "operation %q", op)
	}
}

func page(limit int) int {
	switch {
	case limit <= 0:
		return defaultLimit
	case limit > maxLimit:
		return maxLimit
	default:
		return limit
	}
}

func offset(cursor string) (int, *abi.Error) {
	if cursor == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0, abi.Errorf(abi.ErrInvalid, "bad cursor %q", cursor)
	}
	return n, nil
}

func summary(m abi.Message) abi.Message {
	if m.Snippet == "" && m.BodyText != "" {
		m.Snippet = truncate(m.BodyText, 120)
	}
	m.BodyText = ""
	return m
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func paginate(all []abi.Message, cursor string, limit int) (abi.MailListResult, *abi.Error) {
	sort.Slice(all, func(i, j int) bool {
		if all[i].Date.Equal(all[j].Date) {
			return all[i].ID < all[j].ID
		}
		return all[i].Date.After(all[j].Date)
	})
	start, perr := offset(cursor)
	if perr != nil {
		return abi.MailListResult{}, perr
	}
	if start > len(all) {
		start = len(all)
	}
	end := start + page(limit)
	next := ""
	if end < len(all) {
		next = strconv.Itoa(end)
	} else {
		end = len(all)
	}
	out := make([]abi.Message, 0, end-start)
	for _, m := range all[start:end] {
		out = append(out, summary(m))
	}
	return abi.MailListResult{Messages: out, NextCursor: next}, nil
}

func (s *Store) mailList(st *accountState, p abi.MailListParams) (any, *abi.Error) {
	folder := p.Folder
	if folder == "" {
		folder = "INBOX"
	}
	var all []abi.Message
	for _, m := range st.messages {
		if m.Folder == folder && (!p.UnreadOnly || m.Unread) {
			all = append(all, m)
		}
	}
	return paginate(all, p.Cursor, p.Limit)
}

func (s *Store) mailSearch(st *accountState, p abi.MailSearchParams) (any, *abi.Error) {
	q := strings.ToLower(strings.TrimSpace(p.Query))
	if q == "" {
		return nil, abi.Errorf(abi.ErrInvalid, "query is required")
	}
	var all []abi.Message
	for _, m := range st.messages {
		if p.Folder != "" && m.Folder != p.Folder {
			continue
		}
		if p.Folder == "" && (m.Folder == trashFolder || m.Folder == draftFolder) {
			continue
		}
		hay := strings.ToLower(m.Subject + " " + m.BodyText + " " + m.From.Email + " " + m.Snippet)
		if strings.Contains(hay, q) {
			all = append(all, m)
		}
	}
	return paginate(all, p.Cursor, p.Limit)
}

func (s *Store) thread(st *accountState, id string) (any, *abi.Error) {
	var msgs []abi.Message
	for _, m := range st.messages {
		if m.ThreadID == id && m.Folder != trashFolder && m.Folder != draftFolder {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return nil, abi.Errorf(abi.ErrNotFound, "thread %q", id)
	}
	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].Date.Equal(msgs[j].Date) {
			return msgs[i].ID < msgs[j].ID
		}
		return msgs[i].Date.Before(msgs[j].Date)
	})
	return abi.Thread{ID: id, Messages: msgs}, nil
}

func (s *Store) mailSend(st *accountState, p abi.MailSendParams) (any, *abi.Error) {
	if strings.TrimSpace(p.IdempotencyKey) == "" {
		return nil, abi.Errorf(abi.ErrInvalid, "idempotency_key is required")
	}
	if prev, ok := st.sendKeys[p.IdempotencyKey]; ok {
		prev.Replayed = true
		return prev, nil
	}
	if len(p.To) == 0 {
		return nil, abi.Errorf(abi.ErrInvalid, "at least one recipient is required")
	}
	if len(p.To)+len(p.Cc)+len(p.Bcc) > maxRecipient {
		return nil, abi.Errorf(abi.ErrInvalid, "more than %d recipients", maxRecipient)
	}
	if len(p.BodyText) > maxBody {
		return nil, abi.Errorf(abi.ErrInvalid, "body exceeds %d bytes", maxBody)
	}
	st.seq++
	id := fmt.Sprintf("sent-%d", st.seq)
	thread := p.ThreadID
	if thread == "" {
		thread = id
	}
	self := s.seed.SelfEmail
	st.messages[id] = abi.Message{
		ID: id, ThreadID: thread, Folder: sentFolder, From: abi.Address{Email: self},
		To: p.To, Cc: p.Cc, Subject: p.Subject, BodyText: p.BodyText, InReplyTo: p.InReplyTo,
		Date: time.Now().UTC(),
	}
	res := abi.MailSendResult{ProviderMessageID: id, ThreadID: thread}
	st.sendKeys[p.IdempotencyKey] = res
	return res, nil
}

func validEvent(e abi.Event) *abi.Error {
	if strings.TrimSpace(e.Title) == "" {
		return abi.Errorf(abi.ErrInvalid, "title is required")
	}
	if !e.End.After(e.Start) {
		return abi.Errorf(abi.ErrInvalid, "end must be after start")
	}
	return nil
}

func (s *Store) calList(st *accountState, p abi.CalListParams) (any, *abi.Error) {
	if !p.To.After(p.From) {
		return nil, abi.Errorf(abi.ErrInvalid, "'to' must be after 'from'")
	}
	var evs []abi.Event
	for _, e := range st.events {
		if e.Start.Before(p.To) && e.End.After(p.From) {
			evs = append(evs, e)
		}
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].Start.Equal(evs[j].Start) {
			return evs[i].ID < evs[j].ID
		}
		return evs[i].Start.Before(evs[j].Start)
	})
	start, perr := offset(p.Cursor)
	if perr != nil {
		return nil, perr
	}
	if start > len(evs) {
		start = len(evs)
	}
	end := start + page(p.Limit)
	next := ""
	if end < len(evs) {
		next = strconv.Itoa(end)
	} else {
		end = len(evs)
	}
	return abi.CalListResult{Events: evs[start:end], NextCursor: next}, nil
}

func (s *Store) calCreate(st *accountState, p abi.CalWriteParams) (any, *abi.Error) {
	if p.IdempotencyKey != "" {
		if prev, ok := st.createKey[p.IdempotencyKey]; ok {
			return prev, nil
		}
	}
	if e := validEvent(p.Event); e != nil {
		return nil, e
	}
	st.seq++
	ev := p.Event
	ev.ID = fmt.Sprintf("ev-%d", st.seq)
	st.events[ev.ID] = ev
	if p.IdempotencyKey != "" {
		st.createKey[p.IdempotencyKey] = ev
	}
	return ev, nil
}

func (s *Store) calRespond(st *accountState, p abi.CalRespondParams) (any, *abi.Error) {
	switch p.Response {
	case "accepted", "declined", "tentative":
	default:
		return nil, abi.Errorf(abi.ErrInvalid, "response must be accepted, declined or tentative")
	}
	ev, ok := st.events[p.ID]
	if !ok {
		return nil, abi.Errorf(abi.ErrNotFound, "event %q", p.ID)
	}
	found := false
	attendees := append([]abi.Attendee(nil), ev.Attendees...)
	for i := range attendees {
		if attendees[i].Email == s.seed.SelfEmail {
			attendees[i].Status = p.Response
			found = true
		}
	}
	if !found {
		return nil, abi.Errorf(abi.ErrInvalid, "the account is not an attendee of event %q", p.ID)
	}
	ev.Attendees = attendees
	st.events[p.ID] = ev
	return ev, nil
}

func (s *Store) freeBusy(st *accountState, p abi.FreeBusyParams) (any, *abi.Error) {
	if !p.To.After(p.From) {
		return nil, abi.Errorf(abi.ErrInvalid, "'to' must be after 'from'")
	}
	out := abi.FreeBusyResult{Busy: map[string][]abi.BusyInterval{}}
	for _, raw := range p.Emails {
		email, err := abi.NormalizeEmail(raw)
		if err != nil {
			return nil, abi.Errorf(abi.ErrInvalid, "bad email %q", raw)
		}
		spans := []abi.BusyInterval{}
		for _, e := range st.events {
			if !(e.Start.Before(p.To) && e.End.After(p.From)) {
				continue
			}
			for _, a := range e.Attendees {
				if a.Email == email && a.Status != "declined" {
					spans = append(spans, abi.BusyInterval{Start: e.Start, End: e.End})
					break
				}
			}
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].Start.Before(spans[j].Start) })
		out.Busy[email] = spans
	}
	return out, nil
}

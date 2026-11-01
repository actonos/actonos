// Package abi defines the wire contract between the ActonOS core and mail/calendar
// provider plugins. It is imported by the core and by WASM guests, so it must stay
// dependency-light (standard library and x/net/idna only).
//
// Compatibility rule: after v1 ships this package is additive-only. Never rename,
// remove or change the meaning of a field, export name or error code; add new
// optional fields and new exports instead. TestGoldenV1 enforces this.
package abi

import (
	"encoding/json"
	"fmt"
	"time"
)

// Capability strings declared in a plugin manifest's "capabilities" list.
const (
	CapMail     = "mail@1"
	CapCalendar = "calendar@1"
)

// Export names. Every export has the signature (reqPtr, reqLen uint32) -> uint64,
// where the result packs (resPtr<<32 | resLen). Both buffers hold JSON.
const (
	ExportCapabilities    = "acton_mailcal_capabilities"
	ExportMailList        = "acton_mail_list"
	ExportMailGet         = "acton_mail_get"
	ExportMailGetThread   = "acton_mail_get_thread"
	ExportMailSearch      = "acton_mail_search"
	ExportMailCreateDraft = "acton_mail_create_draft"
	ExportMailSend        = "acton_mail_send"
	ExportMailMark        = "acton_mail_mark"
	ExportMailMove        = "acton_mail_move"
	ExportMailDelete      = "acton_mail_delete"
	ExportCalListEvents   = "acton_calendar_list_events"
	ExportCalGetEvent     = "acton_calendar_get_event"
	ExportCalCreateEvent  = "acton_calendar_create_event"
	ExportCalUpdateEvent  = "acton_calendar_update_event"
	ExportCalRespond      = "acton_calendar_respond"
	ExportCalDeleteEvent  = "acton_calendar_delete_event"
	ExportCalFreeBusy     = "acton_calendar_free_busy"
)

// AllExports lists every export a provider may implement.
var AllExports = []string{
	ExportCapabilities,
	ExportMailList, ExportMailGet, ExportMailGetThread, ExportMailSearch,
	ExportMailCreateDraft, ExportMailSend, ExportMailMark, ExportMailMove, ExportMailDelete,
	ExportCalListEvents, ExportCalGetEvent, ExportCalCreateEvent, ExportCalUpdateEvent,
	ExportCalRespond, ExportCalDeleteEvent, ExportCalFreeBusy,
}

// ErrorCode classifies a provider failure so the core can decide how to react.
type ErrorCode string

const (
	// ErrAuthExpired: credentials are missing, revoked or expired. Core pauses the account.
	ErrAuthExpired ErrorCode = "auth_expired"
	// ErrRateLimited: back off for RetryAfterMS then retry.
	ErrRateLimited ErrorCode = "rate_limited"
	// ErrNotFound: the referenced message, thread, event or folder does not exist.
	ErrNotFound ErrorCode = "not_found"
	// ErrUnsupported: this provider does not implement the operation.
	ErrUnsupported ErrorCode = "unsupported"
	// ErrTransient: network or server hiccup; safe to retry reads, never retry a send without its key.
	ErrTransient ErrorCode = "transient"
	// ErrInvalid: the request itself is malformed or violates a provider limit.
	ErrInvalid ErrorCode = "invalid"
)

// Error is the structured failure carried in Response.Error.
type Error struct {
	Code         ErrorCode `json:"code"`
	Message      string    `json:"message"`
	RetryAfterMS int64     `json:"retry_after_ms,omitempty"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Errorf builds an *Error.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// AccountRef identifies the account a call is for. Config holds non-secret settings
// (host names, folder names, calendar id). Secrets are never in the request: guests
// obtain tokens through host_get_token and passwords through get_secret.
type AccountRef struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config,omitempty"`
}

// Request is the envelope every export receives.
type Request struct {
	Account AccountRef      `json:"account"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is the envelope every export returns. Exactly one of Result and Error is set.
type Response struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Capabilities is returned by acton_mailcal_capabilities.
type Capabilities struct {
	ABI    []string `json:"abi"`    // e.g. ["mail@1","calendar@1"]
	Ops    []string `json:"ops"`    // export names the provider implements
	Limits Limits   `json:"limits"` // provider limits the core enforces up front
	Notes  string   `json:"notes,omitempty"`
}

// Limits are provider-side ceilings. Zero means "no stated limit".
type Limits struct {
	MaxRecipients      int   `json:"max_recipients,omitempty"`
	MaxBodyBytes       int   `json:"max_body_bytes,omitempty"`
	MaxAttachmentBytes int64 `json:"max_attachment_bytes,omitempty"` // 0 and no attachment support in v1
}

// Supports reports whether the provider declared an export.
func (c Capabilities) Supports(export string) bool {
	for _, op := range c.Ops {
		if op == export {
			return true
		}
	}
	return false
}

// Address is a normalized mailbox. Email is lower-case with an ASCII (punycode) domain.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// AttachmentMeta describes an attachment. v1 never transfers attachment bytes.
type AttachmentMeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MIME string `json:"mime,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// Message is one email. Body is plain text only; providers convert HTML.
type Message struct {
	ID          string           `json:"id"`
	ThreadID    string           `json:"thread_id"`
	Folder      string           `json:"folder,omitempty"`
	From        Address          `json:"from"`
	To          []Address        `json:"to,omitempty"`
	Cc          []Address        `json:"cc,omitempty"`
	Subject     string           `json:"subject"`
	Date        time.Time        `json:"date"`
	Snippet     string           `json:"snippet,omitempty"`
	BodyText    string           `json:"body_text,omitempty"` // empty in list results
	Labels      []string         `json:"labels,omitempty"`
	Unread      bool             `json:"unread,omitempty"`
	InReplyTo   string           `json:"in_reply_to,omitempty"` // provider message id of the parent
	Attachments []AttachmentMeta `json:"attachments,omitempty"`
}

// Thread is a conversation, oldest message first.
type Thread struct {
	ID       string    `json:"id"`
	Messages []Message `json:"messages"`
}

// MailListParams selects messages. Query is provider syntax-free: the core only passes
// plain words, and providers translate them into their own search.
type MailListParams struct {
	Folder     string `json:"folder,omitempty"` // default "INBOX"
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"` // default 25, max 100
	UnreadOnly bool   `json:"unread_only,omitempty"`
}

// MailSearchParams is a free-text search.
type MailSearchParams struct {
	Query  string `json:"query"`
	Folder string `json:"folder,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// MailListResult is a page of messages without bodies.
type MailListResult struct {
	Messages   []Message `json:"messages"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// MailGetParams fetches one message with its body.
type MailGetParams struct {
	ID string `json:"id"`
}

// MailGetThreadParams fetches a whole conversation.
type MailGetThreadParams struct {
	ThreadID string `json:"thread_id"`
}

// MailSendParams is the final, already-approved outgoing message. The core builds it
// from an immutable draft; a provider must send exactly this and nothing else.
type MailSendParams struct {
	To             []Address `json:"to"`
	Cc             []Address `json:"cc,omitempty"`
	Bcc            []Address `json:"bcc,omitempty"`
	Subject        string    `json:"subject"`
	BodyText       string    `json:"body_text"`
	InReplyTo      string    `json:"in_reply_to,omitempty"` // provider id of the message being answered
	ThreadID       string    `json:"thread_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"` // a replay with the same key must not send twice
}

// MailSendResult is returned after sending.
type MailSendResult struct {
	ProviderMessageID string `json:"provider_message_id"`
	ThreadID          string `json:"thread_id,omitempty"`
	Replayed          bool   `json:"replayed,omitempty"` // true when the key was already used
}

// MailDraftParams asks the provider to keep a copy as a draft in its own UI. Optional.
type MailDraftParams struct {
	To        []Address `json:"to"`
	Cc        []Address `json:"cc,omitempty"`
	Subject   string    `json:"subject"`
	BodyText  string    `json:"body_text"`
	InReplyTo string    `json:"in_reply_to,omitempty"`
}

// MailDraftResult identifies the provider-side draft.
type MailDraftResult struct {
	ProviderDraftID string `json:"provider_draft_id"`
}

// MailMarkParams changes the read state of messages.
type MailMarkParams struct {
	IDs    []string `json:"ids"`
	Unread bool     `json:"unread"`
}

// MailMoveParams moves messages to a folder or label.
type MailMoveParams struct {
	IDs    []string `json:"ids"`
	Folder string   `json:"folder"`
}

// MailDeleteParams deletes (or trashes) messages.
type MailDeleteParams struct {
	IDs []string `json:"ids"`
}

// Attendee is one invitee.
type Attendee struct {
	Email  string `json:"email"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"` // needs_action|accepted|declined|tentative
}

// Event is one calendar entry. Times are RFC 3339; AllDay events use midnight start/end.
type Event struct {
	ID            string     `json:"id"`
	CalendarID    string     `json:"calendar_id,omitempty"`
	Title         string     `json:"title"`
	Start         time.Time  `json:"start"`
	End           time.Time  `json:"end"`
	TimeZone      string     `json:"time_zone,omitempty"`
	AllDay        bool       `json:"all_day,omitempty"`
	Location      string     `json:"location,omitempty"`
	Description   string     `json:"description,omitempty"`
	ConferenceURL string     `json:"conference_url,omitempty"`
	Attendees     []Attendee `json:"attendees,omitempty"`
	Organizer     *Address   `json:"organizer,omitempty"`
}

// CalListParams selects events in [From, To).
type CalListParams struct {
	CalendarID string    `json:"calendar_id,omitempty"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Cursor     string    `json:"cursor,omitempty"`
	Limit      int       `json:"limit,omitempty"`
}

// CalListResult is a page of events. Recurring events arrive already expanded.
type CalListResult struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

// CalGetParams fetches one event.
type CalGetParams struct {
	CalendarID string `json:"calendar_id,omitempty"`
	ID         string `json:"id"`
}

// CalWriteParams creates or replaces an event. For create, Event.ID is ignored.
// IdempotencyKey makes a retried create safe.
type CalWriteParams struct {
	Event          Event  `json:"event"`
	SendUpdates    bool   `json:"send_updates,omitempty"` // notify attendees
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// CalRespondParams answers an invitation: accepted, declined or tentative.
type CalRespondParams struct {
	CalendarID string `json:"calendar_id,omitempty"`
	ID         string `json:"id"`
	Response   string `json:"response"`
	Comment    string `json:"comment,omitempty"`
}

// CalDeleteParams removes an event.
type CalDeleteParams struct {
	CalendarID  string `json:"calendar_id,omitempty"`
	ID          string `json:"id"`
	SendUpdates bool   `json:"send_updates,omitempty"`
}

// FreeBusyParams asks when the given people are busy within [From, To).
type FreeBusyParams struct {
	Emails []string  `json:"emails"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

// BusyInterval is one busy span.
type BusyInterval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// FreeBusyResult maps a normalized email to its busy spans.
type FreeBusyResult struct {
	Busy map[string][]BusyInterval `json:"busy"`
}

// Empty is the result of operations that return nothing.
type Empty struct{}

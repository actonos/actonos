// Package provkit is the toolkit every mail/calendar provider plugin is built from. It depends
// only on package abi, so a plugin compiled to WebAssembly does not drag in the host.
//
// A plugin author implements Backend (embedding Base for the operations the provider does not
// support), and provkit turns it into the JSON-in/JSON-out exports of the ABI.
package provkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

// Backend is the typed view of the ABI. Its method set is identical to mailcal.Provider, so
// anything that implements one implements the other.
type Backend interface {
	Capabilities(ctx context.Context, acct abi.AccountRef) (abi.Capabilities, error)

	MailList(ctx context.Context, acct abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error)
	MailSearch(ctx context.Context, acct abi.AccountRef, p abi.MailSearchParams) (abi.MailListResult, error)
	MailGet(ctx context.Context, acct abi.AccountRef, id string) (abi.Message, error)
	MailGetThread(ctx context.Context, acct abi.AccountRef, threadID string) (abi.Thread, error)
	MailCreateDraft(ctx context.Context, acct abi.AccountRef, p abi.MailDraftParams) (abi.MailDraftResult, error)
	MailSend(ctx context.Context, acct abi.AccountRef, p abi.MailSendParams) (abi.MailSendResult, error)
	MailMark(ctx context.Context, acct abi.AccountRef, p abi.MailMarkParams) error
	MailMove(ctx context.Context, acct abi.AccountRef, p abi.MailMoveParams) error
	MailDelete(ctx context.Context, acct abi.AccountRef, p abi.MailDeleteParams) error

	CalendarList(ctx context.Context, acct abi.AccountRef, p abi.CalListParams) (abi.CalListResult, error)
	CalendarGet(ctx context.Context, acct abi.AccountRef, p abi.CalGetParams) (abi.Event, error)
	CalendarCreate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error)
	CalendarUpdate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error)
	CalendarRespond(ctx context.Context, acct abi.AccountRef, p abi.CalRespondParams) (abi.Event, error)
	CalendarDelete(ctx context.Context, acct abi.AccountRef, p abi.CalDeleteParams) error
	CalendarFreeBusy(ctx context.Context, acct abi.AccountRef, p abi.FreeBusyParams) (abi.FreeBusyResult, error)
}

// Base answers every operation with an "unsupported" error. Embed it and override what the
// provider can do; list the overridden exports in Capabilities().Ops.
type Base struct{}

func unsupported(what string) error {
	return abi.Errorf(abi.ErrUnsupported, "this provider does not support %s", what)
}

func (Base) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{}, unsupported("capabilities")
}
func (Base) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	return abi.MailListResult{}, unsupported("mail list")
}
func (Base) MailSearch(context.Context, abi.AccountRef, abi.MailSearchParams) (abi.MailListResult, error) {
	return abi.MailListResult{}, unsupported("mail search")
}
func (Base) MailGet(context.Context, abi.AccountRef, string) (abi.Message, error) {
	return abi.Message{}, unsupported("mail get")
}
func (Base) MailGetThread(context.Context, abi.AccountRef, string) (abi.Thread, error) {
	return abi.Thread{}, unsupported("mail threads")
}
func (Base) MailCreateDraft(context.Context, abi.AccountRef, abi.MailDraftParams) (abi.MailDraftResult, error) {
	return abi.MailDraftResult{}, unsupported("provider-side drafts")
}
func (Base) MailSend(context.Context, abi.AccountRef, abi.MailSendParams) (abi.MailSendResult, error) {
	return abi.MailSendResult{}, unsupported("sending mail")
}
func (Base) MailMark(context.Context, abi.AccountRef, abi.MailMarkParams) error {
	return unsupported("marking mail")
}
func (Base) MailMove(context.Context, abi.AccountRef, abi.MailMoveParams) error {
	return unsupported("moving mail")
}
func (Base) MailDelete(context.Context, abi.AccountRef, abi.MailDeleteParams) error {
	return unsupported("deleting mail")
}
func (Base) CalendarList(context.Context, abi.AccountRef, abi.CalListParams) (abi.CalListResult, error) {
	return abi.CalListResult{}, unsupported("listing events")
}
func (Base) CalendarGet(context.Context, abi.AccountRef, abi.CalGetParams) (abi.Event, error) {
	return abi.Event{}, unsupported("reading events")
}
func (Base) CalendarCreate(context.Context, abi.AccountRef, abi.CalWriteParams) (abi.Event, error) {
	return abi.Event{}, unsupported("creating events")
}
func (Base) CalendarUpdate(context.Context, abi.AccountRef, abi.CalWriteParams) (abi.Event, error) {
	return abi.Event{}, unsupported("updating events")
}
func (Base) CalendarRespond(context.Context, abi.AccountRef, abi.CalRespondParams) (abi.Event, error) {
	return abi.Event{}, unsupported("answering invitations")
}
func (Base) CalendarDelete(context.Context, abi.AccountRef, abi.CalDeleteParams) error {
	return unsupported("deleting events")
}
func (Base) CalendarFreeBusy(context.Context, abi.AccountRef, abi.FreeBusyParams) (abi.FreeBusyResult, error) {
	return abi.FreeBusyResult{}, unsupported("free/busy")
}

// Dispatch executes one ABI export against b and returns the encoded abi.Response. It never
// fails and never panics: every problem, including a panic inside b, is a coded error.
func Dispatch(ctx context.Context, b Backend, op string, reqJSON []byte) (out []byte) {
	defer func() {
		if r := recover(); r != nil {
			out = encode(nil, abi.Errorf(abi.ErrTransient, "provider crashed: %v", r))
		}
	}()
	var req abi.Request
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		return encode(nil, abi.Errorf(abi.ErrInvalid, "decoding request: %v", err))
	}
	result, err := run(ctx, b, op, req)
	if err != nil {
		if e, ok := err.(*abi.Error); ok {
			return encode(nil, e)
		}
		return encode(nil, abi.Errorf(abi.ErrTransient, "%v", err))
	}
	return encode(result, nil)
}

func encode(result any, e *abi.Error) []byte {
	resp := abi.Response{Error: e}
	if e == nil {
		raw, err := json.Marshal(result)
		if err != nil {
			return encode(nil, abi.Errorf(abi.ErrInvalid, "encoding result: %v", err))
		}
		resp.Result = raw
	}
	out, _ := json.Marshal(resp)
	return out
}

func decode[T any](params json.RawMessage) (T, error) {
	var v T
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(params, &v); err != nil {
		return v, abi.Errorf(abi.ErrInvalid, "decoding params: %v", err)
	}
	return v, nil
}

// with decodes params into P and calls fn, so each case below stays one line.
func with[P, R any](req abi.Request, fn func(P) (R, error)) (any, error) {
	p, err := decode[P](req.Params)
	if err != nil {
		return nil, err
	}
	return fn(p)
}

func none[P any](req abi.Request, fn func(P) error) (any, error) {
	p, err := decode[P](req.Params)
	if err != nil {
		return nil, err
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func run(ctx context.Context, b Backend, op string, req abi.Request) (any, error) {
	a := req.Account
	switch op {
	case abi.ExportCapabilities:
		return b.Capabilities(ctx, a)
	case abi.ExportMailList:
		return with(req, func(p abi.MailListParams) (abi.MailListResult, error) { return b.MailList(ctx, a, p) })
	case abi.ExportMailSearch:
		return with(req, func(p abi.MailSearchParams) (abi.MailListResult, error) { return b.MailSearch(ctx, a, p) })
	case abi.ExportMailGet:
		return with(req, func(p abi.MailGetParams) (abi.Message, error) { return b.MailGet(ctx, a, p.ID) })
	case abi.ExportMailGetThread:
		return with(req, func(p abi.MailGetThreadParams) (abi.Thread, error) { return b.MailGetThread(ctx, a, p.ThreadID) })
	case abi.ExportMailCreateDraft:
		return with(req, func(p abi.MailDraftParams) (abi.MailDraftResult, error) { return b.MailCreateDraft(ctx, a, p) })
	case abi.ExportMailSend:
		return with(req, func(p abi.MailSendParams) (abi.MailSendResult, error) { return b.MailSend(ctx, a, p) })
	case abi.ExportMailMark:
		return none(req, func(p abi.MailMarkParams) error { return b.MailMark(ctx, a, p) })
	case abi.ExportMailMove:
		return none(req, func(p abi.MailMoveParams) error { return b.MailMove(ctx, a, p) })
	case abi.ExportMailDelete:
		return none(req, func(p abi.MailDeleteParams) error { return b.MailDelete(ctx, a, p) })
	case abi.ExportCalListEvents:
		return with(req, func(p abi.CalListParams) (abi.CalListResult, error) { return b.CalendarList(ctx, a, p) })
	case abi.ExportCalGetEvent:
		return with(req, func(p abi.CalGetParams) (abi.Event, error) { return b.CalendarGet(ctx, a, p) })
	case abi.ExportCalCreateEvent:
		return with(req, func(p abi.CalWriteParams) (abi.Event, error) { return b.CalendarCreate(ctx, a, p) })
	case abi.ExportCalUpdateEvent:
		return with(req, func(p abi.CalWriteParams) (abi.Event, error) { return b.CalendarUpdate(ctx, a, p) })
	case abi.ExportCalRespond:
		return with(req, func(p abi.CalRespondParams) (abi.Event, error) { return b.CalendarRespond(ctx, a, p) })
	case abi.ExportCalDeleteEvent:
		return none(req, func(p abi.CalDeleteParams) error { return b.CalendarDelete(ctx, a, p) })
	case abi.ExportCalFreeBusy:
		return with(req, func(p abi.FreeBusyParams) (abi.FreeBusyResult, error) { return b.CalendarFreeBusy(ctx, a, p) })
	}
	return nil, abi.Errorf(abi.ErrUnsupported, "unknown export %q", op)
}

// Describe is a helper for error messages.
func Describe(acct abi.AccountRef) string { return fmt.Sprintf("account %q", acct.ID) }

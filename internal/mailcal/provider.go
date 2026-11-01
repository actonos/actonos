// Package mailcal is the core of ActonOS email and calendar support: a provider-agnostic
// model, an account registry, immutable drafts, and the tools agents use. Provider code
// (Gmail, Graph, CalDAV, IMAP/SMTP) lives in plugins, never here.
package mailcal

import (
	"context"
	"errors"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

// Provider is the Go view of the mail/calendar ABI. Every method returns either a
// result or an *abi.Error (use AsError to inspect). Implementations: the WASM bridge
// (BridgeProvider) and, in tests, the reference store behind the same bridge.
type Provider interface {
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

// AsError extracts the coded provider error from err, or returns nil when err is not one.
func AsError(err error) *abi.Error {
	var e *abi.Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// IsCode reports whether err is a provider error with the given code.
func IsCode(err error, code abi.ErrorCode) bool {
	e := AsError(err)
	return e != nil && e.Code == code
}

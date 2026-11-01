package mailcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/plugin"
)

// Caller invokes one ABI export with a JSON request and returns the JSON response.
// *plugin.PluginInstance implements it; tests use StoreCaller.
type Caller interface {
	CallJSON(ctx context.Context, export string, req []byte) ([]byte, error)
}

// BridgeProvider implements Provider on top of a Caller.
type BridgeProvider struct {
	caller Caller
}

// NewBridgeProvider wraps caller.
func NewBridgeProvider(caller Caller) *BridgeProvider { return &BridgeProvider{caller: caller} }

var _ Provider = (*BridgeProvider)(nil)

// invoke runs one export and decodes its result into out (which may be nil).
func (b *BridgeProvider) invoke(ctx context.Context, export string, acct abi.AccountRef, params any, out any) error {
	var rawParams json.RawMessage
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return abi.Errorf(abi.ErrInvalid, "encoding params: %v", err)
		}
		rawParams = raw
	}
	reqBytes, err := json.Marshal(abi.Request{Account: acct, Params: rawParams})
	if err != nil {
		return abi.Errorf(abi.ErrInvalid, "encoding request: %v", err)
	}
	respBytes, err := b.caller.CallJSON(ctx, export, reqBytes)
	if err != nil {
		if errors.Is(err, plugin.ErrFunctionNotFound) {
			return abi.Errorf(abi.ErrUnsupported, "provider does not implement %s", export)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return abi.Errorf(abi.ErrTransient, "calling %s: %v", export, err)
	}
	var resp abi.Response
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return abi.Errorf(abi.ErrInvalid, "provider returned malformed JSON from %s: %v", export, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out == nil {
		return nil
	}
	if len(resp.Result) == 0 {
		return abi.Errorf(abi.ErrInvalid, "provider returned neither result nor error from %s", export)
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return abi.Errorf(abi.ErrInvalid, "decoding %s result: %v", export, err)
	}
	return nil
}

// call is a typed helper around invoke.
func call[T any](ctx context.Context, b *BridgeProvider, export string, acct abi.AccountRef, params any) (T, error) {
	var out T
	err := b.invoke(ctx, export, acct, params, &out)
	return out, err
}

func (b *BridgeProvider) Capabilities(ctx context.Context, acct abi.AccountRef) (abi.Capabilities, error) {
	return call[abi.Capabilities](ctx, b, abi.ExportCapabilities, acct, nil)
}

func (b *BridgeProvider) MailList(ctx context.Context, acct abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error) {
	return call[abi.MailListResult](ctx, b, abi.ExportMailList, acct, p)
}

func (b *BridgeProvider) MailSearch(ctx context.Context, acct abi.AccountRef, p abi.MailSearchParams) (abi.MailListResult, error) {
	return call[abi.MailListResult](ctx, b, abi.ExportMailSearch, acct, p)
}

func (b *BridgeProvider) MailGet(ctx context.Context, acct abi.AccountRef, id string) (abi.Message, error) {
	return call[abi.Message](ctx, b, abi.ExportMailGet, acct, abi.MailGetParams{ID: id})
}

func (b *BridgeProvider) MailGetThread(ctx context.Context, acct abi.AccountRef, threadID string) (abi.Thread, error) {
	return call[abi.Thread](ctx, b, abi.ExportMailGetThread, acct, abi.MailGetThreadParams{ThreadID: threadID})
}

func (b *BridgeProvider) MailCreateDraft(ctx context.Context, acct abi.AccountRef, p abi.MailDraftParams) (abi.MailDraftResult, error) {
	return call[abi.MailDraftResult](ctx, b, abi.ExportMailCreateDraft, acct, p)
}

func (b *BridgeProvider) MailSend(ctx context.Context, acct abi.AccountRef, p abi.MailSendParams) (abi.MailSendResult, error) {
	return call[abi.MailSendResult](ctx, b, abi.ExportMailSend, acct, p)
}

func (b *BridgeProvider) MailMark(ctx context.Context, acct abi.AccountRef, p abi.MailMarkParams) error {
	return b.invoke(ctx, abi.ExportMailMark, acct, p, nil)
}

func (b *BridgeProvider) MailMove(ctx context.Context, acct abi.AccountRef, p abi.MailMoveParams) error {
	return b.invoke(ctx, abi.ExportMailMove, acct, p, nil)
}

func (b *BridgeProvider) MailDelete(ctx context.Context, acct abi.AccountRef, p abi.MailDeleteParams) error {
	return b.invoke(ctx, abi.ExportMailDelete, acct, p, nil)
}

func (b *BridgeProvider) CalendarList(ctx context.Context, acct abi.AccountRef, p abi.CalListParams) (abi.CalListResult, error) {
	return call[abi.CalListResult](ctx, b, abi.ExportCalListEvents, acct, p)
}

func (b *BridgeProvider) CalendarGet(ctx context.Context, acct abi.AccountRef, p abi.CalGetParams) (abi.Event, error) {
	return call[abi.Event](ctx, b, abi.ExportCalGetEvent, acct, p)
}

func (b *BridgeProvider) CalendarCreate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	return call[abi.Event](ctx, b, abi.ExportCalCreateEvent, acct, p)
}

func (b *BridgeProvider) CalendarUpdate(ctx context.Context, acct abi.AccountRef, p abi.CalWriteParams) (abi.Event, error) {
	return call[abi.Event](ctx, b, abi.ExportCalUpdateEvent, acct, p)
}

func (b *BridgeProvider) CalendarRespond(ctx context.Context, acct abi.AccountRef, p abi.CalRespondParams) (abi.Event, error) {
	return call[abi.Event](ctx, b, abi.ExportCalRespond, acct, p)
}

func (b *BridgeProvider) CalendarDelete(ctx context.Context, acct abi.AccountRef, p abi.CalDeleteParams) error {
	return b.invoke(ctx, abi.ExportCalDeleteEvent, acct, p, nil)
}

func (b *BridgeProvider) CalendarFreeBusy(ctx context.Context, acct abi.AccountRef, p abi.FreeBusyParams) (abi.FreeBusyResult, error) {
	return call[abi.FreeBusyResult](ctx, b, abi.ExportCalFreeBusy, acct, p)
}

// describe is used in logs and errors.
func describe(acct abi.AccountRef) string { return fmt.Sprintf("account %q", acct.ID) }

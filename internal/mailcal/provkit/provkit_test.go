package provkit_test

import (
	"context"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/conformance"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/plugin"
)

// dispatchCaller serves a Backend through provkit.Dispatch, exactly as a plugin's exports would.
type dispatchCaller struct{ b provkit.Backend }

func (c dispatchCaller) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	for _, known := range abi.AllExports {
		if known == export {
			return provkit.Dispatch(ctx, c.b, export, req), nil
		}
	}
	return nil, plugin.ErrFunctionNotFound
}

// The reference provider, served through the kit and bridged back, must still pass the whole
// suite: that proves Dispatch and BridgeProvider are exact inverses.
func TestKitRoundTripPassesConformance(t *testing.T) {
	conformance.RunSeeded(t, func(t *testing.T) mailcal.Provider {
		var backend provkit.Backend = mailcal.NewMemProvider(refstore.DefaultSeed()) // a Provider is a Backend
		return mailcal.NewBridgeProvider(dispatchCaller{b: backend})
	})
}

type panicky struct{ provkit.Base }

func (panicky) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	panic("boom")
}

func TestBaseIsUnsupportedAndPanicsBecomeCodedErrors(t *testing.T) {
	p := mailcal.NewBridgeProvider(dispatchCaller{b: panicky{}})
	_, err := p.MailSend(context.Background(), abi.AccountRef{ID: "a"}, abi.MailSendParams{})
	if !mailcal.IsCode(err, abi.ErrUnsupported) {
		t.Fatalf("an unimplemented operation must be 'unsupported', got %v", err)
	}
	_, err = p.MailList(context.Background(), abi.AccountRef{ID: "a"}, abi.MailListParams{})
	if !mailcal.IsCode(err, abi.ErrTransient) {
		t.Fatalf("a panic must become a transient coded error, got %v", err)
	}
	if out := provkit.Dispatch(context.Background(), panicky{}, "acton_nope", []byte(`{}`)); len(out) == 0 {
		t.Fatal("unknown exports still answer")
	}
	if out := provkit.Dispatch(context.Background(), panicky{}, abi.ExportMailList, []byte(`not json`)); len(out) == 0 {
		t.Fatal("bad input still answers")
	}
}

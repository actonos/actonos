package mailcal

import (
	"context"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/plugin"
)

// StoreCaller adapts the reference store to Caller so the in-process provider takes
// exactly the same JSON path as a real plugin. An operation the store does not know
// reports ErrFunctionNotFound, like a WASM module missing an export.
type StoreCaller struct{ Store *refstore.Store }

// CallJSON implements Caller.
func (c StoreCaller) CallJSON(_ context.Context, export string, req []byte) ([]byte, error) {
	known := false
	for _, name := range abi.AllExports {
		if name == export {
			known = true
			break
		}
	}
	if !known {
		return nil, plugin.ErrFunctionNotFound
	}
	return c.Store.Handle(export, req), nil
}

// NewMemProvider returns an in-process Provider backed by a fresh reference store seeded
// with seed. Use refstore.DefaultSeed() for the standard dataset.
func NewMemProvider(seed refstore.Seed) *BridgeProvider {
	return NewBridgeProvider(StoreCaller{Store: refstore.New(seed)})
}

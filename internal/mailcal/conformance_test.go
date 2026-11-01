package mailcal_test

import (
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/conformance"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
	"github.com/actonos/actonos/internal/mailcal/refstore"
)

// The in-process reference provider and the real WASM guest must be indistinguishable.
func TestConformanceMemProvider(t *testing.T) {
	conformance.RunSeeded(t, func(t *testing.T) mailcal.Provider {
		return mailcal.NewMemProvider(refstore.DefaultSeed())
	})
}

func TestConformanceWasmGuest(t *testing.T) {
	conformance.RunSeeded(t, func(t *testing.T) mailcal.Provider {
		return mailcal.NewBridgeProvider(refplugintest.Instance(t))
	})
}

func TestConformanceShapeOnlyIsEnoughForAnyProvider(t *testing.T) {
	conformance.RunShape(t, mailcal.NewMemProvider(refstore.DefaultSeed()), abi.AccountRef{ID: "sandbox"})
}

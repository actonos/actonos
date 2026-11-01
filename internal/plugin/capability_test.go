package plugin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCallJSONRoundTripReturnsACopy(t *testing.T) {
	inst := newABIInstance(t, 0)
	first, err := inst.CallJSON(context.Background(), "acton_connector_handle_webhook", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("CallJSON: %v", err)
	}
	if string(first) != abiWebhookResult {
		t.Fatalf("response = %q, want %q", first, abiWebhookResult)
	}
	// Scribbling over the returned slice must not corrupt guest memory for the next call.
	for i := range first {
		first[i] = 'X'
	}
	second, err := inst.CallJSON(context.Background(), "acton_connector_handle_webhook", []byte(`{"x":2}`))
	if err != nil || string(second) != abiWebhookResult {
		t.Fatalf("second call = %q, %v", second, err)
	}
}

func TestCallJSONMissingExportIsDistinguishable(t *testing.T) {
	inst := newABIInstance(t, 0)
	_, err := inst.CallJSON(context.Background(), "acton_mail_list", []byte(`{}`))
	if !errors.Is(err, ErrFunctionNotFound) {
		t.Fatalf("a missing export must wrap ErrFunctionNotFound, got %v", err)
	}
}

func TestCallJSONRejectsEmptyRequest(t *testing.T) {
	inst := newABIInstance(t, 0)
	if _, err := inst.CallJSON(context.Background(), "acton_connector_handle_webhook", nil); err == nil {
		t.Fatal("an empty request must be rejected before it reaches the guest")
	}
}

func TestCallJSONHonoursCancelledContext(t *testing.T) {
	inst := newABIInstance(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := inst.CallJSON(ctx, "acton_connector_handle_webhook", []byte(`{}`)); err == nil {
		t.Fatal("a cancelled context must fail the call")
	}
}

type recordingHook struct {
	mu          sync.Mutex
	activated   []string
	deactivated []string
	onActivated func(PluginManifest, Caller)
}

func (h *recordingHook) PluginActivated(_ context.Context, m PluginManifest, c Caller) {
	h.mu.Lock()
	h.activated = append(h.activated, m.ID)
	h.mu.Unlock()
	if h.onActivated != nil {
		h.onActivated(m, c)
	}
}

func (h *recordingHook) PluginDeactivated(_ context.Context, id string) {
	h.mu.Lock()
	h.deactivated = append(h.deactivated, id)
	h.mu.Unlock()
}

func newHookedManager(t *testing.T, hook CapabilityHook) *Manager {
	t.Helper()
	ctx := context.Background()
	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loader.Close(ctx) })
	m := NewManager(loader, nil, nil, nil, nil, nil, t.TempDir())
	m.SetCapabilityHook(hook)
	return m
}

func TestCapabilityHookSeesActivationAfterTheLockIsReleased(t *testing.T) {
	var m *Manager
	hook := &recordingHook{}
	hook.onActivated = func(manifest PluginManifest, caller Caller) {
		// Would deadlock if the Manager lock were still held.
		if _, ok := m.GetPlugin(manifest.ID); !ok {
			t.Errorf("plugin %s is not registered yet when the hook runs", manifest.ID)
		}
		out, err := caller.CallJSON(context.Background(), "acton_connector_handle_webhook", []byte(`{}`))
		if err != nil || string(out) != abiWebhookResult {
			t.Errorf("hook could not call the plugin: %q, %v", out, err)
		}
	}
	m = newHookedManager(t, hook)

	manifest := PluginManifest{ID: "mailish", Name: "Mailish", Version: "1.0.0", Capabilities: []string{"mail@1"}}
	if err := m.activatePlugin(context.Background(), manifest, buildABIModule(0), t.TempDir(), true); err != nil {
		t.Fatalf("activatePlugin: %v", err)
	}
	if len(hook.activated) != 1 || hook.activated[0] != "mailish" {
		t.Fatalf("activated = %v", hook.activated)
	}
}

func TestCapabilityHookIsToldWhenAPluginGoesAway(t *testing.T) {
	hook := &recordingHook{}
	m := newHookedManager(t, hook)
	manifest := PluginManifest{ID: "mailish", Name: "Mailish", Version: "1.0.0", Capabilities: []string{"calendar@1"}}
	if err := m.activatePlugin(context.Background(), manifest, buildABIModule(0), t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	if err := m.DisablePlugin(context.Background(), "mailish"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(hook.deactivated, ",") != "mailish" {
		t.Fatalf("deactivated = %v", hook.deactivated)
	}
}

func TestReplacingAPluginDeactivatesTheOldInstanceFirst(t *testing.T) {
	hook := &recordingHook{}
	m := newHookedManager(t, hook)
	manifest := PluginManifest{ID: "mailish", Name: "Mailish", Version: "1.0.0", Capabilities: []string{"mail@1"}}
	for i := 0; i < 2; i++ {
		if err := m.activatePlugin(context.Background(), manifest, buildABIModule(0), t.TempDir(), true); err != nil {
			t.Fatal(err)
		}
	}
	if len(hook.activated) != 2 || len(hook.deactivated) != 1 {
		t.Fatalf("activated=%v deactivated=%v; a reload must deactivate the old instance once", hook.activated, hook.deactivated)
	}
}

func TestNoHookIsFine(t *testing.T) {
	m := newHookedManager(t, nil)
	manifest := PluginManifest{ID: "plain", Name: "Plain", Version: "1.0.0", Capabilities: []string{"tool"}}
	if err := m.activatePlugin(context.Background(), manifest, buildABIModule(0), t.TempDir(), true); err != nil {
		t.Fatalf("activation without a hook must be unaffected: %v", err)
	}
}

// buildSpinModule exports acton_spin, which never returns, and the usual alloc/memory.
func buildSpinModule() []byte {
	const i32, i64 = 0x7f, 0x7e
	types := vec(
		[]byte{0x60, 0x01, i32, 0x01, i32},
		[]byte{0x60, 0x02, i32, i32, 0x01, i64},
	)
	funcs := vec([]byte{0}, []byte{1})
	mem := vec([]byte{0x00, 0x01})
	exports := vec(
		append(name("memory"), 0x02, 0x00),
		append(name("acton_alloc"), 0x00, 0x00),
		append(name("acton_spin"), 0x00, 0x01),
	)
	spin := []byte{0x03, 0x40, 0x0c, 0x00, 0x0b, 0x42, 0x00} // loop; br 0; end; i64.const 0
	code := vec(funcBody(i32Const(100)...), funcBody(spin...))
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, s := range [][]byte{section(1, types), section(3, funcs), section(5, mem), section(7, exports), section(10, code)} {
		out = append(out, s...)
	}
	return out
}

func TestCallJSONInterruptsAGuestThatNeverReturns(t *testing.T) {
	ctx := context.Background()
	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close(ctx)
	manifest := PluginManifest{ID: "spinner", Capabilities: []string{"mail@1"}}
	if _, err := loader.Compile(ctx, "spinner", buildSpinModule()); err != nil {
		t.Fatal(err)
	}
	host := &HostContext{PluginID: "spinner", Manifest: manifest, Gate: NewSecurityGate(manifest)}
	inst, err := loader.Instantiate(ctx, "spinner", manifest, host)
	if err != nil {
		t.Fatal(err)
	}

	callCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := inst.CallJSON(callCtx, "acton_spin", []byte(`{}`))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a call that exceeds its deadline must fail")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the guest was not interrupted; the context deadline has no effect")
	}
}

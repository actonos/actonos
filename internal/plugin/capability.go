package plugin

import (
	"context"
	"fmt"
	"time"
)

// pluginCallTimeout bounds a single CallJSON. It is shorter than pluginToolTimeout
// because capability calls (mail, calendar) are interactive requests, and the caller's
// own context deadline still applies when it is sooner.
const pluginCallTimeout = 90 * time.Second

// Caller invokes an export that follows the JSON-in/JSON-out convention: the request is
// copied into guest memory, the export is called with (ptr, len) and returns
// (resPtr<<32 | resLen). *PluginInstance implements it.
type Caller interface {
	CallJSON(ctx context.Context, export string, req []byte) ([]byte, error)
}

var _ Caller = (*PluginInstance)(nil)

// CallJSON calls export with req and returns a copy of the response bytes. It wraps
// ErrFunctionNotFound when the module does not export the function, so callers can tell
// "not implemented" from "failed". Calls on one instance are serialized.
func (inst *PluginInstance) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()

	fn := inst.mod.ExportedFunction(export)
	if fn == nil {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, export)
	}
	if len(req) == 0 {
		return nil, fmt.Errorf("calling %s: empty request", export)
	}

	callCtx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	callCtx = WithHostContext(callCtx, inst.hostCtx)

	packed, err := writeBufferToGuest(callCtx, inst.hostCtx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: allocating request for %s: %v", ErrMemoryAllocation, export, err)
	}

	results, err := fn.Call(callCtx, packed>>32, packed&0xffffffff)
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", export, err)
	}
	if len(results) == 0 || results[0] == 0 {
		return nil, fmt.Errorf("calling %s: empty response", export)
	}

	resPtr, resLen := uint32(results[0]>>32), uint32(results[0])
	view, err := readBufferFromMemory(inst.mod.Memory(), resPtr, resLen)
	if err != nil {
		return nil, fmt.Errorf("reading %s response: %w", export, err)
	}
	// The memory view is only valid until the guest runs again; the caller keeps a copy.
	return append([]byte(nil), view...), nil
}

// CapabilityHook lets a subsystem react to plugins that declare capabilities it owns
// (for example "mail@1"), without the plugin package knowing about that subsystem.
type CapabilityHook interface {
	// PluginActivated runs after a plugin is running and the Manager lock is released,
	// so the implementation may call back into the Manager.
	PluginActivated(ctx context.Context, manifest PluginManifest, caller Caller)
	// PluginDeactivated runs just before an instance is closed, while the Manager lock
	// IS held. Implementations must not call Manager methods; drop references and return.
	PluginDeactivated(ctx context.Context, pluginID string)
}

// SetCapabilityHook registers h. Call it before ScanAndLoadAll so plugins loaded at
// startup are announced. Passing nil removes the hook.
func (m *Manager) SetCapabilityHook(h CapabilityHook) {
	m.mu.Lock()
	m.capHook = h
	m.mu.Unlock()
}

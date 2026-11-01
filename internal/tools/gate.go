package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// GateAction is a ToolGate's verdict on one concrete tool call.
type GateAction string

const (
	// GateAllow lets the call continue to the normal threshold/approval logic.
	GateAllow GateAction = "allow"
	// GateRequireApproval forces a human approval for this exact input, regardless of
	// the agent's approval threshold, WithBypassApproval, or any active grant.
	GateRequireApproval GateAction = "needs_approval"
	// GateDeny refuses the call.
	GateDeny GateAction = "deny"
)

// GateDecision is what a ToolGate returns. Reason is shown to the operator and the agent.
type GateDecision struct {
	Action GateAction
	Reason string
}

// ToolGate is a mandatory, per-tool pre-execution check owned by the subsystem that
// registered the tool. It sees the same normalized input bytes the tool will receive, so
// the verdict cannot be separated from what runs.
type ToolGate interface {
	Check(ctx context.Context, agentID, toolName string, input json.RawMessage) (GateDecision, error)
}

var (
	// ErrToolDeniedByGate is returned when a gate denies a call or is missing/failing.
	ErrToolDeniedByGate = errors.New("tool call denied by gate")
)

// externalEffectTools are the tools whose effect leaves the device (mail, calendar). They
// are always High risk, can never be covered by an approval grant or auto-approval, and
// refuse to run unless a gate is registered for them.
var externalEffectTools = map[string]struct{}{
	"native_mail_send":             {},
	"native_mail_delete":           {},
	"native_calendar_create_event": {},
	"native_calendar_update_event": {},
	"native_calendar_respond":      {},
	"native_calendar_delete_event": {},
}

// IsExternalEffectTool reports whether name is a tool with an effect outside the device.
func IsExternalEffectTool(name string) bool {
	_, ok := externalEffectTools[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// RegisterGate installs gate for the named tools, replacing any earlier gate for them.
func (r *ToolRegistry) RegisterGate(gate ToolGate, toolNames ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gates == nil {
		r.gates = make(map[string]ToolGate)
	}
	for _, name := range toolNames {
		if gate == nil {
			delete(r.gates, name)
			continue
		}
		r.gates[name] = gate
	}
}

// RequireGate marks tools that must never run without a registered gate.
func (r *ToolRegistry) RequireGate(toolNames ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gateRequired == nil {
		r.gateRequired = make(map[string]struct{})
	}
	for _, name := range toolNames {
		r.gateRequired[name] = struct{}{}
	}
}

// evaluateGate runs the gate for name, failing closed. It returns whether approval is
// forced. Tools that are external-effect or marked with RequireGate and have no gate are denied.
func (r *ToolRegistry) evaluateGate(ctx context.Context, agentID, name string, input json.RawMessage) (forceApproval bool, err error) {
	r.mu.RLock()
	gate := r.gates[name]
	_, required := r.gateRequired[name]
	r.mu.RUnlock()

	if gate == nil {
		if required || IsExternalEffectTool(name) {
			return false, fmt.Errorf("%w: %s has no gate registered", ErrToolDeniedByGate, name)
		}
		return false, nil
	}
	decision, err := gate.Check(ctx, agentID, name, input)
	if err != nil {
		return false, fmt.Errorf("%w: gate failed for %s: %v", ErrToolDeniedByGate, name, err)
	}
	switch decision.Action {
	case GateAllow:
		return false, nil
	case GateRequireApproval:
		return true, nil
	case GateDeny:
		return false, fmt.Errorf("%w: %s", ErrToolDeniedByGate, decision.Reason)
	default:
		return false, fmt.Errorf("%w: unknown gate action %q", ErrToolDeniedByGate, decision.Action)
	}
}

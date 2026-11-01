package tools

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/memory"
)

type stubExternalTool struct {
	name  string
	calls int
}

func (s *stubExternalTool) Name() string        { return s.name }
func (s *stubExternalTool) Description() string { return "stub" }
func (s *stubExternalTool) Category() string    { return "native" }
func (s *stubExternalTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (s *stubExternalTool) Execute(context.Context, json.RawMessage) (*ToolResult, error) {
	s.calls++
	return &ToolResult{Content: "done"}, nil
}

type funcGate func(ctx context.Context, agentID, tool string, input json.RawMessage) (GateDecision, error)

func (f funcGate) Check(ctx context.Context, agentID, tool string, input json.RawMessage) (GateDecision, error) {
	return f(ctx, agentID, tool, input)
}

func gateTestRegistry(t *testing.T, threshold string) (*ToolRegistry, *stubExternalTool) {
	t.Helper()
	db, err := memory.Open(filepath.Join(t.TempDir(), "gate.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	registry := NewToolRegistry(nil)
	tool := &stubExternalTool{name: "native_mail_send"}
	registry.Register(tool)
	registry.SetApprovalManager(NewApprovalManager(db.SQLDB()))
	registry.SetPolicyResolver(func(context.Context, string) (AgentToolPolicy, error) {
		return AgentToolPolicy{AuthorizedTools: []string{"*"}, ApprovalThreshold: threshold, AllowedPaths: []string{"*"}}, nil
	})
	return registry, tool
}

func allowAll() ToolGate {
	return funcGate(func(context.Context, string, string, json.RawMessage) (GateDecision, error) {
		return GateDecision{Action: GateAllow}, nil
	})
}

func TestExternalEffectToolWithoutGateIsDenied(t *testing.T) {
	registry, tool := gateTestRegistry(t, "low")
	_, err := registry.Execute(context.Background(), "agent", "native_mail_send", json.RawMessage(`{"draft_id":"d1"}`))
	if !errors.Is(err, ErrToolDeniedByGate) {
		t.Fatalf("expected ErrToolDeniedByGate, got %v", err)
	}
	if tool.calls != 0 {
		t.Fatal("tool ran without a gate")
	}
}

func TestRequireGateAppliesToAnyTool(t *testing.T) {
	registry, _ := gateTestRegistry(t, "low")
	other := &stubExternalTool{name: "native_custom_write"}
	registry.Register(other)
	registry.RequireGate("native_custom_write")
	if _, err := registry.Execute(context.Background(), "agent", "native_custom_write", json.RawMessage(`{}`)); !errors.Is(err, ErrToolDeniedByGate) {
		t.Fatalf("expected denial, got %v", err)
	}
	registry.RegisterGate(allowAll(), "native_custom_write")
	if _, err := registry.Execute(context.Background(), "agent", "native_custom_write", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("gated tool should run once a gate allows it: %v", err)
	}
}

func TestGateDenyAndErrorFailClosed(t *testing.T) {
	registry, tool := gateTestRegistry(t, "low")
	registry.RegisterGate(funcGate(func(context.Context, string, string, json.RawMessage) (GateDecision, error) {
		return GateDecision{Action: GateDeny, Reason: "blocked by rule"}, nil
	}), "native_mail_send")
	if _, err := registry.Execute(context.Background(), "agent", "native_mail_send", json.RawMessage(`{}`)); !errors.Is(err, ErrToolDeniedByGate) {
		t.Fatalf("deny: got %v", err)
	}

	registry.RegisterGate(funcGate(func(context.Context, string, string, json.RawMessage) (GateDecision, error) {
		return GateDecision{}, errors.New("rule store offline")
	}), "native_mail_send")
	if _, err := registry.Execute(context.Background(), "agent", "native_mail_send", json.RawMessage(`{}`)); !errors.Is(err, ErrToolDeniedByGate) {
		t.Fatalf("gate error must fail closed: got %v", err)
	}

	registry.RegisterGate(funcGate(func(context.Context, string, string, json.RawMessage) (GateDecision, error) {
		return GateDecision{Action: "banana"}, nil
	}), "native_mail_send")
	if _, err := registry.Execute(context.Background(), "agent", "native_mail_send", json.RawMessage(`{}`)); !errors.Is(err, ErrToolDeniedByGate) {
		t.Fatalf("unknown action must fail closed: got %v", err)
	}
	if tool.calls != 0 {
		t.Fatal("tool ran despite a denying gate")
	}
}

func needsApprovalGate() ToolGate {
	return funcGate(func(context.Context, string, string, json.RawMessage) (GateDecision, error) {
		return GateDecision{Action: GateRequireApproval, Reason: "no rule"}, nil
	})
}

// A gate that demands approval must win over a permissive threshold, WithBypassApproval
// and an existing grant. This is the property the whole auto-send design rests on.
func TestGateApprovalCannotBeBypassed(t *testing.T) {
	registry, tool := gateTestRegistry(t, "low") // "low": threshold alone never asks
	registry.RegisterGate(needsApprovalGate(), "native_mail_send")
	input := json.RawMessage(`{"draft_id":"d1"}`)

	ctx := WithBypassApproval(context.Background())
	_, err := registry.Execute(ctx, "agent", "native_mail_send", input)
	var approvalErr *ApprovalRequiredError
	if !errors.As(err, &approvalErr) {
		t.Fatalf("bypass context must not skip the gate, got %v", err)
	}
	if tool.calls != 0 {
		t.Fatal("tool ran without approval")
	}

	// A grant for the same agent+tool cannot be created for an external-effect tool.
	approved, err := registry.approvals.Decide(context.Background(), approvalErr.Approval.ID, "approved", "tester", "")
	if err != nil {
		t.Fatalf("approving: %v", err)
	}
	if _, err := registry.approvals.CreateGrant(context.Background(), approved, DontAskToday, "tester"); err == nil {
		t.Fatal("grants must not be available for external-effect tools")
	}

	// Approval of the exact input works, and a different input is rejected.
	ctx = WithApprovalID(context.Background(), approved.ID)
	if _, err := registry.Execute(ctx, "agent", "native_mail_send", input); err != nil {
		t.Fatalf("approved exact input should run: %v", err)
	}
	if _, err := registry.Execute(ctx, "agent", "native_mail_send", json.RawMessage(`{"draft_id":"d2"}`)); !errors.Is(err, ErrApprovalInvalid) {
		t.Fatalf("approval must bind to the exact input, got %v", err)
	}
}

func TestExternalEffectToolsAreHighRiskAndGrantIneligible(t *testing.T) {
	for name := range externalEffectTools {
		if ToolRiskLevel(name) != "High" {
			t.Errorf("%s: ToolRiskLevel = %s", name, ToolRiskLevel(name))
		}
		if ClassifyRisk(name, nil) != RiskTierHigh {
			t.Errorf("%s: ClassifyRisk = %s", name, ClassifyRisk(name, nil))
		}
		if GrantEligibleTool(name) {
			t.Errorf("%s must not be grant-eligible", name)
		}
		if CanAutoApprove(RiskTierLow, name) && ClassifyRisk(name, nil) != RiskTierHigh {
			t.Errorf("%s must not be auto-approvable", name)
		}
	}
	if IsExternalEffectTool("native_mail_list") || IsExternalEffectTool("native_mail_draft") {
		t.Fatal("read and draft tools are not external-effect tools")
	}
}

func TestGateAllowStillRespectsThreshold(t *testing.T) {
	registry, tool := gateTestRegistry(t, "high")
	registry.RegisterGate(allowAll(), "native_mail_send")
	// Gate allow means "no extra restriction"; the agent's own threshold still applies.
	_, err := registry.Execute(context.Background(), "agent", "native_mail_send", json.RawMessage(`{}`))
	var approvalErr *ApprovalRequiredError
	if !errors.As(err, &approvalErr) {
		t.Fatalf("threshold should still ask for High risk, got %v", err)
	}
	if tool.calls != 0 {
		t.Fatal("tool ran before approval")
	}
}

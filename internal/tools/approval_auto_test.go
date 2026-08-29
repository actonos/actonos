package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		toolName string
		input    string
		wantTier string
	}{
		{"native_file_read", `{"path":"a.txt"}`, RiskTierLow},
		{"native_sysinfo", `{}`, RiskTierLow},
		{"native_workspace_list", `{}`, RiskTierLow},
		{"native_file_write", `{"path":"a.txt"}`, RiskTierMedium},
		{"native_http_fetch", `{"url":"https://example.com"}`, RiskTierMedium},
		{"native_exec", `{"cmd":"ls"}`, RiskTierHigh},
		{"native_file_delete", `{"path":"a.txt"}`, RiskTierHigh},
		{"system_restart", `{}`, RiskTierHigh},
		{"system_ota_apply", `{}`, RiskTierHigh},
		{"mcp_custom_tool", `{}`, RiskTierHigh},
		{"wasm_plugin_tool", `{}`, RiskTierHigh},
	}

	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			got := ClassifyRisk(tc.toolName, json.RawMessage(tc.input))
			if got != tc.wantTier {
				t.Fatalf("ClassifyRisk(%s) = %s, want %s", tc.toolName, got, tc.wantTier)
			}
		})
	}
}

func TestCanAutoApprove(t *testing.T) {
	if !CanAutoApprove(RiskTierLow, "native_file_read") {
		t.Fatal("expected native_file_read with RiskTierLow to be auto-approvable")
	}
	if CanAutoApprove(RiskTierHigh, "native_exec") {
		t.Fatal("expected native_exec with RiskTierHigh NOT to be auto-approvable")
	}
	if CanAutoApprove(RiskTierMedium, "native_file_write") {
		t.Fatal("expected native_file_write with RiskTierMedium NOT to be auto-approvable")
	}
	if CanAutoApprove(RiskTierLow, "native_file_delete") {
		t.Fatal("expected delete tool to be blocked by blacklist even if marked low")
	}
}

func TestApprovalSweeper(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE IF NOT EXISTS approvals (
		id TEXT PRIMARY KEY,
		trace_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		risk_level TEXT NOT NULL,
		risk_tier TEXT NOT NULL DEFAULT 'medium',
		auto_approve_after INTEGER NOT NULL DEFAULT 0,
		auto_approve_scope TEXT NOT NULL DEFAULT '',
		action_hash TEXT NOT NULL,
		input_json TEXT NOT NULL,
		status TEXT NOT NULL,
		reason TEXT,
		requested_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		decided_at TIMESTAMP,
		decided_by TEXT,
		task_id TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT ''
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating schema: %v", err)
	}

	mgr := NewApprovalManager(db)
	ctx := context.Background()

	// 1. Create a low-risk request that should auto-approve
	req, err := mgr.Request(ctx, "trace_1", "agent_1", "native_file_read", "Low", json.RawMessage(`{"path":"test.txt"}`))
	if err != nil {
		t.Fatalf("requesting approval: %v", err)
	}
	if req.RiskTier != RiskTierLow {
		t.Fatalf("expected RiskTierLow, got %s", req.RiskTier)
	}
	if req.AutoApproveAfter <= 0 {
		t.Fatalf("expected AutoApproveAfter > 0, got %d", req.AutoApproveAfter)
	}

	// 2. Artificially backdate requested_at to 5 hours ago so it qualifies for sweep
	fiveHoursAgo := time.Now().UTC().Add(-5 * time.Hour)
	if _, err := db.Exec("UPDATE approvals SET requested_at = ? WHERE id = ?", fiveHoursAgo, req.ID); err != nil {
		t.Fatalf("updating requested_at: %v", err)
	}

	// 3. Create a high-risk request that should NOT auto-approve even when backdated
	reqHigh, err := mgr.Request(ctx, "trace_2", "agent_1", "native_exec", "High", json.RawMessage(`{"cmd":"rm -rf"}`))
	if err != nil {
		t.Fatalf("requesting high approval: %v", err)
	}
	if _, err := db.Exec("UPDATE approvals SET requested_at = ?, auto_approve_after = 1 WHERE id = ?", fiveHoursAgo, reqHigh.ID); err != nil {
		t.Fatalf("updating requested_at high: %v", err)
	}

	// 4. Run sweep
	resolved, err := mgr.SweepAutoApprovals(ctx)
	if err != nil {
		t.Fatalf("SweepAutoApprovals failed: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved approval, got %d", len(resolved))
	}
	if resolved[0].ID != req.ID {
		t.Fatalf("expected resolved ID %s, got %s", req.ID, resolved[0].ID)
	}
	if resolved[0].Status != "approved" || resolved[0].DecidedBy != "auto" {
		t.Fatalf("expected status=approved and decided_by=auto, got status=%s decided_by=%s", resolved[0].Status, resolved[0].DecidedBy)
	}

	// Verify high-risk is still pending
	curHigh, err := mgr.Get(ctx, reqHigh.ID)
	if err != nil {
		t.Fatalf("loading high request: %v", err)
	}
	if curHigh.Status != "pending" {
		t.Fatalf("expected high risk request to remain pending, got %s", curHigh.Status)
	}
}

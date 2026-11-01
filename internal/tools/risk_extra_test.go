package tools

import "testing"

func TestClassifyRiskTable(t *testing.T) {
	cases := []struct {
		tool string
		want string
	}{
		// Always high
		{"native_exec", RiskTierHigh},
		{"NATIVE_EXEC", RiskTierHigh},
		{"  native_exec  ", RiskTierHigh},
		{"native_cron_schedule", RiskTierHigh},
		{"system_restart", RiskTierHigh},
		{"admin_anything_at_all", RiskTierHigh},
		{"native_vault_write", RiskTierHigh},
		{"vault_set_secret", RiskTierHigh},
		{"native_file_delete", RiskTierHigh},
		{"native_workspace_remove", RiskTierHigh},
		{"mcp_github_create_issue", RiskTierHigh},
		{"wasm_custom_tool", RiskTierHigh},
		// A destructive suffix beats a read-only prefix
		{"native_file_read_and_delete", RiskTierHigh},
		{"native_task_delete", RiskTierHigh},
		// Read-only
		{"native_file_read", RiskTierLow},
		{"native_workspace_list", RiskTierLow},
		{"native_file_search", RiskTierLow},
		{"native_sysinfo", RiskTierLow},
		{"native_task_status", RiskTierLow},
		{"native_file_exists", RiskTierLow},
		// Workspace writes and network fetches
		{"native_file_write", RiskTierMedium},
		{"native_file_edit", RiskTierMedium},
		{"native_http_fetch", RiskTierMedium},
		{"native_channel_notify", RiskTierMedium},
		// Unknown tools must never default to low
		{"totally_new_tool", RiskTierMedium},
		{"", RiskTierMedium},
	}
	for _, tc := range cases {
		if got := ClassifyRisk(tc.tool, nil); got != tc.want {
			t.Errorf("ClassifyRisk(%q) = %q, want %q", tc.tool, got, tc.want)
		}
	}
}

func TestCanAutoApproveIsLowRiskOnly(t *testing.T) {
	if !CanAutoApprove(RiskTierLow, "native_file_read") {
		t.Fatal("a low-risk read should be auto-approvable")
	}
	if !CanAutoApprove(" LOW ", "native_sysinfo") {
		t.Fatal("tier matching should ignore case and whitespace")
	}
	for _, tier := range []string{RiskTierMedium, RiskTierHigh, "", "unknown", "critical"} {
		if CanAutoApprove(tier, "native_file_read") {
			t.Errorf("tier %q must never be auto-approved", tier)
		}
	}
	// Even a mislabelled "low" tier cannot auto-approve a dangerous primitive.
	for _, tool := range []string{
		"native_exec", "native_file_delete", "native_workspace_remove", "vault_read",
		"system_restart", "system_ota_apply", "native_cron_schedule", "admin_reset", "ADMIN_RESET",
	} {
		if CanAutoApprove(RiskTierLow, tool) {
			t.Errorf("%q must never be auto-approved even when labelled low", tool)
		}
	}
}

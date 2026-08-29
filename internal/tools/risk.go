package tools

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	// RiskTierLow represents safe, read-only or low-impact operations.
	RiskTierLow = "low"
	// RiskTierMedium represents standard workspace modifications or internal actions.
	RiskTierMedium = "medium"
	// RiskTierHigh represents high-impact, external, destructive, or system-altering actions.
	RiskTierHigh = "high"

	// AutoApproveScopeThisOnly applies auto-approval only to the exact request.
	AutoApproveScopeThisOnly = "this_only"
	// AutoApproveScopeSession applies to the originating session.
	AutoApproveScopeSession = "session"
	// AutoApproveScopeAgent applies to the agent for similar operations.
	AutoApproveScopeAgent = "agent"
	// AutoApproveScopeNever forbids auto-approval.
	AutoApproveScopeNever = "never"

	// DefaultLowRiskAutoApproveDuration is the default timeout for auto-approving low-risk items (4 hours).
	DefaultLowRiskAutoApproveDuration = 4 * time.Hour
)

// ClassifyRisk determines the RiskTier ("low", "medium", "high") of a tool execution.
// - Low: safe workspace reading, system status queries, idempotent reads.
// - Medium: local workspace modifications, safe http fetch, internal notifications.
// - High: shell execution, file deletion, root/system modifications, external messaging,
//         vault writes, system reboot, OTA updates, cron schedule.
func ClassifyRisk(toolName string, input json.RawMessage) string {
	name := strings.TrimSpace(strings.ToLower(toolName))

	// Critical / High risk operations:
	if name == "native_exec" ||
		name == "native_cron_schedule" ||
		name == "system_restart" ||
		name == "admin_restart" ||
		name == "system_ota_apply" ||
		name == "admin_ota_apply" ||
		name == "system_mcp_connect" ||
		strings.HasPrefix(name, "admin_") ||
		strings.Contains(name, "vault_write") ||
		strings.Contains(name, "vault_delete") ||
		strings.Contains(name, "vault_set") {
		return RiskTierHigh
	}

	// Deletion operations
	if strings.Contains(name, "delete") || strings.Contains(name, "remove") {
		return RiskTierHigh
	}

	// MCP or WASM extensions default to High unless explicitly categorized
	if strings.HasPrefix(name, "mcp_") || strings.HasPrefix(name, "wasm_") {
		return RiskTierHigh
	}

	// Low risk tools: Read-only, inspection, status
	if strings.HasPrefix(name, "native_file_read") ||
		strings.HasPrefix(name, "native_workspace_read") ||
		strings.HasPrefix(name, "native_file_list") ||
		strings.HasPrefix(name, "native_workspace_list") ||
		strings.HasPrefix(name, "native_file_search") ||
		strings.HasPrefix(name, "native_sysinfo") ||
		strings.HasPrefix(name, "native_task_") ||
		name == "native_workspace_get" ||
		name == "native_file_exists" {
		return RiskTierLow
	}

	// Medium risk tools: localized writes within workspace
	if strings.HasPrefix(name, "native_file_write") ||
		strings.HasPrefix(name, "native_file_edit") ||
		strings.HasPrefix(name, "native_workspace_write") ||
		strings.HasPrefix(name, "native_browser_navigate") ||
		strings.HasPrefix(name, "native_browser_screenshot") ||
		strings.HasPrefix(name, "native_http_fetch") ||
		strings.HasPrefix(name, "native_web_search") ||
		strings.HasPrefix(name, "native_channel_notify") {
		return RiskTierMedium
	}

	return RiskTierMedium
}

// CanAutoApprove reports whether an approval request is eligible for automated approval after timeout.
// It strictly enforces that ONLY low-risk actions can be auto-approved, and blacklists dangerous primitives.
func CanAutoApprove(riskTier string, toolName string) bool {
	tier := strings.ToLower(strings.TrimSpace(riskTier))
	if tier != RiskTierLow && tier != "low" {
		return false
	}
	// Extra safety blacklist for never-auto-approve tools
	name := strings.ToLower(strings.TrimSpace(toolName))
	if strings.Contains(name, "exec") ||
		strings.Contains(name, "delete") ||
		strings.Contains(name, "remove") ||
		strings.Contains(name, "vault") ||
		strings.Contains(name, "restart") ||
		strings.Contains(name, "ota") ||
		strings.Contains(name, "cron") ||
		strings.HasPrefix(name, "admin_") {
		return false
	}
	return true
}

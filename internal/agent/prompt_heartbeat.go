package agent

import (
	"fmt"
	"strings"
)

// BuildHeartbeatMissionPrompt formats the autonomous backlog mission execution cycle prompt.
func BuildHeartbeatMissionPrompt(taskTitle, taskDesc, standingDirectives string) string {
	var sb strings.Builder
	sb.WriteString("<autonomous_mission_cycle>\n")
	fmt.Fprintf(&sb, "  <mission_title>%s</mission_title>\n", taskTitle)
	if taskDesc != "" {
		fmt.Fprintf(&sb, "  <mission_directive>\n%s\n  </mission_directive>\n", indentContent(strings.TrimSpace(taskDesc), "    "))
	}
	if standingDirectives != "" {
		fmt.Fprintf(&sb, "  <standing_directives>\n%s\n  </standing_directives>\n", indentContent(strings.TrimSpace(standingDirectives), "    "))
	}
	sb.WriteString("  <execution_protocol>\n")
	sb.WriteString("    <rule>Autonomously inspect, execute, and verify concrete progress toward completing this mission using your authorized tools.</rule>\n")
	sb.WriteString("    <rule>Provide a decisive progress report summarizing tool actions taken and verified state changes.</rule>\n")
	sb.WriteString("  </execution_protocol>\n")
	sb.WriteString("</autonomous_mission_cycle>")
	return sb.String()
}

// BuildHeartbeatPulsePrompt formats the periodic autonomous background heartbeat check prompt.
func BuildHeartbeatPulsePrompt(standingDirectives, backlogSummary string) string {
	var sb strings.Builder
	sb.WriteString("<autonomous_heartbeat_pulse>\n")
	if standingDirectives != "" {
		fmt.Fprintf(&sb, "  <standing_directives>\n%s\n  </standing_directives>\n", indentContent(strings.TrimSpace(standingDirectives), "    "))
	} else {
		sb.WriteString("  <standing_directives>Inspect system health and autonomously verify local workspace integrity.</standing_directives>\n")
	}
	if backlogSummary != "" {
		fmt.Fprintf(&sb, "  <backlog_state>\n%s\n  </backlog_state>\n", indentContent(strings.TrimSpace(backlogSummary), "    "))
	}
	sb.WriteString("  <execution_protocol>\n")
	sb.WriteString("    <rule>If all systems and state are healthy with NO pending actionable tasks, reply with EXACTLY `HEARTBEAT_OK`.</rule>\n")
	sb.WriteString("    <rule>If an anomaly or actionable task is identified, execute corrective tools and report findings.</rule>\n")
	sb.WriteString("  </execution_protocol>\n")
	sb.WriteString("</autonomous_heartbeat_pulse>")
	return sb.String()
}

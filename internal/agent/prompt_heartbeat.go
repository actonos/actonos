package agent

import (
	"fmt"
	"strings"
)

// BuildHeartbeatMissionPrompt formats the autonomous backlog mission execution cycle prompt.
func BuildHeartbeatMissionPrompt(taskTitle, taskDesc, standingDirectives string, skills ...SkillPromptEntry) string {
	var sb strings.Builder
	sb.WriteString("<autonomous_mission_cycle>\n")
	fmt.Fprintf(&sb, "  <mission_title>%s</mission_title>\n", taskTitle)
	if taskDesc != "" {
		fmt.Fprintf(&sb, "  <mission_directive>\n%s\n  </mission_directive>\n", indentContent(strings.TrimSpace(taskDesc), "    "))
	}
	if standingDirectives != "" {
		fmt.Fprintf(&sb, "  <standing_directives>\n%s\n  </standing_directives>\n", indentContent(strings.TrimSpace(standingDirectives), "    "))
	}
	if catalog := renderAvailableSkills(skills); catalog != "" {
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(catalog, "\n", "\n  "))
		sb.WriteString("\n")
	}
	sb.WriteString("  <execution_protocol>\n")
	sb.WriteString("    <rule>Autonomously inspect, execute, and verify concrete progress toward completing this mission using your authorized tools.</rule>\n")
	if len(skills) > 0 {
		sb.WriteString("    <rule>If an available skill matches this mission, invoke that skill tool FIRST and follow its returned instructions before improvising the deliverable.</rule>\n")
	}
	sb.WriteString("    <rule>Provide a decisive progress report summarizing tool actions taken and verified state changes.</rule>\n")
	sb.WriteString("    <rule>Do NOT call `native_channel_notify` unless the mission directive explicitly instructs you to send an external message. Deliver your final output directly in your response.</rule>\n")
	sb.WriteString("    <rule>Do exactly what was requested. Deliver the result directly without conversational filler, pleasantries, or asking follow-up questions (e.g. never ask 'do you want me to do more?').</rule>\n")
	sb.WriteString("  </execution_protocol>\n")
	sb.WriteString("</autonomous_mission_cycle>")
	return sb.String()
}

// BuildHeartbeatPulsePrompt formats the periodic autonomous background heartbeat check prompt.
func BuildHeartbeatPulsePrompt(standingDirectives, backlogSummary string) string {
	var sb strings.Builder
	sb.WriteString("<autonomous_heartbeat_pulse>\n")
	hasDirectives := strings.TrimSpace(standingDirectives) != ""
	if hasDirectives {
		fmt.Fprintf(&sb, "  <standing_directives>\n%s\n  </standing_directives>\n", indentContent(strings.TrimSpace(standingDirectives), "    "))
	} else {
		sb.WriteString("  <standing_directives>Inspect system health and autonomously verify local workspace integrity.</standing_directives>\n")
	}
	if backlogSummary != "" {
		fmt.Fprintf(&sb, "  <backlog_state>\n%s\n  </backlog_state>\n", indentContent(strings.TrimSpace(backlogSummary), "    "))
	}
	sb.WriteString("  <execution_protocol>\n")
	if hasDirectives {
		sb.WriteString("    <rule>Autonomously execute the standing directives above, perform any requested actions, and report substantive findings or output.</rule>\n")
		sb.WriteString("    <rule>Do NOT invoke `native_channel_notify` unless the standing directive explicitly asks to send an external notification. Output the full requested content directly in your response text.</rule>\n")
		sb.WriteString("    <rule>Deliver the requested output directly and concisely. Do NOT ask follow-up questions, do NOT offer unsolicited future tasks (e.g. never say 'if you want I can tell another story...'), and do NOT add conversational filler.</rule>\n")
		sb.WriteString("    <rule>Only reply with `HEARTBEAT_OK` if there are no pending tasks AND the standing directives require no action or new output.</rule>\n")
	} else {
		sb.WriteString("    <rule>If all systems and state are healthy with NO pending actionable tasks, reply with EXACTLY `HEARTBEAT_OK`.</rule>\n")
		sb.WriteString("    <rule>If an anomaly or actionable task is identified, execute corrective tools and report findings.</rule>\n")
	}
	sb.WriteString("  </execution_protocol>\n")
	sb.WriteString("</autonomous_heartbeat_pulse>")
	return sb.String()
}

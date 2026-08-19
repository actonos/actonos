package agent

import (
	"fmt"
	"strings"
)

// BuildReflectionPrompt formats the memory reflection and knowledge distillation prompt.
func BuildReflectionPrompt(userMsg, asstResp string) string {
	var sb strings.Builder

	sb.WriteString("<memory_reflection_task>\n")
	sb.WriteString("  <objective>Analyze the following conversation turn to extract durable user preferences and memorable episodic knowledge.</objective>\n\n")

	sb.WriteString("  <conversation_turn>\n")
	fmt.Fprintf(&sb, "    <user_message>\n%s\n    </user_message>\n", indentContent(strings.TrimSpace(userMsg), "      "))
	fmt.Fprintf(&sb, "    <assistant_response>\n%s\n    </assistant_response>\n", indentContent(strings.TrimSpace(asstResp), "      "))
	sb.WriteString("  </conversation_turn>\n\n")

	sb.WriteString("  <extraction_rules>\n")
	sb.WriteString("    <rule id=\"preference\">Extract `preference_key` & `preference_value` only for genuine long-term user preferences (e.g. coding conventions, language choices, architectural rules). NEVER extract transient requests, bot directives, or heartbeat tasks. If none, leave empty \"\".</rule>\n")
	sb.WriteString("    <rule id=\"episodic_memory\">Extract `episodic_memory` as a concise 1-2 sentence summary of key decisions, facts, or technical solutions reached in this turn that will be valuable for future sessions. If trivial greeting or ephemeral chit-chat, leave empty \"\".</rule>\n")
	sb.WriteString("    <rule id=\"output_format\">Respond STRICTLY with valid JSON matching the schema below, without markdown commentary.</rule>\n")
	sb.WriteString("  </extraction_rules>\n\n")

	sb.WriteString("  <output_schema>\n")
	sb.WriteString(`{
  "preference_key": "",
  "preference_value": "",
  "episodic_memory": ""
}`)
	sb.WriteString("\n  </output_schema>\n")
	sb.WriteString("</memory_reflection_task>")

	return sb.String()
}

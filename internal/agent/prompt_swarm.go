package agent

import (
	"fmt"
	"strings"
)

// BuildSwarmDelegationPrompt constructs the structured system and user prompts for sub-agent execution.
func BuildSwarmDelegationPrompt(parentName, taskTitle, taskPrompt string) (systemPrompt, userPrompt string) {
	var sys strings.Builder
	sys.WriteString("<delegated_subtask_role>\n")
	fmt.Fprintf(&sys, "  <parent_agent>%s</parent_agent>\n", parentName)
	fmt.Fprintf(&sys, "  <assigned_task>%s</assigned_task>\n", taskTitle)
	sys.WriteString("  <execution_directive>You are an atomic sub-agent executing a delegated task. Focus purely on completing the objective with tool execution evidence. Deliver concrete results with zero conversational filler.</execution_directive>\n")
	sys.WriteString("</delegated_subtask_role>")

	var usr strings.Builder
	usr.WriteString("<subtask_payload>\n")
	fmt.Fprintf(&usr, "  <title>%s</title>\n", taskTitle)
	fmt.Fprintf(&usr, "  <instructions>\n%s\n  </instructions>\n", indentContent(strings.TrimSpace(taskPrompt), "    "))
	usr.WriteString("</subtask_payload>")

	return sys.String(), usr.String()
}

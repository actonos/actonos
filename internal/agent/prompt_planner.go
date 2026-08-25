package agent

import (
	"fmt"
	"strings"
)

// BuildPlannerPrompt constructs a structured goal decomposition prompt with dynamically
// serialized agents and their authorized tools.
func BuildPlannerPrompt(goal string, agents []AgentManifest, skills ...SkillPromptEntry) string {
	var sb strings.Builder

	sb.WriteString("<goal_decomposition_task>\n")
	sb.WriteString("  <objective>Decompose the following high-level user goal into 2 to 5 sequential, dependency-aware actionable plan steps.</objective>\n")
	fmt.Fprintf(&sb, "  <target_goal>\n%s\n  </target_goal>\n", indentContent(strings.TrimSpace(goal), "    "))

	// Serialize available agents and their specialized capabilities
	sb.WriteString("  <available_agents>\n")
	if len(agents) > 0 {
		for _, a := range agents {
			toolsStr := "none"
			if len(a.AuthorizedTools) > 0 {
				toolsStr = strings.Join(a.AuthorizedTools, ", ")
			}
			desc := a.Description
			if desc == "" {
				desc = a.Name
			}
			fmt.Fprintf(&sb, "    <agent id=\"%s\" name=\"%s\" tools=\"%s\">%s</agent>\n",
				a.AgentID, a.Name, toolsStr, desc)
		}
	} else {
		sb.WriteString("    <agent id=\"agent_primary\" name=\"Primary Agent\" tools=\"all\">General purpose autonomous execution agent</agent>\n")
	}
	sb.WriteString("  </available_agents>\n\n")

	if catalog := renderAvailableSkills(skills); catalog != "" {
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(catalog, "\n", "\n  "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("  <planning_rules>\n")
	sb.WriteString("    <rule>Break down the goal into distinct, non-overlapping atomic steps.</rule>\n")
	sb.WriteString("    <rule>Assign each step to the most suitable `agent_id` from `<available_agents>` whose tools match the step's requirements.</rule>\n")
	if len(skills) > 0 {
		sb.WriteString("    <rule>When an available skill matches the goal, include an early step that invokes that skill tool and follows its instructions instead of reinventing the procedure.</rule>\n")
	}
	sb.WriteString("    <rule>Track sequential dependencies via `dependencies` (e.g. task_2 depends on task_1).</rule>\n")
	sb.WriteString("    <rule>Return ONLY the JSON array structure with no markdown conversational wrapper.</rule>\n")
	sb.WriteString("  </planning_rules>\n\n")

	sb.WriteString("  <output_schema>\n")
	sb.WriteString(`[
  {
    "id": "task_1",
    "description": "Clear action description",
    "agent_role": "<agent_id_from_available_agents>",
    "dependencies": []
  },
  {
    "id": "task_2",
    "description": "Dependent verification or execution step",
    "agent_role": "<agent_id_from_available_agents>",
    "dependencies": ["task_1"]
  }
]`)
	sb.WriteString("\n  </output_schema>\n")
	sb.WriteString("</goal_decomposition_task>")

	return sb.String()
}

// BuildPlanStepPrompt formats the step execution prompt for an autonomous planner sub-step.
func BuildPlanStepPrompt(stepID, goal, stepDesc, role, acceptance string, skills ...SkillPromptEntry) string {
	var sb strings.Builder
	sb.WriteString("<plan_step_execution>\n")
	fmt.Fprintf(&sb, "  <step_id>%s</step_id>\n", stepID)
	fmt.Fprintf(&sb, "  <overall_goal>%s</overall_goal>\n", goal)
	fmt.Fprintf(&sb, "  <step_objective>%s</step_objective>\n", stepDesc)
	fmt.Fprintf(&sb, "  <assigned_role>%s</assigned_role>\n", role)
	if catalog := renderAvailableSkills(skills); catalog != "" {
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(catalog, "\n", "\n  "))
		sb.WriteString("\n")
	}
	if acceptance != "" {
		fmt.Fprintf(&sb, "  <acceptance_criteria>%s</acceptance_criteria>\n", acceptance)
	} else {
		sb.WriteString("  <acceptance_criteria>Complete this step with tool evidence and explicit verification.</acceptance_criteria>\n")
	}
	if len(skills) > 0 {
		sb.WriteString("  <rule>If an available skill applies to this step, call that skill tool before producing the deliverable.</rule>\n")
	}
	sb.WriteString("</plan_step_execution>")
	return sb.String()
}

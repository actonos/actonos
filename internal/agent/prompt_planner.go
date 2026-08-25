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
	sb.WriteString("  <objective>Turn the user goal into a professional work-breakdown DAG the autonomous runtime can execute one step per turn.</objective>\n")
	fmt.Fprintf(&sb, "  <target_goal>\n%s\n  </target_goal>\n", indentContent(strings.TrimSpace(goal), "    "))

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
	sb.WriteString("    <rule>Read the target goal in whatever language it is written. Count deliverables from meaning, not from keywords.</rule>\n")
	sb.WriteString("    <rule>Emit 2 to 5 steps. A 1-step plan is allowed only when the whole goal is one action and you set `atomic` to true.</rule>\n")
	sb.WriteString("    <rule>If the goal requests more than one artifact (parts, files, chapters, PDFs, a series — in any language), emit one `kind=produce` step per artifact (cap 5). Never a single outline-only plan.</rule>\n")
	sb.WriteString("    <rule>Set `kind` to produce (tool-created artifact), research (gather facts), or verify (check prior artifacts).</rule>\n")
	sb.WriteString("    <rule>Each step is a named work package: verb-first title, one owner, one deliverable, one done-when test. Do not overlap scope with another step.</rule>\n")
	sb.WriteString("    <rule>Write `description` as an executable brief the assignee can finish in one tool-using turn: action, artifact, and constraints. Never paste the whole goal into every step.</rule>\n")
	sb.WriteString("    <rule>Write `acceptance` as an observable check (file exists, content matches X, command output, cited source). Vague claims such as 'looks good' are invalid.</rule>\n")
	sb.WriteString("    <rule>Order the DAG: gather facts or inputs, then produce, then verify or deliver. Independent work may run in parallel (empty or non-overlapping `dependencies`).</rule>\n")
	sb.WriteString("    <rule>Assign `agent_role` to a real `agent_id` from `<available_agents>` whose tools match the step. Use `general` only when no specialist exists.</rule>\n")
	sb.WriteString("    <rule>Forbidden steps: 'make a plan', 'think', 'start working', restating the goal, waiting for the user, or asking the user a question.</rule>\n")
	sb.WriteString("    <rule>IDs must be unique `task_1`..`task_n`. `dependencies` may only list earlier step IDs. Write titles and descriptions in the language of `<target_goal>`.</rule>\n")
	if len(skills) > 0 {
		sb.WriteString("    <rule>When an available skill matches the goal, include an early step that invokes that skill tool and follows its instructions instead of reinventing the procedure.</rule>\n")
	}
	sb.WriteString("    <rule>Return ONLY the JSON array. No markdown fences, no commentary.</rule>\n")
	sb.WriteString("  </planning_rules>\n\n")

	sb.WriteString("  <output_schema>\n")
	sb.WriteString(`[
  {
    "id": "task_1",
    "title": "Verb-first work package title",
    "description": "Do X and produce Y under constraint Z.",
    "acceptance": "Y exists and satisfies the stated check.",
    "agent_role": "<agent_id_from_available_agents>",
    "kind": "produce",
    "atomic": false,
    "dependencies": []
  },
  {
    "id": "task_2",
    "title": "Dependent production or verification step",
    "description": "Use the output of task_1 to produce the next artifact.",
    "acceptance": "The next artifact is verified against the goal.",
    "agent_role": "<agent_id_from_available_agents>",
    "kind": "produce",
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
	fmt.Fprintf(&sb, "  <step_objective>\n%s\n  </step_objective>\n", indentContent(strings.TrimSpace(stepDesc), "    "))
	fmt.Fprintf(&sb, "  <assigned_role>%s</assigned_role>\n", role)
	if catalog := renderAvailableSkills(skills); catalog != "" {
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(catalog, "\n", "\n  "))
		sb.WriteString("\n")
	}
	if acceptance != "" {
		fmt.Fprintf(&sb, "  <acceptance_criteria>\n%s\n  </acceptance_criteria>\n", indentContent(strings.TrimSpace(acceptance), "    "))
	} else {
		sb.WriteString("  <acceptance_criteria>Complete only this step with tool evidence. Do not start later steps.</acceptance_criteria>\n")
	}
	sb.WriteString("  <execution_rules>\n")
	sb.WriteString("    <rule>Execute only this step. Produce the step deliverable with tools. Do not begin later DAG steps.</rule>\n")
	sb.WriteString("    <rule>Do not ask the operator whether to continue, in any language. Do not offer optional next steps. The runtime starts the next DAG step by itself.</rule>\n")
	sb.WriteString("    <rule>Call tools to create or inspect the named deliverable. Do not claim the whole mission is done.</rule>\n")
	sb.WriteString("    <rule>When THIS step's acceptance criteria are met, stop. Do not ask. Write in the language of the overall goal.</rule>\n")
	if len(skills) > 0 {
		sb.WriteString("    <rule>If an available skill applies to this step, call that skill tool before producing the deliverable.</rule>\n")
	}
	sb.WriteString("  </execution_rules>\n")
	sb.WriteString("</plan_step_execution>")
	return sb.String()
}

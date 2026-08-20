package agent

import (
	"context"
	"strings"
	"testing"
)

func TestPromptBuilder_Composition(t *testing.T) {
	builder := NewPromptBuilder()
	builder.WithSection(&RawTextSection{Content: "First Header"})
	builder.WithSection(&RawTextSection{Content: ""}) // Empty section should be skipped
	builder.WithSection(&MetaDirectiveSection{})

	res := builder.Build()
	if !strings.Contains(res, "First Header") {
		t.Errorf("expected First Header in output, got: %s", res)
	}
	if !strings.Contains(res, "<operating_standards>") {
		t.Errorf("expected <operating_standards> in output, got: %s", res)
	}
}

func TestBuildCognitiveSystemPrompt(t *testing.T) {
	ctx := context.Background()
	agent := &AgentManifest{
		AgentID:            "agent_lead",
		Name:               "Lead Architect",
		Description:        "Full-stack Go and React developer",
		AuthorizedTools:    []string{"native_exec", "native_file_read", "native_file_write"},
		SystemInstructions: "Deliver clean, maintainable code with unit tests.",
	}

	prompt, _ := BuildCognitiveSystemPrompt(ctx, agent.AgentID, agent, "/data", "/data/workspace", nil, nil, "Hello")

	requiredTags := []string{
		"<operating_standards>",
		"<agent_identity",
		"<environment>",
		"<operational_constraints>",
	}

	for _, tag := range requiredTags {
		if !strings.Contains(prompt, tag) {
			t.Errorf("expected cognitive prompt to contain tag %s, got:\n%s", tag, prompt)
		}
	}

	if !strings.Contains(prompt, "Lead Architect") {
		t.Errorf("expected prompt to contain agent name 'Lead Architect'")
	}
	if !strings.Contains(prompt, "native_exec") {
		t.Errorf("expected prompt to list authorized tool 'native_exec'")
	}
}

func TestBuildPlannerPrompt(t *testing.T) {
	agents := []AgentManifest{
		{AgentID: "agent_code", Name: "Coder", AuthorizedTools: []string{"native_exec"}},
		{AgentID: "agent_research", Name: "Researcher", AuthorizedTools: []string{"native_web_search"}},
	}

	prompt := BuildPlannerPrompt("Build a full stack web app", agents)

	if !strings.Contains(prompt, "<goal_decomposition_task>") {
		t.Errorf("expected planner prompt to have <goal_decomposition_task>")
	}
	if !strings.Contains(prompt, "agent_code") || !strings.Contains(prompt, "agent_research") {
		t.Errorf("expected planner prompt to list real agents")
	}
	if !strings.Contains(prompt, "<output_schema>") {
		t.Errorf("expected planner prompt to include output schema")
	}
}

func TestBuildReflectionPrompt(t *testing.T) {
	prompt := BuildReflectionPrompt("I prefer tabs instead of spaces", "Understood, I will use tabs.")

	if !strings.Contains(prompt, "<memory_reflection_task>") {
		t.Errorf("expected reflection prompt to contain <memory_reflection_task>")
	}
	if !strings.Contains(prompt, "I prefer tabs instead of spaces") {
		t.Errorf("expected user message in reflection prompt")
	}
}

func TestBuildSwarmDelegationPrompt(t *testing.T) {
	sys, usr := BuildSwarmDelegationPrompt("Parent Lead", "Code Review", "Review PR #42")

	if !strings.Contains(sys, "<delegated_subtask_role>") || !strings.Contains(sys, "Parent Lead") {
		t.Errorf("unexpected swarm system prompt: %s", sys)
	}
	if !strings.Contains(usr, "<subtask_payload>") || !strings.Contains(usr, "Review PR #42") {
		t.Errorf("unexpected swarm user prompt: %s", usr)
	}
}

func TestBuildHeartbeatPrompts(t *testing.T) {
	missionPrompt := BuildHeartbeatMissionPrompt("Audit DB", "Verify SQLite indexes", "Keep latency < 5ms")
	if !strings.Contains(missionPrompt, "<autonomous_mission_cycle>") || !strings.Contains(missionPrompt, "Audit DB") {
		t.Errorf("unexpected heartbeat mission prompt: %s", missionPrompt)
	}

	pulsePrompt := BuildHeartbeatPulsePrompt("Check health", "0 pending tasks")
	if !strings.Contains(pulsePrompt, "<autonomous_heartbeat_pulse>") || !strings.Contains(pulsePrompt, "HEARTBEAT_OK") {
		t.Errorf("unexpected heartbeat pulse prompt: %s", pulsePrompt)
	}
}

func TestCleanJSONPayloadAndExtract(t *testing.T) {
	// Test Case 1: Markdown code fence with trailing text
	rawWithFence := "Here is the result:\n```json\n[\n  {\"id\": \"task_1\", \"description\": \"Do something\"}\n]\n```\nHope this helps!"
	var steps []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}

	if err := ExtractAndUnmarshalJSON(rawWithFence, &steps); err != nil {
		t.Fatalf("failed to extract JSON from code fence: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "task_1" {
		t.Fatalf("unexpected unmarshaled steps: %+v", steps)
	}

	// Test Case 2: Object with reasoning preamble
	rawObject := "Thinking Process: The user wants tabs.\n{\n  \"preference_key\": \"indentation\",\n  \"preference_value\": \"tabs\"\n}"
	var pref struct {
		Key string `json:"preference_key"`
		Val string `json:"preference_value"`
	}

	if err := ExtractAndUnmarshalJSON(rawObject, &pref); err != nil {
		t.Fatalf("failed to extract JSON object: %v", err)
	}
	if pref.Key != "indentation" || pref.Val != "tabs" {
		t.Fatalf("unexpected unmarshaled preference: %+v", pref)
	}
}

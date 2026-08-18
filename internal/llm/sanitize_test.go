package llm

import (
	"encoding/json"
	"testing"
)

func TestSanitizeMessages_OrphanedToolCalls(t *testing.T) {
	// History where assistant made tool_calls but no tool response exists
	raw := []Message{
		{Role: RoleSystem, Content: "System instructions"},
		{Role: RoleUser, Content: "Check weather in Hanoi"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_weather_1", Type: "function", Function: FunctionCall{Name: "get_weather", Arguments: json.RawMessage(`{"city":"Hanoi"}`)}},
			},
		},
		{Role: RoleUser, Content: "What about Da Nang?"},
	}

	sanitized := SanitizeMessages(raw)
	if len(sanitized) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(sanitized))
	}

	// The assistant message at index 2 should have its ToolCalls stripped
	if len(sanitized[2].ToolCalls) != 0 {
		t.Errorf("expected assistant ToolCalls to be stripped, got %d", len(sanitized[2].ToolCalls))
	}
	if sanitized[2].Content == "" {
		t.Errorf("expected assistant Content to not be empty")
	}
}

func TestSanitizeMessages_PartialToolResponses(t *testing.T) {
	// Assistant made 2 tool calls, but only 1 tool response was captured
	raw := []Message{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "fetch_a"}},
				{ID: "call_2", Type: "function", Function: FunctionCall{Name: "fetch_b"}},
			},
		},
		{Role: RoleTool, ToolCallID: "call_1", Content: "Result A"},
	}

	sanitized := SanitizeMessages(raw)
	// Should have assistant message, call_1 response, AND injected call_2 placeholder response
	if len(sanitized) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(sanitized))
	}
	if sanitized[1].ToolCallID != "call_1" {
		t.Errorf("expected index 1 to be call_1, got %s", sanitized[1].ToolCallID)
	}
	if sanitized[2].ToolCallID != "call_2" {
		t.Errorf("expected index 2 to be injected call_2, got %s", sanitized[2].ToolCallID)
	}
}

func TestSanitizeMessages_ValidSequence(t *testing.T) {
	raw := []Message{
		{Role: RoleUser, Content: "Hello"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "search"}},
			},
		},
		{Role: RoleTool, ToolCallID: "call_1", Content: "Search results"},
		{Role: RoleAssistant, Content: "Here is your answer"},
	}

	sanitized := SanitizeMessages(raw)
	if len(sanitized) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(sanitized))
	}
	if len(sanitized[1].ToolCalls) != 1 {
		t.Errorf("expected tool call to be preserved")
	}
}

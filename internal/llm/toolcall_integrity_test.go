package llm

import (
	"encoding/json"
	"testing"
)

// assertToolCallPairing enforces the provider contract: every assistant
// tool_call has exactly one following tool result, and no tool result is
// orphaned. A violation is what produced intermittent tool-call failures.
func assertToolCallPairing(t *testing.T, messages []Message) {
	t.Helper()
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == RoleTool {
			t.Fatalf("orphaned tool result at index %d (id=%q)", i, msg.ToolCallID)
		}
		if msg.Role != RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		expected := map[string]bool{}
		for _, tc := range msg.ToolCalls {
			if tc.ID == "" {
				t.Fatalf("assistant message at index %d has a tool_call with no id", i)
			}
			expected[tc.ID] = true
		}
		j := i + 1
		for j < len(messages) && messages[j].Role == RoleTool {
			id := messages[j].ToolCallID
			if !expected[id] {
				t.Fatalf("tool result at index %d has unexpected or duplicate id %q", j, id)
			}
			delete(expected, id)
			j++
		}
		if len(expected) != 0 {
			t.Fatalf("assistant message at index %d has %d unanswered tool_call(s)", i, len(expected))
		}
		i = j - 1
	}
}

func TestSanitizeDropsToolCallsWithoutID(t *testing.T) {
	// A streamed tool_call fragment that never received an id cannot be paired.
	msgs := []Message{
		{Role: RoleUser, Content: "search"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{Function: FunctionCall{Name: "web_search", Arguments: []byte(`{}`)}},
		}},
		{Role: RoleTool, Content: "result with no owner"},
	}
	assertToolCallPairing(t, SanitizeMessages(msgs))
}

func TestSanitizeDropsDuplicateAndForeignToolResults(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "search"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_a", Type: "function", Function: FunctionCall{Name: "web_search"}},
		}},
		{Role: RoleTool, ToolCallID: "call_a", Content: "ok"},
		{Role: RoleTool, ToolCallID: "call_a", Content: "duplicate"},
		{Role: RoleTool, ToolCallID: "call_ghost", Content: "belongs to nothing"},
	}
	cleaned := SanitizeMessages(msgs)
	assertToolCallPairing(t, cleaned)
	for _, m := range cleaned {
		if m.Content == "duplicate" || m.Content == "belongs to nothing" {
			t.Fatalf("stray tool result survived sanitization: %q", m.Content)
		}
	}
}

func TestSanitizeFillsMissingToolResultsWithName(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "do two things"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_a", Type: "function", Function: FunctionCall{Name: "tool_a"}},
			{ID: "call_b", Type: "function", Function: FunctionCall{Name: "tool_b"}},
		}},
		{Role: RoleTool, ToolCallID: "call_a", Name: "tool_a", Content: "done"},
	}
	cleaned := SanitizeMessages(msgs)
	assertToolCallPairing(t, cleaned)
	for _, m := range cleaned {
		if m.Role == RoleTool && m.ToolCallID == "call_b" && m.Name != "tool_b" {
			t.Fatalf("synthetic tool result must carry Name, got %q", m.Name)
		}
	}
}

func TestSanitizeHistoryToolCallsWithNoResults(t *testing.T) {
	// Replayed history often carries tool_calls whose results were never stored.
	msgs := []Message{
		{Role: RoleUser, Content: "earlier question"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_old", Type: "function", Function: FunctionCall{Name: "web_search"}},
		}},
		{Role: RoleUser, Content: "new question"},
	}
	assertToolCallPairing(t, SanitizeMessages(msgs))
}

func TestToOpenAIMessagesNullsContentForToolCallOnlyTurn(t *testing.T) {
	converted := toOpenAIMessages([]Message{
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_a", Type: "function", Function: FunctionCall{Name: "tool_a"}},
		}},
		{Role: RoleTool, ToolCallID: "call_a", Content: "ok"},
	})
	if converted[0].Content != nil {
		t.Fatal("tool_calls-only assistant content must serialize as null")
	}
	// An empty tool result must still be an empty string, never null.
	if converted[1].Content == nil {
		t.Fatal("tool result content must never be null")
	}
}

func TestToOpenAIMessagesDefaultsEmptyArguments(t *testing.T) {
	converted := toOpenAIMessages([]Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_a", Function: FunctionCall{Name: "tool_a"}},
		}},
	})
	if got := converted[0].ToolCalls[0].Function.Arguments; got != "{}" {
		t.Fatalf("empty arguments must default to {}, got %q", got)
	}
	if got := converted[0].ToolCalls[0].Type; got != "function" {
		t.Fatalf("tool call type must default to \"function\", got %q", got)
	}
}

func TestToOpenAIMessagesOmitsReasoningContent(t *testing.T) {
	raw, err := json.Marshal(toOpenAIMessages([]Message{
		{Role: RoleAssistant, Content: "answer", ReasoningContent: "internal chain of thought"},
	}))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, present := decoded[0]["reasoning_content"]; present {
		t.Fatal("reasoning_content must not be replayed to the provider")
	}
}

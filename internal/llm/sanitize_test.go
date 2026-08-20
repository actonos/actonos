package llm

import (
	"encoding/json"
	"strings"
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

func TestExtractEmbeddedToolCalls_DeepSeekDSML(t *testing.T) {
	raw := `<｜｜DSML｜｜tool_calls>

<｜｜DSML｜｜invoke name="native_file_read">

<｜｜DSML｜｜parameter name="path" string="true">agent_system_core/actonos-architecture-email.html</｜｜DSML｜｜parameter>

</｜｜DSML｜｜invoke>

</｜｜DSML｜｜tool_calls>`

	cleaned, calls := ExtractEmbeddedToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("expected 1 extracted tool call, got %d", len(calls))
	}
	if calls[0].Function.Name != "native_file_read" {
		t.Errorf("expected tool name native_file_read, got %q", calls[0].Function.Name)
	}

	var args map[string]any
	if err := json.Unmarshal(calls[0].Function.Arguments, &args); err != nil {
		t.Fatalf("unmarshalling extracted args failed: %v", err)
	}
	if args["path"] != "agent_system_core/actonos-architecture-email.html" {
		t.Errorf("expected path parameter, got %v", args["path"])
	}
	if cleaned != "" {
		t.Errorf("expected empty cleaned content, got %q", cleaned)
	}
}

func TestExtractEmbeddedToolCalls_MixedTextAndXML(t *testing.T) {
	raw := `Let me read the file for you.
<invoke name="native_file_read">
<path>notes.txt</path>
</invoke>
I will analyze it once read.`

	cleaned, calls := ExtractEmbeddedToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("expected 1 extracted tool call, got %d", len(calls))
	}
	if calls[0].Function.Name != "native_file_read" {
		t.Errorf("expected tool name native_file_read, got %q", calls[0].Function.Name)
	}
	if !strings.Contains(cleaned, "Let me read the file for you.") || strings.Contains(cleaned, "<invoke") {
		t.Errorf("unexpected cleaned content: %q", cleaned)
	}
}

func TestExtractThinkingContent_DeepSeekAndClaude(t *testing.T) {
	// DeepSeek-R1 <think> tag
	dsRaw := `<think>
I need to inspect the directory structure first.
Let's plan to call native_file_list.
</think>Here is the directory report.`
	dsClean, dsReasoning := ExtractThinkingContent(dsRaw, "")
	if dsClean != "Here is the directory report." {
		t.Errorf("expected clean text, got %q", dsClean)
	}
	if !strings.Contains(dsReasoning, "I need to inspect the directory structure first.") {
		t.Errorf("expected extracted reasoning, got %q", dsReasoning)
	}

	// Claude <thinking> tag
	claudeRaw := `<thinking>Analyzing user input for security parameters.</thinking>Processed successfully.`
	claudeClean, claudeReasoning := ExtractThinkingContent(claudeRaw, "Prior thoughts.")
	if claudeClean != "Processed successfully." {
		t.Errorf("expected clean text, got %q", claudeClean)
	}
	if !strings.Contains(claudeReasoning, "Prior thoughts.") || !strings.Contains(claudeReasoning, "Analyzing user input") {
		t.Errorf("expected combined reasoning, got %q", claudeReasoning)
	}
}

func TestExtractEmbeddedToolCalls_DeepSeekNativeTokens(t *testing.T) {
	raw := `<｜tool calls｜><｜tool call begin｜>function:native_file_write{"path":"hello.txt","content":"world"}<｜tool call end｜><｜tool calls end｜>`
	cleaned, calls := ExtractEmbeddedToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "native_file_write" {
		t.Errorf("expected native_file_write, got %q", calls[0].Function.Name)
	}
	if cleaned != "" {
		t.Errorf("expected empty cleaned content, got %q", cleaned)
	}
}

func TestExtractEmbeddedToolCalls_AnthropicFunctionTag(t *testing.T) {
	raw := `<function=native_file_read>{"path": "config.json"}</function>`
	cleaned, calls := ExtractEmbeddedToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "native_file_read" {
		t.Errorf("expected native_file_read, got %q", calls[0].Function.Name)
	}
	if cleaned != "" {
		t.Errorf("expected empty cleaned, got %q", cleaned)
	}
}

func TestExtractEmbeddedToolCalls_MarkdownBlock(t *testing.T) {
	raw := "I will create the file now.\n```tool_call\n{\"name\": \"native_file_write\", \"arguments\": {\"path\": \"index.js\", \"content\": \"console.log(1)\"}}\n```\nDone."
	cleaned, calls := ExtractEmbeddedToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "native_file_write" {
		t.Errorf("expected native_file_write, got %q", calls[0].Function.Name)
	}
	if strings.Contains(cleaned, "```") || !strings.Contains(cleaned, "I will create the file now.") {
		t.Errorf("unexpected cleaned content: %q", cleaned)
	}
}

func TestExtractEmbeddedToolCalls_UserReportedDeepSeekSnippet(t *testing.T) {
	raw := `Tôi sẽ xem file email kiến trúc ActonOS đã có sẵn để nắm thông tin sản phẩm, sau đó tạo email marketing mới hoàn chỉnh trong workspace.

<｜｜DSML｜｜tool_calls>

<｜｜DSML｜｜invoke name="WebSearch">

<｜｜DSML｜｜parameter name="query" string="true">ActonOS</｜｜DSML｜｜parameter>

</｜｜DSML｜｜invoke>

<｜｜DSML｜｜invoke name="ReadFile">

<｜｜DSML｜｜parameter name="file" string="true">data/workspace/agent_system_core/actonos-architecture-email.html</｜｜DSML｜｜parameter>

</｜｜DSML｜｜invoke>

</｜｜DSML｜｜tool_calls>`

	cleaned, calls := ExtractEmbeddedToolCalls(raw)
	if len(calls) != 2 {
		t.Fatalf("expected 2 extracted calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "native_web_search" {
		t.Errorf("expected native_web_search, got %q", calls[0].Function.Name)
	}
	if calls[1].Function.Name != "native_file_read" {
		t.Errorf("expected native_file_read, got %q", calls[1].Function.Name)
	}
	if strings.Contains(cleaned, "<｜｜DSML") || strings.Contains(cleaned, "invoke") {
		t.Errorf("cleaned content still contains DSML tags: %q", cleaned)
	}
	if !strings.Contains(cleaned, "Tôi sẽ xem file email kiến trúc ActonOS") {
		t.Errorf("cleaned content missing original prose: %q", cleaned)
	}
}

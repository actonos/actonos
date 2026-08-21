package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestModelCascadeRouter_PrimarySuccess(t *testing.T) {
	router := NewModelCascadeRouter()

	mockPrimary := NewMockProvider("claude-3-7-sonnet", "Primary response")
	mockFallback := NewMockProvider("gemini-2.5-flash", "Fallback response")

	router.RegisterProvider("claude", mockPrimary)
	router.RegisterProvider("gemini", mockFallback)

	ctx := context.Background()
	resp, err := router.CompleteWithCascade(ctx, []string{"claude", "gemini"}, []Message{
		{Role: RoleUser, Content: "Hello"},
	}, CompletionOptions{})

	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if resp.Content != "Primary response" {
		t.Fatalf("expected 'Primary response', got '%s'", resp.Content)
	}

	if mockPrimary.CompleteCalls != 1 {
		t.Fatalf("expected primary to be called 1 time, got %d", mockPrimary.CompleteCalls)
	}
	if mockFallback.CompleteCalls != 0 {
		t.Fatalf("expected fallback to be called 0 times, got %d", mockFallback.CompleteCalls)
	}
}

func TestModelCascadeRouter_FallbackTriggered(t *testing.T) {
	router := NewModelCascadeRouter()

	mockPrimary := NewMockProvider("claude-3-7-sonnet", "")
	mockPrimary.CompleteFunc = func(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
		return nil, errors.New("429 Too Many Requests (Rate Limit Exceeded)")
	}

	mockFallback := NewMockProvider("gemini-2.5-flash", "Fallback response")

	router.RegisterProvider("claude", mockPrimary)
	router.RegisterProvider("gemini", mockFallback)

	ctx := context.Background()
	resp, err := router.CompleteWithCascade(ctx, []string{"claude", "gemini"}, []Message{
		{Role: RoleUser, Content: "Hello"},
	}, CompletionOptions{})

	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}

	if resp.Content != "Fallback response" {
		t.Fatalf("expected 'Fallback response', got '%s'", resp.Content)
	}

	if mockPrimary.CompleteCalls != 1 {
		t.Fatalf("expected primary to be called 1 time, got %d", mockPrimary.CompleteCalls)
	}
	if mockFallback.CompleteCalls != 1 {
		t.Fatalf("expected fallback to be called 1 time, got %d", mockFallback.CompleteCalls)
	}
}

func TestModelCascadeRouter_Embed(t *testing.T) {
	router := NewModelCascadeRouter()
	mockProvider := NewMockProvider("mock-embed", "test")
	router.RegisterProvider("mock", mockProvider)

	ctx := context.Background()
	embeddings, err := router.Embed(ctx, "mock", []string{"ActonOS", "Agent Engine"})
	if err != nil {
		t.Fatalf("expected embed success, got error: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embedding vectors, got %d", len(embeddings))
	}
	if len(embeddings[0]) != 64 {
		t.Fatalf("expected 64 dimensions, got %d", len(embeddings[0]))
	}
}

func TestToOpenAIMessages_ToolArgumentsSerializedAsString(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "Find hot tech news"},
		{
			Role:    RoleAssistant,
			Content: "",
			ToolCalls: []ToolCall{
				{
					ID:   "call_search_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "web_search",
						Arguments: []byte(`{"query":"hot tech news 2026"}`),
					},
				},
			},
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_search_1",
			Content:    "Found 5 articles",
		},
	}

	converted := toOpenAIMessages(msgs)
	if len(converted) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(converted))
	}

	asst := converted[1]
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(asst.ToolCalls))
	}

	if asst.ToolCalls[0].Function.Arguments != `{"query":"hot tech news 2026"}` {
		t.Fatalf("expected raw JSON string arguments, got %s", asst.ToolCalls[0].Function.Arguments)
	}

	req := openAIChatRequest{
		Model:    "deepseek-chat",
		Messages: converted,
	}
	rawJSON, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify that arguments in the JSON output is a string ("arguments":"{...}") and NOT a raw object ("arguments":{...})
	var rawMap map[string]any
	if err := json.Unmarshal(rawJSON, &rawMap); err != nil {
		t.Fatalf("json.Unmarshal into generic map failed: %v", err)
	}

	messagesList, ok := rawMap["messages"].([]any)
	if !ok || len(messagesList) < 2 {
		t.Fatalf("unexpected messages payload in json: %v", rawMap["messages"])
	}

	asstMap, ok := messagesList[1].(map[string]any)
	if !ok {
		t.Fatalf("messages[1] is not a map: %v", messagesList[1])
	}

	tcList, ok := asstMap["tool_calls"].([]any)
	if !ok || len(tcList) == 0 {
		t.Fatalf("tool_calls missing in messages[1]: %v", asstMap)
	}

	tc0, ok := tcList[0].(map[string]any)
	if !ok {
		t.Fatalf("tool_calls[0] is not a map: %v", tcList[0])
	}

	fnMap, ok := tc0["function"].(map[string]any)
	if !ok {
		t.Fatalf("function is not a map: %v", tc0["function"])
	}

	argsVal, ok := fnMap["arguments"].(string)
	if !ok {
		t.Fatalf("expected function.arguments to be a string in JSON, but got %T: %v", fnMap["arguments"], fnMap["arguments"])
	}

	if argsVal != `{"query":"hot tech news 2026"}` {
		t.Fatalf("expected string value `{\"query\":\"hot tech news 2026\"}`, got: %s", argsVal)
	}

	// reasoning_content is an OUTPUT-only field. DeepSeek/R1 reject a request that
	// echoes it back in an input message, so it must never be serialized here.
	if _, hasRC := asstMap["reasoning_content"]; hasRC {
		t.Fatal("reasoning_content must not be sent back to the provider in a request message")
	}

	// An assistant turn carrying only tool_calls must send content as JSON null.
	// An empty string paired with tool_calls is rejected by several
	// OpenAI-compatible gateways.
	contentVal, hasContent := asstMap["content"]
	if !hasContent {
		t.Fatal("expected content key to be present in assistant message JSON")
	}
	if contentVal != nil {
		t.Fatalf("expected content to be null for a tool_calls-only assistant message, got %T: %v", contentVal, contentVal)
	}
}

func TestIsOpenAIEndpoint(t *testing.T) {
	if !isOpenAIEndpoint("https://api.openai.com/v1") {
		t.Error("expected true for api.openai.com")
	}
	if !isOpenAIEndpoint("") {
		t.Error("expected true for empty baseURL")
	}
	if isOpenAIEndpoint("http://localhost:11434/v1") {
		t.Error("expected false for localhost ollama")
	}
	if isOpenAIEndpoint("https://api.deepseek.com/v1") {
		t.Error("expected false for deepseek")
	}
}

func TestOpenAIProviderProtocolSelection(t *testing.T) {
	native := NewOpenAIProvider("", "gpt-5", "https://api.openai.com/v1")
	if !native.UseResponsesAPI {
		t.Fatal("native OpenAI provider must use Responses API")
	}
	for _, baseURL := range []string{
		"https://api.deepseek.com/v1",
		"https://openrouter.ai/api/v1",
		"http://localhost:8000/v1",
	} {
		compatible := NewOpenAIProvider("", "model", baseURL)
		if compatible.UseResponsesAPI {
			t.Fatalf("compatible endpoint %q must keep Chat Completions", baseURL)
		}
	}
}

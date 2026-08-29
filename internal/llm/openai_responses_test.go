package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIResponsesCompleteUsesResponsesContract(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5","output_text":"ready","output":[{"type":"reasoning","id":"rs_1","summary":[]},{"type":"function_call","call_id":"call-1","name":"lookup","arguments":"{\"q\":\"x\"}"}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider("key", "gpt-5", server.URL+"/v1")
	provider.UseResponsesAPI = true
	resp, err := provider.Complete(context.Background(), []Message{
		{Role: RoleSystem, Content: "Be concise."},
		{Role: RoleUser, Content: "Find x"},
	}, CompletionOptions{Tools: []ToolDefinition{{Type: "function", Function: FunctionDefinition{Name: "lookup", Description: "Find data", Parameters: json.RawMessage(`{"type":"object"}`)}}}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("expected Responses path, got %q", gotPath)
	}
	if _, ok := gotBody["messages"]; ok {
		t.Fatal("native OpenAI request must not contain Chat Completions messages")
	}
	if gotBody["instructions"] != "Be concise." {
		t.Fatalf("unexpected instructions: %#v", gotBody["instructions"])
	}
	if _, ok := gotBody["temperature"]; ok {
		t.Fatal("native Responses request must not contain sampling temperature")
	}
	reasoning, ok := gotBody["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != DefaultReasoningEffort {
		t.Fatalf("expected default reasoning effort %q, got %#v", DefaultReasoningEffort, gotBody["reasoning"])
	}
	if resp.Content != "ready" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call-1" || resp.Usage.TotalTokens != 18 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.ProviderItems) != 1 {
		t.Fatalf("reasoning output item was not retained: %+v", resp.ProviderItems)
	}
}

func TestOpenAIResponsesCompleteContinuesToolOutput(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5","output_text":"done","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
	}))
	defer server.Close()
	provider := NewOpenAIProvider("", "gpt-5", server.URL+"/v1")
	provider.UseResponsesAPI = true
	_, err := provider.Complete(context.Background(), []Message{
		{Role: RoleAssistant, ProviderItems: []json.RawMessage{json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[]}`)}, ToolCalls: []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: json.RawMessage(`{"q":"x"}`)}}}},
		{Role: RoleTool, ToolCallID: "call-1", Content: "result"},
	}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := gotBody["input"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("unexpected input items: %#v", gotBody["input"])
	}
	call := items[0].(map[string]any)
	functionCall := items[1].(map[string]any)
	output := items[2].(map[string]any)
	if call["type"] != "reasoning" || functionCall["type"] != "function_call" || output["type"] != "function_call_output" || output["call_id"] != "call-1" || output["output"] != "result" {
		t.Fatalf("tool continuation lost correlation: %#v", items)
	}
}

func TestOpenAIResponsesCompleteEmptyToolOutputRetainsOutputField(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5","output_text":"done","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
	}))
	defer server.Close()
	provider := NewOpenAIProvider("", "gpt-5", server.URL+"/v1")
	provider.UseResponsesAPI = true
	_, err := provider.Complete(context.Background(), []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call-empty", Type: "function", Function: FunctionCall{Name: "exec", Arguments: json.RawMessage(`{}`)}}}},
		{Role: RoleTool, ToolCallID: "call-empty", Content: ""},
	}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := gotBody["input"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected input items: %#v", gotBody["input"])
	}
	output := items[1].(map[string]any)
	if output["type"] != "function_call_output" || output["call_id"] != "call-empty" {
		t.Fatalf("unexpected output item: %#v", output)
	}
	val, exists := output["output"]
	if !exists {
		t.Fatal("expected 'output' field to be present even when empty string, but was missing!")
	}
	if val != "" {
		t.Fatalf("expected empty string output, got %#v", val)
	}
}

func TestOpenAIResponsesStreamReassemblesTextAndToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"response.output_text.delta","delta":"hel"}`,
			`{"type":"response.output_text.delta","delta":"lo"}`,
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call-2","name":"search"}}`,
			`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"q\":"}`,
			`{"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"q\":\"go\"}"}`,
			`{"type":"response.completed","response":{"model":"gpt-5","usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}}`,
		}
		for _, event := range events {
			_, _ = w.Write([]byte("data: " + event + "\n\n"))
		}
	}))
	defer server.Close()
	provider := NewOpenAIProvider("", "gpt-5", server.URL+"/v1")
	provider.UseResponsesAPI = true
	chunks, err := provider.StreamComplete(context.Background(), []Message{{Role: RoleUser, Content: "search"}}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, calls, usage := collectStream(t, chunks)
	if text != "hello" || len(calls) != 1 || calls[0].ID != "call-2" || string(calls[0].Function.Arguments) != `{"q":"go"}` {
		t.Fatalf("unexpected Responses stream: text=%q calls=%+v", text, calls)
	}
	if usage == nil || usage.TotalTokens != 10 {
		t.Fatalf("unexpected stream usage: %+v", usage)
	}
}

func TestOpenAICompatibleEndpointStillUsesChatCompletions(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"compatible","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	provider := NewOpenAIProvider("", "compatible", server.URL+"/v1")
	if _, err := provider.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, CompletionOptions{}); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" || body["messages"] == nil {
		t.Fatalf("compatible endpoint changed protocol: path=%q body=%s", path, strings.TrimSpace(mustJSON(body)))
	}
	if _, ok := body["temperature"]; ok {
		t.Fatal("Chat Completions request must not contain sampling temperature")
	}
}

func TestOpenAIResponsesExplicitReasoningEffort(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5","output_text":"ok"}`))
	}))
	defer server.Close()
	provider := NewOpenAIProvider("", "gpt-5", server.URL+"/v1")
	provider.UseResponsesAPI = true
	if _, err := provider.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, CompletionOptions{ReasoningEffort: "high"}); err != nil {
		t.Fatal(err)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("explicit reasoning effort was not forwarded: %#v", body["reasoning"])
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

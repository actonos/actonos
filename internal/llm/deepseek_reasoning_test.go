package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureServer records the JSON bodies of chat completion requests and replies
// with a minimal valid completion.
func captureServer(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func toolTurn() []Message {
	return []Message{
		{Role: RoleUser, Content: "write the file"},
		{
			Role:             RoleAssistant,
			ReasoningContent: "I should call the write tool.",
			ToolCalls: []ToolCall{{
				ID: "call_1", Type: "function",
				Function: FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"a.txt","content":"x"}`)},
			}},
		},
		{Role: RoleTool, ToolCallID: "call_1", Name: "native_file_write", Content: "written"},
	}
}

func assistantMessage(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	messages, _ := body["messages"].([]any)
	for _, m := range messages {
		msg, _ := m.(map[string]any)
		if msg["role"] == "assistant" {
			return msg
		}
	}
	t.Fatalf("no assistant message in request: %v", body)
	return nil
}

// DeepSeek thinking mode rejects a follow-up request (HTTP 400, "The
// reasoning_content in the thinking mode must be passed back to the API") when the
// assistant turn that issued tool calls is replayed without its reasoning.
func TestDeepSeekReplaysReasoningForToolCallTurns(t *testing.T) {
	srv, bodies := captureServer(t)
	provider := NewDeepSeekProvider("test-key", "deepseek-v4-flash")
	provider.BaseURL = srv.URL

	if _, err := provider.Complete(context.Background(), toolTurn(), CompletionOptions{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	assistant := assistantMessage(t, (*bodies)[0])
	if assistant["reasoning_content"] != "I should call the write tool." {
		t.Fatalf("reasoning_content must be sent back with the tool-call turn, got %v", assistant)
	}
}

func TestDeepSeekDoesNotReplayReasoningForPlainAnswers(t *testing.T) {
	srv, bodies := captureServer(t)
	provider := NewDeepSeekProvider("test-key", "deepseek-v4-flash")
	provider.BaseURL = srv.URL

	history := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: "hello", ReasoningContent: "greet the user"},
		{Role: RoleUser, Content: "thanks"},
	}
	if _, err := provider.Complete(context.Background(), history, CompletionOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, present := assistantMessage(t, (*bodies)[0])["reasoning_content"]; present {
		t.Fatal("reasoning from a finished turn must not be replayed")
	}
}

func TestGenericOpenAICompatibleNeverReplaysReasoning(t *testing.T) {
	srv, bodies := captureServer(t)
	provider := NewOpenAIProvider("test-key", "some-model", srv.URL)

	if _, err := provider.Complete(context.Background(), toolTurn(), CompletionOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, present := assistantMessage(t, (*bodies)[0])["reasoning_content"]; present {
		t.Fatal("generic OpenAI-compatible endpoints reject reasoning_content in input messages")
	}
}

func TestDeepSeekToolTurnWithoutReasoningOmitsTheField(t *testing.T) {
	srv, bodies := captureServer(t)
	provider := NewDeepSeekProvider("test-key", "deepseek-chat")
	provider.BaseURL = srv.URL

	turn := toolTurn()
	turn[1].ReasoningContent = ""
	if _, err := provider.Complete(context.Background(), turn, CompletionOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, present := assistantMessage(t, (*bodies)[0])["reasoning_content"]; present {
		t.Fatal("an empty reasoning_content must not be sent")
	}
}

package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collectStream(t *testing.T, chunks <-chan StreamChunk) (string, []ToolCall, *Usage) {
	t.Helper()
	var text strings.Builder
	var calls []ToolCall
	var usage *Usage
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Error)
		}
		text.WriteString(chunk.DeltaContent)
		calls = append(calls, chunk.ToolCalls...)
		if chunk.Usage != nil {
			copy := *chunk.Usage
			usage = &copy
		}
	}
	return text.String(), calls, usage
}

func TestOpenAIStreamReassemblesFragmentedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"working \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"pa\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"th\\\":\\\"README.md\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider("", "test", server.URL)
	chunks, err := provider.StreamComplete(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, calls, _ := collectStream(t, chunks)
	if text != "working " || len(calls) != 1 {
		t.Fatalf("unexpected stream result: text=%q calls=%+v", text, calls)
	}
	if calls[0].Function.Name != "read_file" || string(calls[0].Function.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("fragmented tool call was not reconstructed: %+v", calls[0])
	}
}

func TestAnthropicStreamReassemblesToolUseAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"message_start","message":{"model":"test","usage":{"input_tokens":7}}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool-1","name":"shell"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"pwd\"}"}}`,
			`{"type":"message_delta","usage":{"output_tokens":3}}`,
			`{"type":"message_stop"}`,
		}
		for _, event := range events {
			_, _ = w.Write([]byte("data: " + event + "\n\n"))
		}
	}))
	defer server.Close()

	provider := NewAnthropicProvider("", "test")
	provider.BaseURL = server.URL
	chunks, err := provider.StreamComplete(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, calls, usage := collectStream(t, chunks)
	if text != "done" || len(calls) != 1 || string(calls[0].Function.Arguments) != `{"command":"pwd"}` {
		t.Fatalf("unexpected Anthropic stream result: text=%q calls=%+v", text, calls)
	}
	if usage == nil || usage.PromptTokens != 7 || usage.CompletionTokens != 3 || usage.TotalTokens != 10 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestGeminiStreamEmitsTextAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello \"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"world\"}]}}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3,\"totalTokenCount\":5}}\n\n"))
	}))
	defer server.Close()

	provider := NewGeminiProvider("", "test")
	provider.BaseURL = server.URL
	chunks, err := provider.StreamComplete(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, calls, usage := collectStream(t, chunks)
	if text != "hello world" || len(calls) != 0 {
		t.Fatalf("unexpected Gemini stream result: text=%q calls=%+v", text, calls)
	}
	if usage == nil || usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

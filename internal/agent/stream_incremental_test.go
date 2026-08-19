package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/actonos/actonos/internal/llm"
)

// scriptedStreamProvider emits a fixed sequence of chunks and records whether the
// consumer read them one at a time rather than draining the whole stream first.
type scriptedStreamProvider struct {
	chunks []llm.StreamChunk
}

func (p *scriptedStreamProvider) ModelName() string { return "scripted-model" }

func (p *scriptedStreamProvider) Complete(
	_ context.Context, _ []llm.Message, _ llm.CompletionOptions,
) (*llm.Response, error) {
	return &llm.Response{Content: "unused"}, nil
}

func (p *scriptedStreamProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (p *scriptedStreamProvider) StreamComplete(
	_ context.Context, _ []llm.Message, _ llm.CompletionOptions,
) (<-chan llm.StreamChunk, error) {
	// Unbuffered: a send only completes once the engine has received the previous
	// chunk, so ordering assertions below reflect true incremental consumption.
	ch := make(chan llm.StreamChunk)
	go func() {
		defer close(ch)
		for _, chunk := range p.chunks {
			ch <- chunk
		}
	}()
	return ch, nil
}

func newScriptedEngine(chunks []llm.StreamChunk) *Engine {
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("scripted", &scriptedStreamProvider{chunks: chunks})
	return &Engine{llm: router, verifier: NewVerifier()}
}

// collectStreamEvents drains completeStreamIteration on a background goroutine so
// the unbuffered event channel is consumed as events are produced.
func collectStreamEvents(t *testing.T, e *Engine) ([]AgentStreamEvent, *llm.Response) {
	t.Helper()
	events := make(chan AgentStreamEvent)
	var collected []AgentStreamEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range events {
			collected = append(collected, ev)
		}
	}()
	resp, err := e.completeStreamIteration(
		context.Background(), []string{"scripted"}, nil, llm.CompletionOptions{}, events,
	)
	close(events)
	<-done
	if err != nil {
		t.Fatalf("completeStreamIteration failed: %v", err)
	}
	return collected, resp
}

func TestStreamEmitsTokensIncrementally(t *testing.T) {
	e := newScriptedEngine([]llm.StreamChunk{
		{DeltaContent: "Xin "},
		{DeltaContent: "chào "},
		{DeltaContent: "bạn"},
		{Done: true},
	})

	events, resp := collectStreamEvents(t, e)

	var tokens []string
	for _, ev := range events {
		if ev.Type == EventStreamToken {
			tokens = append(tokens, ev.Content)
		}
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 token events, got %d (%v)", len(tokens), tokens)
	}
	// Each delta must arrive as its own event, preserving exact whitespace.
	for i, want := range []string{"Xin ", "chào ", "bạn"} {
		if tokens[i] != want {
			t.Fatalf("token %d: expected %q, got %q", i, want, tokens[i])
		}
	}
	if resp.Content != "Xin chào bạn" {
		t.Fatalf("accumulated content mismatch: %q", resp.Content)
	}
}

func TestStreamRetractsPreambleOnToolCall(t *testing.T) {
	e := newScriptedEngine([]llm.StreamChunk{
		{DeltaContent: "Let me search"},
		{ToolCalls: []llm.ToolCall{{
			ID: "call_a", Type: "function",
			Function: llm.FunctionCall{Name: "native_web_search", Arguments: json.RawMessage(`{}`)},
		}}},
		{Done: true},
	})

	events, resp := collectStreamEvents(t, e)

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected the tool call to survive, got %d", len(resp.ToolCalls))
	}

	var sawToken, sawReset bool
	var resetIndex, tokenIndex int
	for i, ev := range events {
		switch ev.Type {
		case EventStreamToken:
			sawToken, tokenIndex = true, i
		case EventStreamTokenReset:
			sawReset, resetIndex = true, i
		}
	}
	if !sawToken {
		t.Fatal("preamble should still stream live; the client retracts it on reset")
	}
	if !sawReset {
		t.Fatal("a tool-calling turn must emit token_reset so preamble is discarded")
	}
	if resetIndex < tokenIndex {
		t.Fatal("token_reset must come after the tokens it retracts")
	}
}

func TestStreamNoResetOnPlainAnswer(t *testing.T) {
	e := newScriptedEngine([]llm.StreamChunk{
		{DeltaContent: "Final answer"},
		{Done: true},
	})

	events, _ := collectStreamEvents(t, e)
	for _, ev := range events {
		if ev.Type == EventStreamTokenReset {
			t.Fatal("a turn without tool calls must never retract its tokens")
		}
	}
}

func TestStreamReasoningEmittedAsThought(t *testing.T) {
	e := newScriptedEngine([]llm.StreamChunk{
		{DeltaReasoning: "thinking..."},
		{DeltaContent: "answer"},
		{Done: true},
	})

	events, resp := collectStreamEvents(t, e)
	if resp.ReasoningContent != "thinking..." {
		t.Fatalf("reasoning not accumulated: %q", resp.ReasoningContent)
	}
	var thoughts int
	for _, ev := range events {
		if ev.Type == EventStreamThought {
			thoughts++
		}
	}
	if thoughts == 0 {
		t.Fatal("reasoning deltas must surface as thought events")
	}
}

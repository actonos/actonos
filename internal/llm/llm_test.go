package llm

import (
	"context"
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

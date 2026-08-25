package llm

import (
	"context"
	"errors"
	"testing"
)

func TestCascadeRetriesThenBreakerThenLastResort(t *testing.T) {
	router := NewModelCascadeRouter()
	failing := NewMockProvider("primary", "")
	calls := 0
	failing.CompleteFunc = func(context.Context, []Message, CompletionOptions) (*Response, error) {
		calls++
		return nil, errors.New("429 overloaded")
	}
	local := NewMockProvider("ollama", "local-ok")
	router.RegisterProvider("primary", failing)
	router.RegisterProvider("ollama", local)

	resp, err := router.CompleteWithCascade(context.Background(), []string{"primary"}, []Message{{Role: RoleUser, Content: "hi"}}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "local-ok" {
		t.Fatalf("expected last-resort ollama, got %+v", resp)
	}
	if calls < 1 {
		t.Fatalf("expected primary to be attempted, got %d calls", calls)
	}

	// Trip the breaker with more failures, then a cascade that skips primary.
	for i := 0; i < circuitBreakerThreshold; i++ {
		_, _ = router.CompleteWithCascade(context.Background(), []string{"primary"}, []Message{{Role: RoleUser, Content: "hi"}}, CompletionOptions{})
	}
	before := calls
	_, err = router.CompleteWithCascade(context.Background(), []string{"primary"}, []Message{{Role: RoleUser, Content: "hi"}}, CompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if calls-before >= cascadeRetriesPerProvider {
		t.Fatalf("circuit breaker should skip retried primary calls, extra=%d", calls-before)
	}
}

func TestCascadeNeverUsesLocalStub(t *testing.T) {
	router := NewModelCascadeRouter()
	stub := NewMockProvider("local-stub", "stub-ok")
	router.RegisterProvider("local-stub", stub)
	_, err := router.CompleteWithCascade(context.Background(), []string{"openai/gpt-4o"}, []Message{{Role: RoleUser, Content: "hi"}}, CompletionOptions{})
	if err == nil {
		t.Fatal("local-stub must not be treated as a healthy worker for a real model id")
	}
	if _, getErr := router.GetProvider("openai/gpt-4o"); getErr == nil {
		t.Fatal("GetProvider must not return local-stub for openai/gpt-4o")
	}
}

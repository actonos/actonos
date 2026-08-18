package llm

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// MockLLMProvider is a mock implementation of LLMProvider for unit and integration testing.
type MockLLMProvider struct {
	mu sync.Mutex

	Model              string
	CompleteFunc       func(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error)
	StreamCompleteFunc func(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error)
	EmbedFunc          func(ctx context.Context, texts []string) ([][]float32, error)

	CompleteCalls       int
	StreamCompleteCalls int
	EmbedCalls          int
}

// NewMockProvider creates a MockLLMProvider with default canned responses.
func NewMockProvider(model string, defaultResponse string) *MockLLMProvider {
	return &MockLLMProvider{
		Model: model,
		CompleteFunc: func(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
			return &Response{
				Content: defaultResponse,
				Model:   model,
				Usage: Usage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			}, nil
		},
		EmbedFunc: func(ctx context.Context, texts []string) ([][]float32, error) {
			results := make([][]float32, len(texts))
			for i, txt := range texts {
				// Deterministic pseudo-embedding based on string length & hash
				dim := 64
				vec := make([]float32, dim)
				var norm float64
				for j := 0; j < dim; j++ {
					val := float32(len(txt)*(j+1)) / 100.0
					vec[j] = val
					norm += float64(val * val)
				}
				// Normalize to unit vector
				norm = math.Sqrt(norm)
				if norm > 0 {
					for j := 0; j < dim; j++ {
						vec[j] = float32(float64(vec[j]) / norm)
					}
				}
				results[i] = vec
			}
			return results, nil
		},
	}
}

func (m *MockLLMProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CompleteCalls++
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, messages, opts)
	}
	return nil, fmt.Errorf("mock CompleteFunc not set")
}

func (m *MockLLMProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StreamCompleteCalls++
	if m.StreamCompleteFunc != nil {
		return m.StreamCompleteFunc(ctx, messages, opts)
	}

	ch := make(chan StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- StreamChunk{DeltaContent: "mock stream output"}
		ch <- StreamChunk{Done: true, Usage: &Usage{PromptTokens: 5, CompletionTokens: 5, TotalTokens: 10}}
	}()
	return ch, nil
}

func (m *MockLLMProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EmbedCalls++
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, texts)
	}
	return nil, fmt.Errorf("mock EmbedFunc not set")
}

func (m *MockLLMProvider) ModelName() string {
	if m.Model != "" {
		return m.Model
	}
	return "mock-model"
}

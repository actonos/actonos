package llm

import (
	"context"
)

// LLMProvider is the core interface for interacting with Large Language Models.
type LLMProvider interface {
	// Complete executes a non-streaming chat completion request.
	Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error)

	// StreamComplete executes a streaming chat completion request, returning a channel of chunks.
	StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error)

	// Embed generates dense vector embeddings for input texts.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// ModelName returns the default model identifier for this provider.
	ModelName() string
}

// Embedder is an interface for generating text embeddings (can be standalone or implemented by LLMProvider).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

package llm

import (
	"context"
	"net"
	"net/http"
	"time"
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

// NewDefaultHTTPClient returns a configured *http.Client optimized for streaming LLM calls.
func NewDefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 90 * time.Second,
		},
	}
}

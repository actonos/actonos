package llm

import (
	"context"
)

// OllamaProvider wraps OpenAIProvider configured for a local Ollama server.
type OllamaProvider struct {
	*OpenAIProvider
}

// NewOllamaProvider creates a new provider instance for local Ollama running on localhost:11434.
func NewOllamaProvider(model, baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	if model == "" {
		model = "llama3"
	}
	return &OllamaProvider{
		OpenAIProvider: NewOpenAIProvider("ollama", model, baseURL),
	}
}

func (p *OllamaProvider) ModelName() string {
	return p.Model
}

func (p *OllamaProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	return p.OpenAIProvider.Complete(ctx, messages, opts)
}

func (p *OllamaProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	return p.OpenAIProvider.StreamComplete(ctx, messages, opts)
}

func (p *OllamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.OpenAIProvider.Embed(ctx, texts)
}

package llm

import (
	"context"
)

// DeepSeekProvider wraps OpenAIProvider configured for DeepSeek API.
type DeepSeekProvider struct {
	*OpenAIProvider
}

// NewDeepSeekProvider creates a new provider instance for DeepSeek (e.g. deepseek-chat, deepseek-reasoner).
func NewDeepSeekProvider(apiKey, model string) *DeepSeekProvider {
	if model == "" {
		model = "deepseek-chat"
	}
	base := NewOpenAIProvider(apiKey, model, "https://api.deepseek.com/v1")
	base.ReplayReasoning = true
	return &DeepSeekProvider{OpenAIProvider: base}
}

func (p *DeepSeekProvider) ModelName() string {
	return p.Model
}

func (p *DeepSeekProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	return p.OpenAIProvider.Complete(ctx, messages, opts)
}

func (p *DeepSeekProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	return p.OpenAIProvider.StreamComplete(ctx, messages, opts)
}

func (p *DeepSeekProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.OpenAIProvider.Embed(ctx, texts)
}

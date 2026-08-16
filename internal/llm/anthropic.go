package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider implements LLMProvider for Anthropic's Claude API.
type AnthropicProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-3-7-sonnet-20250219"
	}
	return &AnthropicProvider{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}

	maxTokens := 4096
	if opts.MaxTokens != nil && *opts.MaxTokens > 0 {
		maxTokens = *opts.MaxTokens
	}

	var systemPrompt string
	var anthropicMsgs []anthropicMessage

	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemPrompt = msg.Content
		} else {
			role := "user"
			if msg.Role == RoleAssistant {
				role = "assistant"
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	reqBody := anthropicRequest{
		Model:       model,
		Messages:    anthropicMsgs,
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: opts.Temperature,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/messages", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(bodyBytes, &anthropicResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if anthropicResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", anthropicResp.Error.Message)
	}

	var content string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &Response{
		Content: content,
		Model:   anthropicResp.Model,
		Usage: Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		resp, err := p.Complete(ctx, messages, opts)
		if err != nil {
			ch <- StreamChunk{Error: err}
			return
		}
		ch <- StreamChunk{DeltaContent: resp.Content, Done: false}
		ch <- StreamChunk{Done: true, Usage: &resp.Usage}
	}()
	return ch, nil
}

func (p *AnthropicProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("anthropic does not provide an embedding endpoint; configure an embedding provider")
}

func (p *AnthropicProvider) ModelName() string {
	return p.Model
}

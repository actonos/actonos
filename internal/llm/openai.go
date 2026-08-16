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

// OpenAIProvider interacts with OpenAI-compatible chat completion and embedding APIs.
type OpenAIProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewOpenAIProvider creates a new provider instance for OpenAI or compatible endpoints (e.g. Ollama, Groq, DeepSeek).
func NewOpenAIProvider(apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type openAIChatRequest struct {
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Tools       []ToolDefinition  `json:"tools,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}

	reqBody := openAIChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Tools:       opts.Tools,
		Stream:      false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading openai response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshalling openai response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned by openai")
	}

	firstChoice := chatResp.Choices[0]
	return &Response{
		Content:   firstChoice.Message.Content,
		ToolCalls: firstChoice.Message.ToolCalls,
		Model:     chatResp.Model,
		Usage: Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, nil
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	// For streaming, can stream SSE tokens
	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		resp, err := p.Complete(ctx, messages, opts)
		if err != nil {
			ch <- StreamChunk{Error: err}
			return
		}
		ch <- StreamChunk{DeltaContent: resp.Content, ToolCalls: resp.ToolCalls, Done: false}
		ch <- StreamChunk{Done: true, Usage: &resp.Usage}
	}()
	return ch, nil
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (p *OpenAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIEmbedRequest{
		Model: "text-embedding-3-small",
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embeddings api error (status %d): %s", resp.StatusCode, string(b))
	}

	var embedResp openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, err
	}

	res := make([][]float32, len(texts))
	for _, item := range embedResp.Data {
		if item.Index < len(res) {
			res[item.Index] = item.Embedding
		}
	}
	return res, nil
}

func (p *OpenAIProvider) ModelName() string {
	return p.Model
}

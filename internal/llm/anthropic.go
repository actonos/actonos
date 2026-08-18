package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
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

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text,omitempty"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
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

	var anthropicTools []anthropicTool
	for _, t := range opts.Tools {
		schema := t.Function.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		anthropicTools = append(anthropicTools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}

	reqBody := anthropicRequest{
		Model:       model,
		Messages:    anthropicMsgs,
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: opts.Temperature,
		Tools:       anthropicTools,
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
	var toolCalls []ToolCall
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			content += block.Text
		} else if block.Type == "tool_use" {
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		}
	}

	return &Response{
		Content:   content,
		ToolCalls: toolCalls,
		Model:     anthropicResp.Model,
		Usage: Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
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
	for _, message := range messages {
		if message.Role == RoleSystem {
			systemPrompt += message.Content + "\n"
			continue
		}
		role := "user"
		if message.Role == RoleAssistant {
			role = "assistant"
		}
		anthropicMsgs = append(anthropicMsgs, anthropicMessage{Role: role, Content: message.Content})
	}
	var anthropicTools []anthropicTool
	for _, tool := range opts.Tools {
		anthropicTools = append(anthropicTools, anthropicTool{
			Name: tool.Function.Name, Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	payload, err := json.Marshal(anthropicRequest{
		Model: model, Messages: anthropicMsgs, System: strings.TrimSpace(systemPrompt),
		MaxTokens: maxTokens, Temperature: opts.Temperature, Tools: anthropicTools, Stream: true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling anthropic stream request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic stream request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("anthropic stream error (status %d): %s", resp.StatusCode, string(body))
	}
	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		type pendingTool struct {
			id, name string
			args     strings.Builder
		}
		pending := map[int]*pendingTool{}
		usage := Usage{}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 4<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event struct {
				Type    string `json:"type"`
				Index   int    `json:"index"`
				Message struct {
					Model string `json:"model"`
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				ch <- StreamChunk{Error: err}
				return
			}
			switch event.Type {
			case "message_start":
				usage.PromptTokens = event.Message.Usage.InputTokens
			case "content_block_start":
				if event.ContentBlock.Type == "tool_use" {
					pending[event.Index] = &pendingTool{id: event.ContentBlock.ID, name: event.ContentBlock.Name}
				}
			case "content_block_delta":
				if event.Delta.Text != "" {
					ch <- StreamChunk{DeltaContent: event.Delta.Text}
				}
				if tool := pending[event.Index]; tool != nil {
					tool.args.WriteString(event.Delta.PartialJSON)
				}
			case "message_delta":
				usage.CompletionTokens = event.Usage.OutputTokens
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			case "message_stop":
				var calls []ToolCall
				indexes := make([]int, 0, len(pending))
				for index := range pending {
					indexes = append(indexes, index)
				}
				sort.Ints(indexes)
				for _, index := range indexes {
					tool := pending[index]
					if tool != nil {
						calls = append(calls, ToolCall{ID: tool.id, Type: "function", Function: FunctionCall{
							Name: tool.name, Arguments: json.RawMessage(tool.args.String()),
						}})
					}
				}
				if len(calls) > 0 {
					ch <- StreamChunk{ToolCalls: calls}
				}
				ch <- StreamChunk{Done: true, Usage: &usage}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Error: err}
			return
		}
		ch <- StreamChunk{Done: true, Usage: &usage}
	}()
	return ch, nil
}

func (p *AnthropicProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("anthropic does not provide an embedding endpoint; configure an embedding provider")
}

func (p *AnthropicProvider) ModelName() string {
	return p.Model
}

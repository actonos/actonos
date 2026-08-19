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

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	Index    *int               `json:"index,omitempty"`
	ID       string             `json:"id"`
	Type     string             `json:"type"` // "function"
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

func normalizeToolArguments(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}
	if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
		var unquoted string
		if err := json.Unmarshal(raw, &unquoted); err == nil {
			return unquoted
		}
	}
	return trimmed
}

func toOpenAIMessages(messages []Message) []openAIMessage {
	result := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		var toolCalls []openAIToolCall
		if len(m.ToolCalls) > 0 {
			toolCalls = make([]openAIToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				argStr := normalizeToolArguments(tc.Function.Arguments)
				toolType := tc.Type
				if toolType == "" {
					toolType = "function"
				}
				toolCalls = append(toolCalls, openAIToolCall{
					ID:   tc.ID,
					Type: toolType,
					Function: openAIFunctionCall{
						Name:      tc.Function.Name,
						Arguments: argStr,
					},
				})
			}
		}

		// An assistant turn that only carries tool_calls must send content as JSON
		// null, not "". Several OpenAI-compatible gateways reject an empty string
		// paired with tool_calls, which surfaces as a hard 400 on the very next
		// ReAct iteration.
		var contentPtr *string
		if !(m.Role == RoleAssistant && m.Content == "" && len(toolCalls) > 0) {
			c := m.Content
			contentPtr = &c
		}

		// reasoning_content is an OUTPUT-only field. DeepSeek (and other reasoning
		// gateways) reject requests that echo it back in an input message, so it is
		// never replayed to the provider.
		result = append(result, openAIMessage{
			Role:       string(m.Role),
			Content:    contentPtr,
			ToolCalls:  toolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		})
	}
	return result
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
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
	messages = SanitizeMessages(messages)
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}

	reqBody := openAIChatRequest{
		Model:       model,
		Messages:    toOpenAIMessages(messages),
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
	var respToolCalls []ToolCall
	for _, tc := range firstChoice.Message.ToolCalls {
		toolType := tc.Type
		if toolType == "" {
			toolType = "function"
		}
		respToolCalls = append(respToolCalls, ToolCall{
			ID:   tc.ID,
			Type: toolType,
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	return &Response{
		Content:          firstChoice.Message.Content,
		ReasoningContent: firstChoice.Message.ReasoningContent,
		ToolCalls:        respToolCalls,
		Model:            chatResp.Model,
		Usage: Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, nil
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	messages = SanitizeMessages(messages)
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}
	reqBody := openAIChatRequest{
		Model: model, Messages: toOpenAIMessages(messages), Temperature: opts.Temperature,
		MaxTokens: opts.MaxTokens, Tools: opts.Tools, Stream: true,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling streaming request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating streaming request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai streaming request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("openai streaming api error (status %d): %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		type pendingCall struct {
			id        string
			callType  string
			name      string
			arguments strings.Builder
		}
		pending := map[int]*pendingCall{}
		emitPending := func() {
			if len(pending) == 0 {
				return
			}
			calls := make([]ToolCall, 0, len(pending))
			indexes := make([]int, 0, len(pending))
			for index := range pending {
				indexes = append(indexes, index)
			}
			sort.Ints(indexes)
			for _, index := range indexes {
				call := pending[index]
				// A call with no id or no name is an incomplete fragment: forwarding it
				// produces an assistant tool_call that no result can ever be paired with.
				if call == nil || call.id == "" || call.name == "" {
					continue
				}
				args := strings.TrimSpace(call.arguments.String())
				if args == "" {
					args = "{}"
				}
				calls = append(calls, ToolCall{
					ID: call.id, Type: call.callType,
					Function: FunctionCall{Name: call.name, Arguments: json.RawMessage(args)},
				})
			}
			// emitPending may run twice (on [DONE] and again after the scan loop);
			// clearing prevents the same tool_calls being emitted a second time and
			// duplicated into the transcript.
			pending = map[int]*pendingCall{}
			if len(calls) == 0 {
				return
			}
			ch <- StreamChunk{ToolCalls: calls}
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 4<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				emitPending()
				ch <- StreamChunk{Done: true}
				return
			}
			var event struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content,omitempty"`
						ToolCalls        []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
				Usage *Usage `json:"usage"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				ch <- StreamChunk{Error: fmt.Errorf("decoding openai stream event: %w", err)}
				return
			}
			if event.Error != nil {
				ch <- StreamChunk{Error: fmt.Errorf("openai stream error: %s", event.Error.Message)}
				return
			}
			if len(event.Choices) > 0 {
				for _, delta := range event.Choices[0].Delta.ToolCalls {
					call := pending[delta.Index]
					if call == nil {
						call = &pendingCall{}
						pending[delta.Index] = call
					}
					if delta.ID != "" {
						call.id = delta.ID
					}
					if delta.Type != "" {
						call.callType = delta.Type
					}
					if delta.Function.Name != "" {
						call.name = delta.Function.Name
					}
					call.arguments.WriteString(delta.Function.Arguments)
				}
				ch <- StreamChunk{
					DeltaContent:   event.Choices[0].Delta.Content,
					DeltaReasoning: event.Choices[0].Delta.ReasoningContent,
					Usage:          event.Usage,
				}
			} else if event.Usage != nil {
				ch <- StreamChunk{Usage: event.Usage}
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Error: fmt.Errorf("reading openai stream: %w", err)}
			return
		}
		emitPending()
		ch <- StreamChunk{Done: true}
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

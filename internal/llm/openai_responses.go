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
)

// openAIResponsesRequest is the native OpenAI Responses API wire format. It is
// deliberately separate from openAIChatRequest because compatible providers
// still implement Chat Completions rather than Responses.
type openAIResponsesRequest struct {
	Model           string               `json:"model"`
	Input           []json.RawMessage    `json:"input"`
	Instructions    string               `json:"instructions,omitempty"`
	Tools           []openAIResponseTool `json:"tools,omitempty"`
	Reasoning       *openAIReasoning     `json:"reasoning,omitempty"`
	MaxOutputTokens *int                 `json:"max_output_tokens,omitempty"`
	Stream          bool                 `json:"stream,omitempty"`
	Store           bool                 `json:"store"`
}

type openAIReasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary,omitempty"`
}

type openAIMessageItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIFunctionCallItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIFunctionCallOutputItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"` // Mandatory for function_call_output in OpenAI Responses API; never omitempty
}

type openAIResponseTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIResponsesResponse struct {
	Model      string            `json:"model"`
	OutputText string            `json:"output_text"`
	Output     []json.RawMessage `json:"output"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func appendMessageItem(request *openAIResponsesRequest, role, content string) {
	raw, _ := json.Marshal(openAIMessageItem{Role: role, Content: content})
	request.Input = append(request.Input, raw)
}

func appendFunctionCallItem(request *openAIResponsesRequest, callID, name string, args json.RawMessage) {
	raw, _ := json.Marshal(openAIFunctionCallItem{
		Type:      "function_call",
		CallID:    callID,
		Name:      name,
		Arguments: normalizeToolArguments(args),
	})
	request.Input = append(request.Input, raw)
}

func appendFunctionCallOutputItem(request *openAIResponsesRequest, callID, output string) {
	raw, _ := json.Marshal(openAIFunctionCallOutputItem{
		Type:   "function_call_output",
		CallID: callID,
		Output: output,
	})
	request.Input = append(request.Input, raw)
}

func toOpenAIResponsesRequest(messages []Message, model string, opts CompletionOptions, stream bool) openAIResponsesRequest {
	opts = opts.WithDefaults()
	request := openAIResponsesRequest{
		Model:           model,
		Input:           make([]json.RawMessage, 0, len(messages)),
		Reasoning:       &openAIReasoning{Effort: opts.ReasoningEffort, Summary: "detailed"},
		MaxOutputTokens: opts.MaxTokens,
		Stream:          stream,
		Store:           false,
	}
	for _, tool := range opts.Tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		request.Tools = append(request.Tools, openAIResponseTool{
			Type:        "function",
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}

	for _, message := range messages {
		switch message.Role {
		case RoleSystem:
			if message.Content == "" {
				continue
			}
			if request.Instructions == "" {
				request.Instructions = message.Content
			} else {
				request.Instructions += "\n\n" + message.Content
			}
		case RoleTool:
			if message.ToolCallID == "" {
				continue
			}
			appendFunctionCallOutputItem(&request, message.ToolCallID, message.Content)
		case RoleAssistant:
			for _, item := range message.ProviderItems {
				var metadata struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(item, &metadata) == nil && metadata.Type == "reasoning" {
					request.Input = append(request.Input, item)
				}
			}
			if message.Content != "" {
				appendMessageItem(&request, string(RoleAssistant), message.Content)
			}
			for _, call := range message.ToolCalls {
				if call.ID == "" || call.Function.Name == "" {
					continue
				}
				appendFunctionCallItem(&request, call.ID, call.Function.Name, call.Function.Arguments)
			}
		default:
			appendMessageItem(&request, string(message.Role), message.Content)
		}
	}
	return request
}

func (p *OpenAIProvider) completeResponses(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	messages = SanitizeMessages(messages)
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}
	payload, err := json.Marshal(toOpenAIResponsesRequest(messages, model, opts, false))
	if err != nil {
		return nil, fmt.Errorf("marshalling OpenAI Responses request: %w", err)
	}
	req, err := p.newResponsesRequest(ctx, payload, false)
	if err != nil {
		return nil, err
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Responses request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading OpenAI Responses response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("OpenAI Responses API error (status %d): %s", resp.StatusCode, string(body))
	}
	var apiResp openAIResponsesResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decoding OpenAI Responses response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("OpenAI Responses error: %s", apiResp.Error.Message)
	}
	return responseFromOpenAIResponses(apiResp), nil
}

func responseFromOpenAIResponses(apiResp openAIResponsesResponse) *Response {
	content := apiResp.OutputText
	var calls []ToolCall
	var providerItems []json.RawMessage
	for _, rawItem := range apiResp.Output {
		var item struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(rawItem, &item); err != nil {
			continue
		}
		switch item.Type {
		case "function_call":
			if item.CallID == "" || item.Name == "" {
				continue
			}
			args := item.Arguments
			if args == "" {
				args = "{}"
			}
			calls = append(calls, ToolCall{ID: item.CallID, Type: "function", Function: FunctionCall{Name: item.Name, Arguments: json.RawMessage(args)}})
		case "message":
			if content != "" {
				continue
			}
			for _, part := range item.Content {
				if part.Type == "output_text" {
					content += part.Text
				}
			}
		case "reasoning":
			providerItems = append(providerItems, rawItem)
		}
	}
	return &Response{
		Content: content, ToolCalls: calls, Model: apiResp.Model,
		ProviderItems: providerItems,
		Usage:         Usage{PromptTokens: apiResp.Usage.InputTokens, CompletionTokens: apiResp.Usage.OutputTokens, TotalTokens: apiResp.Usage.TotalTokens},
	}
}

func (p *OpenAIProvider) newResponsesRequest(ctx context.Context, payload []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating OpenAI Responses request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	return req, nil
}

func (p *OpenAIProvider) streamResponses(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	messages = SanitizeMessages(messages)
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}
	payload, err := json.Marshal(toOpenAIResponsesRequest(messages, model, opts, true))
	if err != nil {
		return nil, fmt.Errorf("marshalling OpenAI Responses streaming request: %w", err)
	}
	req, err := p.newResponsesRequest(ctx, payload, true)
	if err != nil {
		return nil, err
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Responses streaming request failed: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("OpenAI Responses streaming API error (status %d): %s", resp.StatusCode, string(body))
	}

	chunks := make(chan StreamChunk, 16)
	go func() {
		defer close(chunks)
		defer resp.Body.Close()
		type pendingCall struct {
			callID string
			name   string
			args   strings.Builder
		}
		pending := make(map[int]*pendingCall)
		emitPending := func() {
			indexes := make([]int, 0, len(pending))
			for index := range pending {
				indexes = append(indexes, index)
			}
			sort.Ints(indexes)
			calls := make([]ToolCall, 0, len(indexes))
			for _, index := range indexes {
				call := pending[index]
				if call == nil || call.callID == "" || call.name == "" {
					continue
				}
				args := strings.TrimSpace(call.args.String())
				if args == "" {
					args = "{}"
				}
				calls = append(calls, ToolCall{ID: call.callID, Type: "function", Function: FunctionCall{Name: call.name, Arguments: json.RawMessage(args)}})
			}
			pending = make(map[int]*pendingCall)
			if len(calls) > 0 {
				chunks <- StreamChunk{ToolCalls: calls}
			}
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
				chunks <- StreamChunk{Done: true}
				return
			}
			var event struct {
				Type        string `json:"type"`
				Delta       string `json:"delta"`
				OutputIndex int    `json:"output_index"`
				CallID      string `json:"call_id"`
				Name        string `json:"name"`
				Arguments   string `json:"arguments"`
				Item        *struct {
					Type      string `json:"type"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"item"`
				Response *struct {
					Model string `json:"model"`
					Usage struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
						TotalTokens  int `json:"total_tokens"`
					} `json:"usage"`
				} `json:"response"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				chunks <- StreamChunk{Error: fmt.Errorf("decoding OpenAI Responses stream event: %w", err)}
				return
			}
			var rawEnvelope struct {
				Item json.RawMessage `json:"item"`
			}
			_ = json.Unmarshal([]byte(data), &rawEnvelope)
			if event.Type == "error" || event.Error != nil {
				message := "unknown streaming error"
				if event.Error != nil && event.Error.Message != "" {
					message = event.Error.Message
				}
				chunks <- StreamChunk{Error: fmt.Errorf("OpenAI Responses stream error: %s", message)}
				return
			}
			switch event.Type {
			case "response.output_text.delta":
				if event.Delta != "" {
					chunks <- StreamChunk{DeltaContent: event.Delta}
				}
			case "response.reasoning_summary_text.delta":
				if event.Delta != "" {
					chunks <- StreamChunk{DeltaReasoning: event.Delta}
				}
			case "response.output_item.added":
				if event.Item != nil {
					if event.Item.Type == "reasoning" && len(rawEnvelope.Item) > 0 {
						chunks <- StreamChunk{ProviderItems: []json.RawMessage{rawEnvelope.Item}}
						break
					}
					if event.Item.Type != "function_call" {
						break
					}
					event.Name = event.Item.Name
					event.CallID = event.Item.CallID
					event.Arguments = event.Item.Arguments
				}
				if event.Name != "" || event.CallID != "" {
					call := pending[event.OutputIndex]
					if call == nil {
						call = &pendingCall{}
						pending[event.OutputIndex] = call
					}
					if event.CallID != "" {
						call.callID = event.CallID
					}
					if event.Name != "" {
						call.name = event.Name
					}
				}
			case "response.function_call_arguments.delta":
				call := pending[event.OutputIndex]
				if call == nil {
					call = &pendingCall{}
					pending[event.OutputIndex] = call
				}
				if event.CallID != "" {
					call.callID = event.CallID
				}
				if event.Name != "" {
					call.name = event.Name
				}
				call.args.WriteString(event.Delta)
			case "response.function_call_arguments.done":
				call := pending[event.OutputIndex]
				if call == nil {
					call = &pendingCall{}
					pending[event.OutputIndex] = call
				}
				if event.CallID != "" {
					call.callID = event.CallID
				}
				if event.Name != "" {
					call.name = event.Name
				}
				if event.Arguments != "" {
					call.args.Reset()
					call.args.WriteString(event.Arguments)
				}
			case "response.output_item.done":
				if event.Item == nil || event.Item.Type != "function_call" {
					break
				}
				call := pending[event.OutputIndex]
				if call == nil {
					call = &pendingCall{}
					pending[event.OutputIndex] = call
				}
				if event.Item.CallID != "" {
					call.callID = event.Item.CallID
				}
				if event.Item.Name != "" {
					call.name = event.Item.Name
				}
				if event.Item.Arguments != "" {
					call.args.Reset()
					call.args.WriteString(event.Item.Arguments)
				}
			case "response.completed":
				emitPending()
				if event.Response != nil {
					u := event.Response.Usage
					chunks <- StreamChunk{Usage: &Usage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens, TotalTokens: u.TotalTokens}}
				}
				chunks <- StreamChunk{Done: true}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			chunks <- StreamChunk{Error: fmt.Errorf("reading OpenAI Responses stream: %w", err)}
			return
		}
		emitPending()
		chunks <- StreamChunk{Done: true}
	}()
	return chunks, nil
}

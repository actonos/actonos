package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GeminiProvider implements LLMProvider for Google Gemini API.
type GeminiProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewGeminiProvider creates a new Google Gemini provider.
func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiProvider{
		BaseURL:    "https://generativelanguage.googleapis.com/v1beta",
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: NewDefaultHTTPClient(),
	}
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerateRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (p *GeminiProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}

	var systemInstruction *geminiContent
	var contents []geminiContent

	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemInstruction = &geminiContent{
				Role:  "system",
				Parts: []geminiPart{{Text: msg.Content}},
			}
		} else {
			role := "user"
			if msg.Role == RoleAssistant {
				role = "model"
			}
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: msg.Content}},
			})
		}
	}

	reqBody := geminiGenerateRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling gemini request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.BaseURL, model, p.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiGenerateResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", geminiResp.Error.Message)
	}

	var outputText string
	if len(geminiResp.Candidates) > 0 {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			outputText += part.Text
		}
	}

	return &Response{
		Content: outputText,
		Model:   model,
		Usage: Usage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

func (p *GeminiProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	model := p.Model
	if opts.Model != "" {
		model = opts.Model
	}
	var systemInstruction *geminiContent
	var contents []geminiContent
	for _, message := range messages {
		if message.Role == RoleSystem {
			systemInstruction = &geminiContent{Role: "system", Parts: []geminiPart{{Text: message.Content}}}
			continue
		}
		role := "user"
		if message.Role == RoleAssistant {
			role = "model"
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: message.Content}}})
	}
	payload, err := json.Marshal(geminiGenerateRequest{Contents: contents, SystemInstruction: systemInstruction})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", p.BaseURL, model, p.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("gemini stream error (status %d): %s", resp.StatusCode, string(body))
	}
	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 4<<20)
		var usage Usage
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event geminiGenerateResponse
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				ch <- StreamChunk{Error: err}
				return
			}
			if event.Error != nil {
				ch <- StreamChunk{Error: fmt.Errorf("gemini stream error: %s", event.Error.Message)}
				return
			}
			for _, candidate := range event.Candidates {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						ch <- StreamChunk{DeltaContent: part.Text}
					}
				}
			}
			usage = Usage{
				PromptTokens:     event.UsageMetadata.PromptTokenCount,
				CompletionTokens: event.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      event.UsageMetadata.TotalTokenCount,
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

type geminiEmbedRequest struct {
	Content geminiContent `json:"content"`
}

type geminiEmbedResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
}

func (p *GeminiProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, txt := range texts {
		reqBody := geminiEmbedRequest{
			Content: geminiContent{
				Parts: []geminiPart{{Text: txt}},
			},
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}

		url := fmt.Sprintf("%s/models/text-embedding-004:embedContent?key=%s", p.BaseURL, p.APIKey)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		resp, err := p.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("gemini embed error (status %d): %s", resp.StatusCode, string(b))
		}

		var embedResp geminiEmbedResponse
		if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
			return nil, err
		}
		results[i] = embedResp.Embedding.Values
	}
	return results, nil
}

func (p *GeminiProvider) ModelName() string {
	return p.Model
}

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// 8. Web Search Tool
// -----------------------------------------------------------------------------

type WebSearchTool struct {
	client *http.Client
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *WebSearchTool) Name() string { return "native_web_search" }
func (t *WebSearchTool) Description() string {
	return "Search the web for real-time information, documentation, news, or solutions."
}
func (t *WebSearchTool) Category() string { return "native" }

func (t *WebSearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": { "type": "string", "description": "Search query keywords" },
			"max_results": { "type": "integer", "description": "Max results to return (default 5)" }
		},
		"required": ["query"]
	}`)
}

type SearchItem struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

func (t *WebSearchTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Query == "" {
		return nil, errors.New("query is required")
	}
	if input.MaxResults <= 0 || input.MaxResults > 10 {
		input.MaxResults = 5
	}

	results := t.searchDDGLite(ctx, input.Query, input.MaxResults)
	if len(results) == 0 {
		results = t.searchDDGHTML(ctx, input.Query, input.MaxResults)
	}
	if len(results) == 0 {
		results = t.searchDDGAPI(ctx, input.Query, input.MaxResults)
	}

	if len(results) == 0 {
		return &ToolResult{
			Content: fmt.Sprintf("Search for '%s' completed without extracting structured external snippets. Synthesize the answer using your knowledge base or clarify the query.", input.Query),
			Data:    map[string]any{"query": input.Query, "count": 0},
		}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Web Search Results for '%s' (%d items found):\n\n", input.Query, len(results))
	for i, item := range results {
		fmt.Fprintf(&sb, "%d. **%s**\n   - Snippet: %s\n   - URL: %s\n\n", i+1, item.Title, item.Snippet, item.URL)
	}

	return &ToolResult{
		Content: strings.TrimSpace(sb.String()),
		Data:    map[string]any{"query": input.Query, "count": len(results), "results": results},
	}, nil
}

func cleanHTMLTags(s string) string {
	s = html.UnescapeString(s)
	for {
		start := strings.Index(s, "<")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], ">")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	return strings.TrimSpace(s)
}

func (t *WebSearchTool) searchDDGLite(ctx context.Context, query string, maxResults int) []SearchItem {
	formData := url.Values{"q": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://lite.duckduckgo.com/lite/", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}
	bodyStr := string(bodyBytes)

	var results []SearchItem
	snippets := strings.Split(bodyStr, "class=\"result-snippet\"")
	if len(snippets) <= 1 {
		snippets = strings.Split(bodyStr, "class='result-snippet'")
	}
	for i := 1; i < len(snippets) && len(results) < maxResults; i++ {
		chunk := snippets[i]
		if closeTag := strings.Index(chunk, ">"); closeTag != -1 {
			chunk = chunk[closeTag+1:]
		}
		if endTag := strings.Index(chunk, "</td>"); endTag != -1 {
			snippet := cleanHTMLTags(chunk[:endTag])
			if len(snippet) > 15 {
				results = append(results, SearchItem{
					Title:   fmt.Sprintf("Web Result #%d", len(results)+1),
					Snippet: snippet,
					URL:     "https://duckduckgo.com/?q=" + url.QueryEscape(query),
				})
			}
		}
	}
	return results
}

func (t *WebSearchTool) searchDDGHTML(ctx context.Context, query string, maxResults int) []SearchItem {
	formData := url.Values{"q": {query}, "b": {""}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://html.duckduckgo.com/html/", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}
	bodyStr := string(bodyBytes)

	var results []SearchItem
	snippets := strings.Split(bodyStr, "class=\"result__snippet\"")
	for i := 1; i < len(snippets) && len(results) < maxResults; i++ {
		chunk := snippets[i]
		if closeTag := strings.Index(chunk, ">"); closeTag != -1 {
			chunk = chunk[closeTag+1:]
		}
		if endTag := strings.Index(chunk, "</a>"); endTag != -1 {
			snippet := cleanHTMLTags(chunk[:endTag])
			if len(snippet) > 15 {
				results = append(results, SearchItem{
					Title:   fmt.Sprintf("Result #%d", len(results)+1),
					Snippet: snippet,
					URL:     "https://duckduckgo.com/?q=" + url.QueryEscape(query),
				})
			}
		}
	}
	return results
}

func (t *WebSearchTool) searchDDGAPI(ctx context.Context, query string, maxResults int) []SearchItem {
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "ActonOS/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var data struct {
		AbstractText  string `json:"AbstractText"`
		AbstractURL   string `json:"AbstractURL"`
		Heading       string `json:"Heading"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}

	var results []SearchItem
	if data.AbstractText != "" {
		results = append(results, SearchItem{
			Title:   data.Heading,
			Snippet: data.AbstractText,
			URL:     data.AbstractURL,
		})
	}
	for _, topic := range data.RelatedTopics {
		if len(results) >= maxResults {
			break
		}
		if topic.Text != "" {
			results = append(results, SearchItem{
				Title:   fmt.Sprintf("Topic #%d", len(results)+1),
				Snippet: topic.Text,
				URL:     topic.FirstURL,
			})
		}
	}
	return results
}

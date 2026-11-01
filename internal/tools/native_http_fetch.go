package tools

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/security"
)

// -----------------------------------------------------------------------------
// 1. HTTP Fetch Tool
// -----------------------------------------------------------------------------

// MaxHTTPFetchChars is the maximum readable text returned to the model.
// Raw HTML pages are often hundreds of KB and would overflow the context window.
const MaxHTTPFetchChars = 16000

var (
	htmlScriptRe   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	htmlStyleRe    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	htmlNoScriptRe = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`)
	htmlCommentRe  = regexp.MustCompile(`(?is)<!--.*?-->`)
	htmlTagRe      = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlSpaceRe    = regexp.MustCompile(`[\s\x00]+`)
)

type HTTPFetchTool struct {
	client *http.Client
}

func newOutboundHTTPClient(timeout time.Duration) *http.Client {
	// HTTP/2 against some CDNs (Cloudflare, Akamai) yields INTERNAL_ERROR on
	// stream ID 1. Force HTTP/1.1 for research fetches.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          map[string]func(authority string, c *tls.Conn) http.RoundTripper{},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return security.ValidateOutboundURL(req.Context(), req.URL.String())
		},
	}
}

func NewHTTPFetchTool() *HTTPFetchTool {
	return &HTTPFetchTool{client: newOutboundHTTPClient(30 * time.Second)}
}

func (t *HTTPFetchTool) Name() string { return "native_http_fetch" }
func (t *HTTPFetchTool) Description() string {
	return "Fetch content or JSON from an external URL via HTTP GET."
}
func (t *HTTPFetchTool) Category() string { return "native" }

func (t *HTTPFetchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": { "type": "string", "description": "The full HTTP/HTTPS URL to fetch" }
		},
		"required": ["url"]
	}`)
}

func (t *HTTPFetchTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.URL == "" {
		var strURL string
		if strErr := json.Unmarshal(inputJSON, &strURL); strErr == nil && strURL != "" {
			input.URL = strURL
		} else {
			var m map[string]any
			if mapErr := json.Unmarshal(inputJSON, &m); mapErr == nil {
				if u, ok := m["url"].(string); ok && u != "" {
					input.URL = u
				} else if u, ok := m["URL"].(string); ok && u != "" {
					input.URL = u
				}
			}
		}
	}

	if input.URL == "" {
		return nil, errors.New("url parameter is required")
	}

	if !strings.HasPrefix(input.URL, "http://") && !strings.HasPrefix(input.URL, "https://") {
		input.URL = "https://" + input.URL
	}
	if err := security.ValidateOutboundURL(ctx, input.URL); err != nil {
		return nil, fmt.Errorf("validating outbound URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ActonOS/1.0; +https://actonos.local)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	content := formatHTTPFetchContent(resp.StatusCode, input.URL, resp.Header.Get("Content-Type"), bodyBytes)
	return &ToolResult{
		Content: content,
		Data: map[string]any{
			"status_code": resp.StatusCode,
			"url":         input.URL,
			"bytes":       len(bodyBytes),
			"chars":       len(content),
		},
	}, nil
}

func formatHTTPFetchContent(status int, rawURL, contentType string, body []byte) string {
	text := extractReadableHTTPBody(contentType, body)
	originalChars := len(text)
	if originalChars > MaxHTTPFetchChars {
		text = text[:MaxHTTPFetchChars] + fmt.Sprintf(
			"\n\n[Content truncated to %d of %d characters. Fetch a more specific URL or section if needed.]",
			MaxHTTPFetchChars, originalChars,
		)
	}
	return fmt.Sprintf("HTTP %d %s\n\n%s", status, rawURL, text)
}

func extractReadableHTTPBody(contentType string, body []byte) string {
	if len(body) == 0 {
		return "(empty response)"
	}
	if bytes.IndexByte(body, 0) >= 0 {
		return fmt.Sprintf("(binary response, %d bytes; not converted to text)", len(body))
	}
	raw := string(body)
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "json") {
		return strings.TrimSpace(raw)
	}
	if strings.Contains(ct, "html") || looksLikeHTML(raw) {
		return htmlToVisibleText(raw)
	}
	return strings.TrimSpace(htmlSpaceRe.ReplaceAllString(raw, " "))
}

func looksLikeHTML(raw string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html") ||
		strings.Contains(trimmed, "<head>") || strings.Contains(trimmed, "<body")
}

func htmlToVisibleText(raw string) string {
	text := htmlCommentRe.ReplaceAllString(raw, " ")
	text = htmlScriptRe.ReplaceAllString(text, " ")
	text = htmlStyleRe.ReplaceAllString(text, " ")
	text = htmlNoScriptRe.ReplaceAllString(text, " ")
	text = htmlTagRe.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = htmlSpaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

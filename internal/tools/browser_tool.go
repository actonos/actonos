package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// BrowserNavigateTool opens a URL in a headless browser and extracts rendered title and visible text.
type BrowserNavigateTool struct{}

// NewBrowserNavigateTool creates a new BrowserNavigateTool.
func NewBrowserNavigateTool() *BrowserNavigateTool {
	return &BrowserNavigateTool{}
}

func (t *BrowserNavigateTool) Name() string { return "native_browser_navigate" }
func (t *BrowserNavigateTool) Description() string {
	return "Navigate to a webpage using a headless browser, wait for JavaScript/DOM rendering, and extract clean text."
}
func (t *BrowserNavigateTool) Category() string { return "native" }

func (t *BrowserNavigateTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": { "type": "string", "description": "The HTTP or HTTPS URL to navigate to" },
			"wait_seconds": { "type": "integer", "description": "Seconds to wait after page load for dynamic content (1-10)", "default": 2 }
		},
		"required": ["url"]
	}`)
}

func (t *BrowserNavigateTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	var input struct {
		URL         string `json:"url"`
		WaitSeconds int    `json:"wait_seconds"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if input.URL == "" {
		return nil, errors.New("url parameter is required")
	}
	if input.WaitSeconds <= 0 || input.WaitSeconds > 10 {
		input.WaitSeconds = 2
	}

	// Create headless Chrome context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	// Set timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(taskCtx, 20*time.Second)
	defer timeoutCancel()

	var title string
	var bodyText string

	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(input.URL),
		chromedp.Sleep(time.Duration(input.WaitSeconds)*time.Second),
		chromedp.Title(&title),
		chromedp.Text("body", &bodyText, chromedp.ByQuery),
	)

	if err != nil {
		// If chromedp fails (e.g. headless chromium not installed on host), fallback to standard HTTP fetch
		fallbackResult, fbErr := fallbackHTTPFetch(ctx, input.URL)
		if fbErr == nil {
			return &ToolResult{
				Content: fmt.Sprintf("[Browser Fallback - Static HTTP]\nURL: %s\n\n%s", input.URL, fallbackResult),
				Data: map[string]any{
					"url":      input.URL,
					"fallback": true,
				},
			}, nil
		}
		return nil, fmt.Errorf("browser navigation failed: %w", err)
	}

	// Clean up whitespace
	cleanText := strings.TrimSpace(bodyText)
	if len(cleanText) > 8000 {
		cleanText = cleanText[:8000] + "\n\n...[Content truncated to 8000 characters]..."
	}

	return &ToolResult{
		Content: fmt.Sprintf("Title: %s\nURL: %s\n\n%s", title, input.URL, cleanText),
		Data: map[string]any{
			"title": title,
			"url":   input.URL,
		},
	}, nil
}

// BrowserScreenshotTool captures a screenshot of a rendered webpage.
type BrowserScreenshotTool struct {
	workspaceDir string
}

// NewBrowserScreenshotTool creates a new BrowserScreenshotTool.
func NewBrowserScreenshotTool(workspaceDir string) *BrowserScreenshotTool {
	return &BrowserScreenshotTool{workspaceDir: workspaceDir}
}

func (t *BrowserScreenshotTool) Name() string { return "native_browser_screenshot" }
func (t *BrowserScreenshotTool) Description() string {
	return "Capture a screenshot of a given webpage and save the image into workspace."
}
func (t *BrowserScreenshotTool) Category() string { return "native" }

func (t *BrowserScreenshotTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": { "type": "string", "description": "The URL to screenshot" },
			"output_path": { "type": "string", "description": "Relative filename in workspace (e.g. screenshot.png)", "default": "screenshot.png" }
		},
		"required": ["url"]
	}`)
}

func (t *BrowserScreenshotTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	var input struct {
		URL        string `json:"url"`
		OutputPath string `json:"output_path"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if input.URL == "" {
		return nil, errors.New("url parameter is required")
	}
	if input.OutputPath == "" {
		input.OutputPath = fmt.Sprintf("screenshot_%d.png", time.Now().Unix())
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(taskCtx, 25*time.Second)
	defer timeoutCancel()

	var buf []byte
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(input.URL),
		chromedp.Sleep(2*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)

	if err != nil {
		return nil, fmt.Errorf("screenshot capture failed: %w", err)
	}

	// Save to workspace
	cleanRel := filepath.Clean(input.OutputPath)
	targetPath := filepath.Join(t.workspaceDir, cleanRel)
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)

	if err := os.WriteFile(targetPath, buf, 0644); err != nil {
		return nil, fmt.Errorf("writing screenshot file: %w", err)
	}

	base64Preview := base64.StdEncoding.EncodeToString(buf)
	if len(base64Preview) > 200 {
		base64Preview = base64Preview[:200] + "..."
	}

	return &ToolResult{
		Content: fmt.Sprintf("Captured screenshot of %s (%d bytes). Saved to workspace at %s", input.URL, len(buf), input.OutputPath),
		Data: map[string]any{
			"url":         input.URL,
			"path":        input.OutputPath,
			"size_bytes":  len(buf),
			"preview_b64": base64Preview,
		},
	}, nil
}

func fallbackHTTPFetch(ctx context.Context, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	return string(body), nil
}

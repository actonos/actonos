package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	ErrPathEscape = errors.New("access denied: path escapes allowed workspace directory")
)

// RegisterNativeTools adds default native tools to the registry.
func RegisterNativeTools(r *ToolRegistry, workspaceDir string) {
	if workspaceDir == "" {
		workspaceDir = "./data/workspace"
	}
	_ = r.Register(NewHTTPFetchTool())
	_ = r.Register(NewFileReadTool(workspaceDir))
	_ = r.Register(NewFileWriteTool(workspaceDir))
	_ = r.Register(NewSysInfoTool())
	_ = r.Register(NewBrowserNavigateTool())
	_ = r.Register(NewBrowserScreenshotTool(workspaceDir))
}

// -----------------------------------------------------------------------------
// 1. HTTP Fetch Tool
// -----------------------------------------------------------------------------

type HTTPFetchTool struct {
	client *http.Client
}

func NewHTTPFetchTool() *HTTPFetchTool {
	return &HTTPFetchTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *HTTPFetchTool) Name() string        { return "native_http_fetch" }
func (t *HTTPFetchTool) Description() string { return "Fetch content or JSON from an external URL via HTTP GET." }
func (t *HTTPFetchTool) Category() string    { return "native" }

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
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if input.URL == "" {
		return nil, errors.New("url parameter is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ActonOS-Agent/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size to 1MB
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return &ToolResult{
		Content: string(bodyBytes),
		Data: map[string]any{
			"status_code": resp.StatusCode,
			"url":         input.URL,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 2. File Read Tool
// -----------------------------------------------------------------------------

type FileReadTool struct {
	workspaceDir string
}

func NewFileReadTool(workspaceDir string) *FileReadTool {
	return &FileReadTool{workspaceDir: workspaceDir}
}

func (t *FileReadTool) Name() string        { return "native_file_read" }
func (t *FileReadTool) Description() string { return "Read contents of a file within the authorized workspace." }
func (t *FileReadTool) Category() string    { return "native" }

func (t *FileReadTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Relative path to file in workspace" }
		},
		"required": ["path"]
	}`)
}

func (t *FileReadTool) validatePath(relPath string) (string, error) {
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(relPath) {
		return "", ErrPathEscape
	}
	absWorkspace, _ := filepath.Abs(t.workspaceDir)
	targetPath := filepath.Join(absWorkspace, cleanRel)
	if !strings.HasPrefix(targetPath, absWorkspace) {
		return "", ErrPathEscape
	}
	return targetPath, nil
}

func (t *FileReadTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, err
	}

	targetPath, err := t.validatePath(input.Path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return &ToolResult{
		Content: string(data),
		Data: map[string]any{
			"bytes": len(data),
			"path":  input.Path,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 3. File Write Tool
// -----------------------------------------------------------------------------

type FileWriteTool struct {
	workspaceDir string
}

func NewFileWriteTool(workspaceDir string) *FileWriteTool {
	return &FileWriteTool{workspaceDir: workspaceDir}
}

func (t *FileWriteTool) Name() string        { return "native_file_write" }
func (t *FileWriteTool) Description() string { return "Write or overwrite a file within the authorized workspace." }
func (t *FileWriteTool) Category() string    { return "native" }

func (t *FileWriteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Relative path to file in workspace" },
			"content": { "type": "string", "description": "Text content to write into the file" }
		},
		"required": ["path", "content"]
	}`)
}

func (t *FileWriteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, err
	}

	cleanRel := filepath.Clean(input.Path)
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(input.Path) {
		return nil, ErrPathEscape
	}

	absWorkspace, _ := filepath.Abs(t.workspaceDir)
	targetPath := filepath.Join(absWorkspace, cleanRel)
	if !strings.HasPrefix(targetPath, absWorkspace) {
		return nil, ErrPathEscape
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("creating parent directory: %w", err)
	}

	if err := os.WriteFile(targetPath, []byte(input.Content), 0644); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path),
		Data: map[string]any{
			"path":  input.Path,
			"bytes": len(input.Content),
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 4. SysInfo Tool
// -----------------------------------------------------------------------------

type SysInfoTool struct{}

func NewSysInfoTool() *SysInfoTool { return &SysInfoTool{} }

func (t *SysInfoTool) Name() string        { return "native_sysinfo" }
func (t *SysInfoTool) Description() string { return "Get operating system, runtime, architecture, and current time information." }
func (t *SysInfoTool) Category() string    { return "native" }

func (t *SysInfoTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

func (t *SysInfoTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	info := map[string]any{
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"num_cpu":      runtime.NumCPU(),
		"goroutines":   runtime.NumGoroutine(),
		"alloc_mb":     float64(m.Alloc) / (1024 * 1024),
		"sys_mb":       float64(m.Sys) / (1024 * 1024),
		"current_time": time.Now().UTC().Format(time.RFC3339),
	}

	rawJSON, _ := json.MarshalIndent(info, "", "  ")
	return &ToolResult{
		Content: string(rawJSON),
		Data:    info,
	}, nil
}

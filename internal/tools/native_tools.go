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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/sandbox"
	"github.com/actonos/actonos/internal/security"
)

var (
	ErrPathEscape = errors.New("access denied: path escapes allowed workspace directory")
)

// CronSchedulerProvider defines interface for managing background cron schedules.
type CronSchedulerProvider interface {
	RegisterCron(id, agentID, cronExpr, prompt, targetChannel, targetAccountID, targetRecipient string) error
	RemoveCron(id string)
	ListCrons() []map[string]any
}

// RegisterNativeTools adds default native tools to the registry.
func RegisterNativeTools(r *ToolRegistry, workspaceDir string) {
	if workspaceDir == "" {
		workspaceDir = "./data/workspace"
	}
	_ = r.Register(NewHTTPFetchTool())
	_ = r.Register(NewFileReadTool(workspaceDir))
	_ = r.Register(NewFileWriteTool(workspaceDir))
	_ = r.Register(NewFileListTool(workspaceDir))
	_ = r.Register(NewFileDeleteTool(workspaceDir))
	_ = r.Register(NewFileSearchTool(workspaceDir))
	_ = r.Register(NewExecTool(workspaceDir))
	_ = r.Register(NewWebSearchTool())
	_ = r.Register(NewChannelNotifyTool(r.bus))
	dataDir := filepath.Dir(workspaceDir)
	_ = r.Register(NewSysInfoTool(dataDir))
	_ = r.Register(NewBrowserNavigateTool())
	_ = r.Register(NewBrowserScreenshotTool(workspaceDir))
	_ = r.Register(NewCronScheduleTool(nil))
}

// AttachCronScheduler connects the active CronScheduler to native_cron_schedule.
func AttachCronScheduler(r *ToolRegistry, scheduler CronSchedulerProvider) {
	if t, err := r.Get("native_cron_schedule"); err == nil {
		if ct, ok := t.(*CronScheduleTool); ok {
			ct.SetScheduler(scheduler)
		}
	} else {
		_ = r.Register(NewCronScheduleTool(scheduler))
	}
}

// -----------------------------------------------------------------------------
// 1. HTTP Fetch Tool
// -----------------------------------------------------------------------------

type HTTPFetchTool struct {
	client *http.Client
}

func NewHTTPFetchTool() *HTTPFetchTool {
	return &HTTPFetchTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return security.ValidateOutboundURL(req.Context(), req.URL.String())
			},
		},
	}
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

func (t *FileReadTool) Name() string { return "native_file_read" }
func (t *FileReadTool) Description() string {
	return "Read contents of a file within the authorized workspace."
}
func (t *FileReadTool) Category() string { return "native" }

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
	targetPath, err := security.ResolvePath(t.workspaceDir, relPath, false)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPathEscape, err)
	}
	return targetPath, nil
}

func (t *FileReadTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Path == "" {
		var strPath string
		if strErr := json.Unmarshal(inputJSON, &strPath); strErr == nil && strPath != "" {
			input.Path = strPath
		} else {
			var m map[string]any
			if mapErr := json.Unmarshal(inputJSON, &m); mapErr == nil {
				if p, ok := m["path"].(string); ok && p != "" {
					input.Path = p
				}
			}
		}
	}

	if input.Path == "" {
		return nil, errors.New("path parameter is required")
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

func (t *FileWriteTool) Name() string { return "native_file_write" }
func (t *FileWriteTool) Description() string {
	return "Write or overwrite a file within the authorized workspace."
}
func (t *FileWriteTool) Category() string { return "native" }

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
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Path == "" {
		var m map[string]any
		if mapErr := json.Unmarshal(inputJSON, &m); mapErr == nil {
			if p, ok := m["path"].(string); ok {
				input.Path = p
			}
			if c, ok := m["content"].(string); ok {
				input.Content = c
			}
		}
	}

	if input.Path == "" {
		return nil, errors.New("path parameter is required")
	}

	targetPath, err := security.ResolvePath(t.workspaceDir, input.Path, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
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
// 4. File List Tool
// -----------------------------------------------------------------------------

type FileListTool struct {
	workspaceDir string
}

func NewFileListTool(workspaceDir string) *FileListTool {
	return &FileListTool{workspaceDir: workspaceDir}
}

func (t *FileListTool) Name() string { return "native_file_list" }
func (t *FileListTool) Description() string {
	return "List files and directories within the authorized workspace."
}
func (t *FileListTool) Category() string { return "native" }

func (t *FileListTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Relative subdirectory in workspace (optional, default root '')" },
			"recursive": { "type": "boolean", "description": "Whether to list subdirectories recursively (default false)" }
		}
	}`)
}

func (t *FileListTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	_ = json.Unmarshal(inputJSON, &input)

	targetDir, err := security.ResolvePath(t.workspaceDir, input.Path, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}
	absWorkspace, _ := filepath.Abs(t.workspaceDir)

	_ = os.MkdirAll(targetDir, 0755)

	type FileEntry struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		IsDir   bool   `json:"is_dir"`
		Size    int64  `json:"size"`
		ModTime string `json:"mod_time"`
	}

	var entries []FileEntry
	if input.Recursive {
		err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || path == targetDir {
				return nil
			}
			rel, _ := filepath.Rel(absWorkspace, path)
			entries = append(entries, FileEntry{
				Name:    info.Name(),
				Path:    rel,
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime().Format(time.RFC3339),
			})
			if len(entries) >= 200 {
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		dirEntries, err := os.ReadDir(targetDir)
		if err != nil {
			return nil, err
		}
		for _, de := range dirEntries {
			info, _ := de.Info()
			size := int64(0)
			modTime := ""
			if info != nil {
				size = info.Size()
				modTime = info.ModTime().Format(time.RFC3339)
			}
			rel, _ := filepath.Rel(absWorkspace, filepath.Join(targetDir, de.Name()))
			entries = append(entries, FileEntry{
				Name:    de.Name(),
				Path:    rel,
				IsDir:   de.IsDir(),
				Size:    size,
				ModTime: modTime,
			})
		}
	}

	rawJSON, _ := json.MarshalIndent(entries, "", "  ")
	return &ToolResult{
		Content: fmt.Sprintf("Found %d file(s) in %s:\n%s", len(entries), input.Path, string(rawJSON)),
		Data:    map[string]any{"count": len(entries), "files": entries},
	}, nil
}

// -----------------------------------------------------------------------------
// 5. File Delete Tool
// -----------------------------------------------------------------------------

type FileDeleteTool struct {
	workspaceDir string
}

func NewFileDeleteTool(workspaceDir string) *FileDeleteTool {
	return &FileDeleteTool{workspaceDir: workspaceDir}
}

func (t *FileDeleteTool) Name() string { return "native_file_delete" }
func (t *FileDeleteTool) Description() string {
	return "Delete a file or empty directory in the authorized workspace."
}
func (t *FileDeleteTool) Category() string { return "native" }

func (t *FileDeleteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Relative path of file to delete" }
		},
		"required": ["path"]
	}`)
}

func (t *FileDeleteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Path == "" {
		return nil, errors.New("path is required")
	}

	cleanRel := filepath.Clean(input.Path)
	if cleanRel == "." || cleanRel == "" {
		return nil, ErrPathEscape
	}
	targetPath, err := security.ResolvePath(t.workspaceDir, input.Path, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}

	if err := os.Remove(targetPath); err != nil {
		return nil, fmt.Errorf("deleting file: %w", err)
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully deleted %s", input.Path),
		Data:    map[string]any{"deleted": input.Path},
	}, nil
}

// -----------------------------------------------------------------------------
// 6. File Search Tool (Grep / Find)
// -----------------------------------------------------------------------------

type FileSearchTool struct {
	workspaceDir string
}

func NewFileSearchTool(workspaceDir string) *FileSearchTool {
	return &FileSearchTool{workspaceDir: workspaceDir}
}

func (t *FileSearchTool) Name() string { return "native_file_search" }
func (t *FileSearchTool) Description() string {
	return "Search for text patterns or filenames inside the workspace."
}
func (t *FileSearchTool) Category() string { return "native" }

func (t *FileSearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": { "type": "string", "description": "Text query or keyword to search for" },
			"extension": { "type": "string", "description": "Filter by file extension (e.g. '.md', '.go', '.json')" }
		},
		"required": ["query"]
	}`)
}

func (t *FileSearchTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Query     string `json:"query"`
		Extension string `json:"extension"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Query == "" {
		return nil, errors.New("query is required")
	}

	absWorkspace, _ := filepath.Abs(t.workspaceDir)
	queryLower := strings.ToLower(input.Query)

	type SearchMatch struct {
		Path    string `json:"path"`
		LineNum int    `json:"line_num,omitempty"`
		Snippet string `json:"snippet"`
	}

	var matches []SearchMatch
	_ = filepath.Walk(absWorkspace, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if input.Extension != "" && !strings.HasSuffix(strings.ToLower(path), strings.ToLower(input.Extension)) {
			return nil
		}

		rel, _ := filepath.Rel(absWorkspace, path)
		// Check filename match
		if strings.Contains(strings.ToLower(info.Name()), queryLower) {
			matches = append(matches, SearchMatch{
				Path:    rel,
				Snippet: fmt.Sprintf("[Filename Match] %s", info.Name()),
			})
		}

		// Search file contents (up to 512KB per file)
		if info.Size() < 512*1024 {
			data, err := os.ReadFile(path)
			if err == nil {
				lines := strings.Split(string(data), "\n")
				for idx, line := range lines {
					if strings.Contains(strings.ToLower(line), queryLower) {
						snippet := strings.TrimSpace(line)
						if len(snippet) > 120 {
							snippet = snippet[:120] + "..."
						}
						matches = append(matches, SearchMatch{
							Path:    rel,
							LineNum: idx + 1,
							Snippet: snippet,
						})
						if len(matches) >= 50 {
							return filepath.SkipAll
						}
					}
				}
			}
		}
		return nil
	})

	rawJSON, _ := json.MarshalIndent(matches, "", "  ")
	return &ToolResult{
		Content: fmt.Sprintf("Search for '%s' found %d match(es):\n%s", input.Query, len(matches), string(rawJSON)),
		Data:    map[string]any{"query": input.Query, "count": len(matches), "matches": matches},
	}, nil
}

// -----------------------------------------------------------------------------
// 7. Exec Tool (Sandboxed Command Execution)
// -----------------------------------------------------------------------------

type ExecTool struct {
	workspaceDir string
}

func NewExecTool(workspaceDir string) *ExecTool {
	return &ExecTool{workspaceDir: workspaceDir}
}

func (t *ExecTool) Name() string { return "native_exec" }
func (t *ExecTool) Description() string {
	return "Execute a shell or PowerShell command inside the sandboxed workspace directory (timeout: 60s, max memory: 512MB)."
}
func (t *ExecTool) Category() string { return "native" }

func (t *ExecTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": { "type": "string", "description": "The shell command to execute" },
			"timeout_seconds": { "type": "integer", "description": "Execution timeout in seconds (default: 60)" }
		},
		"required": ["command"]
	}`)
}

func (t *ExecTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Command == "" {
		return nil, errors.New("command is required")
	}

	timeout := 60 * time.Second
	if input.TimeoutSeconds > 0 && input.TimeoutSeconds <= 300 {
		timeout = time.Duration(input.TimeoutSeconds) * time.Second
	}

	sb := sandbox.AutoDetectSandbox()
	result, err := sb.Execute(ctx, sandbox.CommandRequest{
		Command:      input.Command,
		WorkspaceDir: t.workspaceDir,
		Timeout:      timeout,
		MaxMemoryMB:  512,
		MaxProcesses: 30,
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox execution error: %w", err)
	}
	if result.ExitCode != 0 {
		return &ToolResult{
			Content: result.Stdout,
			Data: map[string]any{
				"exit_code":      result.ExitCode,
				"stderr":         result.Stderr,
				"execution_time": result.ExecutionTime.String(),
				"killed":         result.Killed,
			},
			Error: result.Stderr,
		}, fmt.Errorf("command exited with code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	output := result.Stdout
	if result.Stderr != "" {
		if output != "" {
			output += "\n[STDERR]\n" + result.Stderr
		} else {
			output = result.Stderr
		}
	}
	if output == "" {
		output = fmt.Sprintf("(Command completed with exit code %d, no output)", result.ExitCode)
	}

	return &ToolResult{
		Content: output,
		Data: map[string]any{
			"exit_code":      result.ExitCode,
			"execution_time": result.ExecutionTime.String(),
			"killed":         result.Killed,
		},
	}, nil
}

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

	rawJSON, _ := json.MarshalIndent(results, "", "  ")
	return &ToolResult{
		Content: fmt.Sprintf("Web Search Results for '%s':\n%s", input.Query, string(rawJSON)),
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
		AbstractText string `json:"AbstractText"`
		AbstractURL  string `json:"AbstractURL"`
		Heading      string `json:"Heading"`
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

// -----------------------------------------------------------------------------
// 9. Channel Notify Tool (Proactive Message Dispatch)
// -----------------------------------------------------------------------------

type ChannelNotifyTool struct {
	bus *bus.EventBus
}

func NewChannelNotifyTool(eventBus *bus.EventBus) *ChannelNotifyTool {
	return &ChannelNotifyTool{bus: eventBus}
}

func (t *ChannelNotifyTool) Name() string { return "native_channel_notify" }
func (t *ChannelNotifyTool) Description() string {
	return "Proactively send a message or status update to the user via Telegram, WhatsApp, Discord, or Web."
}
func (t *ChannelNotifyTool) Category() string { return "native" }

func (t *ChannelNotifyTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": { "type": "string", "enum": ["telegram", "whatsapp", "discord", "all"], "description": "Target channel (default 'telegram')" },
			"account_id": { "type": "string", "description": "Target account ID or 'all' (default 'all')" },
			"recipient": { "type": "string", "description": "Optional recipient chat ID or phone number" },
			"message": { "type": "string", "description": "Message content to send" }
		},
		"required": ["message"]
	}`)
}

func (t *ChannelNotifyTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Channel   string `json:"channel"`
		AccountID string `json:"account_id"`
		Recipient string `json:"recipient"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Message == "" {
		return nil, errors.New("message parameter is required")
	}
	if input.Channel == "" {
		input.Channel = "telegram"
	}
	if input.AccountID == "" {
		input.AccountID = "all"
	}

	if t.bus != nil {
		t.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, "channel_notify", map[string]any{
			"type":              "proactive_cron_notification",
			"job_name":          "Direct Agent Notification",
			"content":           input.Message,
			"target_channel":    input.Channel,
			"target_account_id": input.AccountID,
			"target_recipient":  input.Recipient,
		}))
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully dispatched proactive notification to channel '%s' (account: %s)", input.Channel, input.AccountID),
		Data: map[string]any{
			"channel":    input.Channel,
			"account_id": input.AccountID,
			"recipient":  input.Recipient,
			"status":     "dispatched",
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 10. SysInfo Tool
// -----------------------------------------------------------------------------

type SysInfoTool struct {
	dataDir string
}

func NewSysInfoTool(dataDir ...string) *SysInfoTool {
	d := "./data"
	if len(dataDir) > 0 && dataDir[0] != "" {
		d = dataDir[0]
	}
	return &SysInfoTool{dataDir: d}
}

func (t *SysInfoTool) Name() string { return "native_sysinfo" }
func (t *SysInfoTool) Description() string {
	return "Get operating system metrics, memory allocation, CPU load, SQLite database & WAL status, vector memory indexes, and local storage health."
}
func (t *SysInfoTool) Category() string { return "native" }

func (t *SysInfoTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

func (t *SysInfoTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	dbPath := filepath.Join(t.dataDir, "storage", "acton.db")
	walPath := filepath.Join(t.dataDir, "storage", "acton.db-wal")
	vectorsPath := filepath.Join(t.dataDir, "vectors")
	workspacePath := filepath.Join(t.dataDir, "workspace")

	dbStatus := "not_found"
	var dbSize int64
	if fi, err := os.Stat(dbPath); err == nil {
		dbStatus = "online_wal_mode"
		dbSize = fi.Size()
	}

	walStatus := "clean_checkpointed"
	var walSize int64
	if fi, err := os.Stat(walPath); err == nil {
		walStatus = "active_transactions"
		walSize = fi.Size()
	}

	vectorStatus := "initialized"
	if _, err := os.Stat(vectorsPath); os.IsNotExist(err) {
		_ = os.MkdirAll(vectorsPath, 0755)
	}

	info := map[string]any{
		"system_health": "nominal_healthy",
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"num_cpu":       runtime.NumCPU(),
		"goroutines":    runtime.NumGoroutine(),
		"memory": map[string]any{
			"alloc_mb": float64(m.Alloc) / (1024 * 1024),
			"sys_mb":   float64(m.Sys) / (1024 * 1024),
		},
		"storage_and_memory_components": map[string]any{
			"sqlite_database": map[string]any{
				"status":  dbStatus,
				"path":    dbPath,
				"size_kb": dbSize / 1024,
			},
			"sqlite_wal_checkpoint": map[string]any{
				"status":  walStatus,
				"path":    walPath,
				"size_kb": walSize / 1024,
			},
			"vector_memory_indexes": map[string]any{
				"status": vectorStatus,
				"engine": "chromem-go",
				"path":   vectorsPath,
			},
			"workspace": map[string]any{
				"status": "online",
				"path":   workspacePath,
			},
		},
		"current_time": time.Now().UTC().Format(time.RFC3339),
	}

	rawJSON, _ := json.MarshalIndent(info, "", "  ")
	return &ToolResult{
		Content: string(rawJSON),
		Data:    info,
	}, nil
}

// -----------------------------------------------------------------------------
// 11. Cron Schedule Tool
// -----------------------------------------------------------------------------

type CronScheduleTool struct {
	scheduler CronSchedulerProvider
}

func NewCronScheduleTool(scheduler CronSchedulerProvider) *CronScheduleTool {
	return &CronScheduleTool{scheduler: scheduler}
}

func (t *CronScheduleTool) SetScheduler(scheduler CronSchedulerProvider) {
	t.scheduler = scheduler
}

func (t *CronScheduleTool) Name() string { return "native_cron_schedule" }
func (t *CronScheduleTool) Description() string {
	return "Schedule, list, or delete autonomous recurring cron tasks (e.g. '0 8 * * *' for daily 8 AM tasks, '*/30 * * * *' for every 30m)."
}
func (t *CronScheduleTool) Category() string { return "native" }

func (t *CronScheduleTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": { "type": "string", "enum": ["create", "list", "delete"], "description": "Action to execute: create, list, or delete" },
			"job_id": { "type": "string", "description": "Unique identifier for the job (e.g. daily_briefing, meeting_reminder)" },
			"agent_id": { "type": "string", "description": "Target agent ID (defaults to 'agent_system_core')" },
			"cron_expression": { "type": "string", "description": "Standard 5-part cron expression (e.g. '0 8 * * *' for 8:00 AM daily, '0 10 17 8 *' for 10:00 AM on August 17th)" },
			"prompt": { "type": "string", "description": "Instructions/prompt that the agent will run autonomously on schedule to generate the reminder content" },
			"target_channel": { "type": "string", "description": "Outbound channel to deliver the notification (e.g. 'telegram', 'whatsapp', 'all'). Default: 'telegram'" },
			"target_account_id": { "type": "string", "description": "Target channel account ID or 'all' (default 'all')" },
			"target_recipient": { "type": "string", "description": "Destination chat ID or phone number (optional, will automatically route to the user's active channel if omitted)" }
		},
		"required": ["action"]
	}`)
}

func (t *CronScheduleTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Action          string `json:"action"`
		JobID           string `json:"job_id"`
		AgentID         string `json:"agent_id"`
		CronExpression  string `json:"cron_expression"`
		Prompt          string `json:"prompt"`
		TargetChannel   string `json:"target_channel"`
		TargetAccountID string `json:"target_account_id"`
		TargetRecipient string `json:"target_recipient"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if t.scheduler == nil {
		return nil, errors.New("cron scheduler provider is not attached to this tool")
	}

	switch strings.ToLower(input.Action) {
	case "list":
		jobs := t.scheduler.ListCrons()
		data, _ := json.MarshalIndent(jobs, "", "  ")
		return &ToolResult{
			Content: fmt.Sprintf("Found %d scheduled cron tasks:\n%s", len(jobs), string(data)),
			Data:    map[string]any{"jobs": jobs, "count": len(jobs)},
		}, nil

	case "delete":
		if input.JobID == "" {
			return nil, errors.New("job_id is required to delete a cron task")
		}
		t.scheduler.RemoveCron(input.JobID)
		return &ToolResult{
			Content: fmt.Sprintf("Successfully removed scheduled cron task '%s'", input.JobID),
			Data:    map[string]any{"job_id": input.JobID, "status": "deleted"},
		}, nil

	case "create":
		if input.AgentID == "" {
			input.AgentID = "agent_system_core"
		}
		if input.JobID == "" {
			input.JobID = fmt.Sprintf("job_%d", time.Now().Unix())
		}
		if input.CronExpression == "" {
			return nil, errors.New("cron_expression is required (e.g. '0 8 * * *')")
		}
		if input.Prompt == "" {
			return nil, errors.New("prompt is required")
		}
		if input.TargetChannel == "" {
			input.TargetChannel = "telegram"
		}
		if input.TargetAccountID == "" {
			input.TargetAccountID = "all"
		}

		if err := t.scheduler.RegisterCron(input.JobID, input.AgentID, input.CronExpression, input.Prompt, input.TargetChannel, input.TargetAccountID, input.TargetRecipient); err != nil {
			return nil, fmt.Errorf("registering cron schedule: %w", err)
		}

		return &ToolResult{
			Content: fmt.Sprintf("Successfully registered scheduled reminder '%s'\nCron: %s\nTarget Channel: %s\nTarget Account: %s\nPrompt: %s", input.JobID, input.CronExpression, input.TargetChannel, input.TargetAccountID, input.Prompt),
			Data: map[string]any{
				"job_id":            input.JobID,
				"cron_expression":   input.CronExpression,
				"agent_id":          input.AgentID,
				"prompt":            input.Prompt,
				"target_channel":    input.TargetChannel,
				"target_account_id": input.TargetAccountID,
				"target_recipient":  input.TargetRecipient,
				"status":            "created",
			},
		}, nil

	default:
		return nil, fmt.Errorf("invalid action '%s', supported: create, list, delete", input.Action)
	}
}

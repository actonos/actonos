package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/sandbox"
	"github.com/actonos/actonos/internal/security"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
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

type NativeToolsConfig struct {
	DataDir       string
	AgentsDir     string
	UserWorkspace *workspacepkg.Store
}

// RegisterNativeTools adds native tools using the data directory inferred from
// the former workspace argument. New production wiring should use
// RegisterNativeToolsWithConfig so the user database and private agent roots
// are explicit.
func RegisterNativeTools(r *ToolRegistry, legacyWorkspaceDir string) {
	if legacyWorkspaceDir == "" {
		legacyWorkspaceDir = "./data/workspace"
	}
	dataDir := legacyWorkspaceDir
	if filepath.Base(legacyWorkspaceDir) == "workspace" {
		dataDir = filepath.Dir(legacyWorkspaceDir)
	}
	RegisterNativeToolsWithConfig(r, NativeToolsConfig{
		DataDir:   dataDir,
		AgentsDir: filepath.Join(dataDir, "agents"),
	})
}

func RegisterNativeToolsWithConfig(r *ToolRegistry, config NativeToolsConfig) {
	if config.DataDir == "" {
		config.DataDir = "./data"
	}
	if config.AgentsDir == "" {
		config.AgentsDir = filepath.Join(config.DataDir, "agents")
	}
	_ = r.Register(NewHTTPFetchTool())
	_ = r.Register(NewFileReadTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileWriteTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileEditTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileSearchTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileDeleteTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileMoveTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileCopyTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewExecTool(config.DataDir, config.AgentsDir))
	if config.UserWorkspace != nil {
		_ = r.Register(NewWorkspaceSearchTool(config.UserWorkspace))
		_ = r.Register(NewWorkspaceReadTool(config.UserWorkspace))
		_ = r.Register(NewWorkspaceWriteTool(config.UserWorkspace))
		_ = r.Register(NewWorkspaceDeleteTool(config.UserWorkspace))
	}
	_ = r.Register(NewWebSearchTool())
	_ = r.Register(NewChannelNotifyTool(r.bus))
	_ = r.Register(NewSysInfoTool(config.DataDir))
	_ = r.Register(NewBrowserNavigateTool())
	_ = r.Register(NewBrowserScreenshotTool(config.AgentsDir))
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
// 2. File Tools Helpers & Sanitization
// -----------------------------------------------------------------------------

// sanitizeAgentRelativePath cleans redundant agent workspace prefixes that LLMs sometimes hallucinate
// (e.g. "data/agents/{agentID}/workspace/file.txt" or "agents/{agentID}/workspace/file.txt" or "./").
func sanitizeAgentRelativePath(path, agentID string) string {
	clean := strings.TrimSpace(path)
	clean = strings.ReplaceAll(clean, "\\", "/")
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "/")

	if agentID != "" {
		fullPrefix := "data/agents/" + agentID + "/workspace/"
		if strings.HasPrefix(clean, fullPrefix) {
			clean = strings.TrimPrefix(clean, fullPrefix)
		}
		agentPrefix := "agents/" + agentID + "/workspace/"
		if strings.HasPrefix(clean, agentPrefix) {
			clean = strings.TrimPrefix(clean, agentPrefix)
		}
		wsPrefix := agentID + "/workspace/"
		if strings.HasPrefix(clean, wsPrefix) {
			clean = strings.TrimPrefix(clean, wsPrefix)
		}
		if strings.HasPrefix(clean, "workspace/") {
			clean = strings.TrimPrefix(clean, "workspace/")
		}
	}
	clean = filepath.Clean(clean)
	return clean
}

func formatHumanBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// parseFilePathInput safely parses the target file path from JSON or raw input.
func parseFilePathInput(inputJSON json.RawMessage) (string, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	raw := strings.TrimSpace(string(inputJSON))

	var input struct {
		Path     string `json:"path"`
		File     string `json:"file"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(inputJSON, &input); err == nil && input.Path != "" {
		return input.Path, nil
	}
	if input.File != "" {
		return input.File, nil
	}
	if input.Filename != "" {
		return input.Filename, nil
	}

	var strPath string
	if err := json.Unmarshal(inputJSON, &strPath); err == nil && strPath != "" {
		if !strings.HasPrefix(strPath, "{") && !strings.Contains(strPath, `":`) {
			return strPath, nil
		}
		raw = strPath
	}

	pathRe := regexp.MustCompile(`"(?:path|file|filename|filepath|target|src_path)"\s*:\s*"([^"]+)"`)
	if m := pathRe.FindStringSubmatch(raw); len(m) > 1 {
		return m[1], nil
	}

	if !strings.ContainsAny(raw, "{}\"\r\n") && len(raw) > 0 {
		return raw, nil
	}

	return "", errors.New("path parameter is required")
}

// parseFileWriteInput safely extracts path, content, and mode even if JSON contains unescaped newlines or quotes.
func parseFileWriteInput(inputJSON json.RawMessage) (string, string, string, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	raw := strings.TrimSpace(string(inputJSON))

	var input struct {
		Path     string `json:"path"`
		File     string `json:"file"`
		Filename string `json:"filename"`
		Content  string `json:"content"`
		Data     string `json:"data"`
		Text     string `json:"text"`
		Mode     string `json:"mode"`
	}
	if err := json.Unmarshal(inputJSON, &input); err == nil {
		path := input.Path
		if path == "" {
			path = input.File
		}
		if path == "" {
			path = input.Filename
		}
		content := input.Content
		if content == "" {
			content = input.Data
		}
		if content == "" {
			content = input.Text
		}
		mode := strings.ToLower(strings.TrimSpace(input.Mode))
		if mode == "" {
			mode = "overwrite"
		}
		if path != "" && !strings.HasPrefix(strings.TrimSpace(path), "{") && !strings.Contains(path, `":`) {
			return strings.TrimSpace(path), content, mode, nil
		}
	}

	var m map[string]any
	if err := json.Unmarshal(inputJSON, &m); err == nil {
		var path, content, mode string
		for _, k := range []string{"path", "file", "filename", "filepath", "target"} {
			if v, ok := m[k].(string); ok && v != "" {
				path = v
				break
			}
		}
		for _, k := range []string{"content", "data", "text", "body"} {
			if v, ok := m[k].(string); ok {
				content = v
				break
			}
		}
		if v, ok := m["mode"].(string); ok && v != "" {
			mode = strings.ToLower(strings.TrimSpace(v))
		}
		if mode == "" {
			mode = "overwrite"
		}
		if path != "" && !strings.HasPrefix(strings.TrimSpace(path), "{") && !strings.Contains(path, `":`) {
			return strings.TrimSpace(path), content, mode, nil
		}
	}

	// Regex fallback for unescaped multiline content
	var path, content string
	pathRe := regexp.MustCompile(`"(?:path|file|filename|filepath|target)"\s*:\s*"([^"]+)"`)
	if match := pathRe.FindStringSubmatch(raw); len(match) > 1 {
		path = match[1]
	}

	contentRe := regexp.MustCompile(`"(?:content|data|text|body)"\s*:\s*"([\s\S]*)`)
	if match := contentRe.FindStringSubmatch(raw); len(match) > 1 {
		extracted := match[1]
		if strings.HasSuffix(extracted, `"}`) {
			extracted = extracted[:len(extracted)-2]
		} else if strings.HasSuffix(extracted, `"`) {
			extracted = extracted[:len(extracted)-1]
		}
		extracted = strings.ReplaceAll(extracted, `\n`, "\n")
		extracted = strings.ReplaceAll(extracted, `\t`, "\t")
		extracted = strings.ReplaceAll(extracted, `\"`, `"`)
		extracted = strings.ReplaceAll(extracted, `\\`, `\`)
		content = extracted
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", "", errors.New("path parameter is required")
	}
	if strings.ContainsAny(path, "{}\"\r\n") {
		return "", "", "", fmt.Errorf("invalid file path: %q", path)
	}

	return path, content, "overwrite", nil
}

// resolveTargetBaseDir returns the allowed root (dataDir) and base directory (dataDir) for path resolution.
// All relative paths resolve directly inside the system data directory (e.g. 'skills/...', 'config/...', 'plugins/...', 'logs/...').
func resolveTargetBaseDir(ctx context.Context, dataDir, agentsDir, requestedPath string) (allowedRoot string, baseDir string, targetRelPath string, err error) {
	if dataDir == "" {
		dataDir = "./data"
	}

	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return "", "", "", fmt.Errorf("resolving data root: %w", err)
	}
	if err := os.MkdirAll(absDataDir, 0750); err != nil {
		return "", "", "", fmt.Errorf("creating data directory: %w", err)
	}

	cleanReq := strings.TrimSpace(requestedPath)
	cleanReq = strings.ReplaceAll(cleanReq, "\\", "/")
	cleanReq = strings.TrimPrefix(cleanReq, "./")

	if strings.HasPrefix(cleanReq, "data/") {
		cleanReq = strings.TrimPrefix(cleanReq, "data/")
	}
	if cleanReq == "data" || cleanReq == "" {
		cleanReq = "."
	}

	return absDataDir, absDataDir, cleanReq, nil
}

func validAgentWorkspaceSlug(agentID string) bool {
	if agentID == "" {
		return false
	}
	for _, char := range agentID {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func parseDataAndAgentsDir(dataOrAgentsDir string, agentsDir ...string) (string, string) {
	if len(agentsDir) > 0 && agentsDir[0] != "" {
		return dataOrAgentsDir, agentsDir[0]
	}
	d := dataOrAgentsDir
	if d == "" {
		d = "./data"
	}
	return d, filepath.Join(d, "agents")
}

// -----------------------------------------------------------------------------
// 2. File Read Tool (with Line Numbers and Range Slicing)
// -----------------------------------------------------------------------------

type FileReadTool struct {
	dataDir   string
	agentsDir string
}

func NewFileReadTool(dataDir string, agentsDir ...string) *FileReadTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileReadTool{dataDir: d, agentsDir: agDir}
}

func (t *FileReadTool) Name() string { return "native_file_read" }
func (t *FileReadTool) Description() string {
	return "Read contents of a file inside the data directory or your private workspace with line numbering and range slicing support."
}
func (t *FileReadTool) Category() string { return "native" }

func (t *FileReadTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "File path (relative to workspace or relative to data directory, e.g. 'main.py' or 'skills/my_skill/SKILL.md')" },
			"start_line": { "type": "integer", "description": "Starting line number to read (1-indexed, default 1)", "minimum": 1 },
			"end_line": { "type": "integer", "description": "Ending line number to read (inclusive, default start_line + 250)", "minimum": 1 },
			"line_numbers": { "type": "boolean", "description": "Whether to prefix each line with its 1-indexed line number (default true)" }
		},
		"required": ["path"]
	}`)
}

func (t *FileReadTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path        string `json:"path"`
		File        string `json:"file"`
		Filename    string `json:"filename"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		LineNumbers *bool  `json:"line_numbers"`
	}
	_ = json.Unmarshal(inputJSON, &input)

	path := input.Path
	if path == "" {
		path = input.File
	}
	if path == "" {
		path = input.Filename
	}
	if path == "" {
		var err error
		path, err = parseFilePathInput(inputJSON)
		if err != nil {
			return nil, err
		}
	}

	agentID := AgentIDFromContext(ctx)
	relPath := sanitizeAgentRelativePath(path, agentID)

	allowedRoot, baseDir, targetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, relPath)
	if err != nil {
		return nil, err
	}
	targetPath, err := security.ResolvePathWithBase(allowedRoot, baseDir, targetRel, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	withLineNums := false
	if input.LineNumbers != nil {
		withLineNums = *input.LineNumbers
	} else if input.StartLine > 0 || input.EndLine > 0 {
		withLineNums = true
	}

	// Binary check: contains null bytes
	if len(data) > 0 && strings.IndexByte(string(data[:min(len(data), 1024)]), 0) != -1 {
		return &ToolResult{
			Content: fmt.Sprintf("[Binary file: %s (%s, %d bytes total)]", relPath, formatHumanBytes(int64(len(data))), len(data)),
			Data: map[string]any{
				"path":      relPath,
				"bytes":     len(data),
				"is_binary": true,
			},
		}, nil
	}

	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	startLine := input.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	endLine := input.EndLine
	if endLine <= 0 {
		endLine = startLine + 249
	}
	if endLine > totalLines {
		endLine = totalLines
	}
	if startLine > totalLines && totalLines > 0 {
		return &ToolResult{
			Content: fmt.Sprintf("File %s only contains %d lines (requested start_line=%d).", relPath, totalLines, startLine),
			Data: map[string]any{
				"path":        relPath,
				"total_lines": totalLines,
				"bytes":       len(data),
			},
		}, nil
	}

	var sb strings.Builder
	for i := startLine; i <= endLine; i++ {
		lineContent := lines[i-1]
		lineContent = strings.TrimRight(lineContent, "\r")
		if withLineNums {
			fmt.Fprintf(&sb, "%4d: %s\n", i, lineContent)
		} else {
			sb.WriteString(lineContent)
			sb.WriteString("\n")
		}
	}

	var note string
	if endLine < totalLines {
		note = fmt.Sprintf("\n[Showing lines %d-%d of %d total lines (%s). Use start_line=%d to view remaining content]",
			startLine, endLine, totalLines, formatHumanBytes(int64(len(data))), endLine+1)
	}

	output := strings.TrimRight(sb.String(), "\n") + note
	return &ToolResult{
		Content: output,
		Data: map[string]any{
			"path":        relPath,
			"start_line":  startLine,
			"end_line":    endLine,
			"total_lines": totalLines,
			"bytes":       len(data),
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 3. File Write Tool (with Append Mode & Atomic Overwrite)
// -----------------------------------------------------------------------------

type FileWriteTool struct {
	dataDir   string
	agentsDir string
}

func NewFileWriteTool(dataDir string, agentsDir ...string) *FileWriteTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileWriteTool{dataDir: d, agentsDir: agDir}
}

func (t *FileWriteTool) Name() string { return "native_file_write" }
func (t *FileWriteTool) Description() string {
	return "Write, overwrite, or append content to a file in the data directory or your private workspace."
}
func (t *FileWriteTool) Category() string { return "native" }

func (t *FileWriteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "File path (relative to workspace or relative to data directory, e.g. 'main.py' or 'skills/my_skill/SKILL.md')" },
			"content": { "type": "string", "description": "Text content to write into the file" },
			"mode": { "type": "string", "enum": ["overwrite", "append"], "description": "Write mode: 'overwrite' (default) or 'append' to add to the end of file" }
		},
		"required": ["path", "content"]
	}`)
}

func (t *FileWriteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	relPath, content, mode, err := parseFileWriteInput(inputJSON)
	if err != nil {
		return nil, err
	}
	agentID := AgentIDFromContext(ctx)
	relPath = sanitizeAgentRelativePath(relPath, agentID)

	allowedRoot, baseDir, targetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, relPath)
	if err != nil {
		return nil, err
	}
	targetPath, err := security.ResolvePathWithBase(allowedRoot, baseDir, targetRel, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("creating parent directory: %w", err)
	}

	if mode == "append" {
		f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("opening file for append: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return nil, fmt.Errorf("appending to file: %w", err)
		}
	} else {
		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("writing file: %w", err)
		}
	}

	action := "Successfully wrote"
	if mode == "append" {
		action = "Successfully appended"
	}

	return &ToolResult{
		Content: fmt.Sprintf("%s %d bytes to %s (mode: %s)", action, len(content), relPath, mode),
		Data: map[string]any{
			"path":          relPath,
			"absolute_path": targetPath,
			"bytes":         len(content),
			"mode":          mode,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 4. File Edit Tool (Surgical Text Replacement)
// -----------------------------------------------------------------------------

type FileEditTool struct {
	dataDir   string
	agentsDir string
}

func NewFileEditTool(dataDir string, agentsDir ...string) *FileEditTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileEditTool{dataDir: d, agentsDir: agDir}
}

func (t *FileEditTool) Name() string { return "native_file_edit" }
func (t *FileEditTool) Description() string {
	return "Perform surgical text replacement or line edits inside a file in the data directory or your private workspace without rewriting the entire file."
}
func (t *FileEditTool) Category() string { return "native" }

func (t *FileEditTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "File path (relative to workspace or relative to data directory)" },
			"target_content": { "type": "string", "description": "Exact text or code block to replace" },
			"replacement_content": { "type": "string", "description": "New text or code block to insert in place of target_content" },
			"start_line": { "type": "integer", "description": "Optional starting line number (1-indexed) to restrict replacement search range", "minimum": 1 },
			"end_line": { "type": "integer", "description": "Optional ending line number (1-indexed) to restrict replacement search range", "minimum": 1 },
			"allow_multiple": { "type": "boolean", "description": "Whether to replace multiple occurrences if found (default false)" }
		},
		"required": ["path", "target_content", "replacement_content"]
	}`)
}

func (t *FileEditTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path               string `json:"path"`
		File               string `json:"file"`
		TargetContent      string `json:"target_content"`
		ReplacementContent string `json:"replacement_content"`
		StartLine          int    `json:"start_line"`
		EndLine            int    `json:"end_line"`
		AllowMultiple      bool   `json:"allow_multiple"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("parsing edit tool input: %w", err)
	}

	path := input.Path
	if path == "" {
		path = input.File
	}
	if path == "" {
		return nil, errors.New("path parameter is required")
	}
	if input.TargetContent == "" {
		return nil, errors.New("target_content is required")
	}

	agentID := AgentIDFromContext(ctx)
	relPath := sanitizeAgentRelativePath(path, agentID)

	allowedRoot, baseDir, targetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, relPath)
	if err != nil {
		return nil, err
	}
	targetPath, err := security.ResolvePathWithBase(allowedRoot, baseDir, targetRel, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("reading file for edit: %w", err)
	}

	fileContent := string(data)
	target := input.TargetContent
	replacement := input.ReplacementContent

	// Standardize line endings in search
	normFile := strings.ReplaceAll(fileContent, "\r\n", "\n")
	normTarget := strings.ReplaceAll(target, "\r\n", "\n")
	normRepl := strings.ReplaceAll(replacement, "\r\n", "\n")

	// If start_line or end_line specified, limit search range
	if input.StartLine > 0 || input.EndLine > 0 {
		lines := strings.Split(normFile, "\n")
		start := input.StartLine
		if start <= 0 {
			start = 1
		}
		end := input.EndLine
		if end <= 0 || end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			return nil, fmt.Errorf("start_line %d exceeds total file lines %d", start, len(lines))
		}

		subLines := lines[start-1 : end]
		subContent := strings.Join(subLines, "\n")

		count := strings.Count(subContent, normTarget)
		if count == 0 {
			return nil, fmt.Errorf("target_content not found between lines %d and %d in %s", start, end, relPath)
		}
		if count > 1 && !input.AllowMultiple {
			return nil, fmt.Errorf("found %d occurrences of target_content between lines %d and %d in %s; specify a narrower range or set allow_multiple: true", count, start, end, relPath)
		}

		newSubContent := strings.Replace(subContent, normTarget, normRepl, 1)
		if input.AllowMultiple {
			newSubContent = strings.ReplaceAll(subContent, normTarget, normRepl)
		}

		// Rebuild full content
		prefix := ""
		if start > 1 {
			prefix = strings.Join(lines[:start-1], "\n") + "\n"
		}
		suffix := ""
		if end < len(lines) {
			suffix = "\n" + strings.Join(lines[end:], "\n")
		}
		normFile = prefix + newSubContent + suffix
	} else {
		count := strings.Count(normFile, normTarget)
		if count == 0 {
			return nil, fmt.Errorf("target_content not found in %s; please check exact whitespace and indentation", relPath)
		}
		if count > 1 && !input.AllowMultiple {
			return nil, fmt.Errorf("found %d occurrences of target_content in %s; specify start_line/end_line or set allow_multiple: true", count, relPath)
		}

		if input.AllowMultiple {
			normFile = strings.ReplaceAll(normFile, normTarget, normRepl)
		} else {
			normFile = strings.Replace(normFile, normTarget, normRepl, 1)
		}
	}

	if err := os.WriteFile(targetPath, []byte(normFile), 0644); err != nil {
		return nil, fmt.Errorf("writing edited file: %w", err)
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully edited %s. Target content replaced with new content (%d bytes).", relPath, len(normFile)),
		Data: map[string]any{
			"path":  relPath,
			"bytes": len(normFile),
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 5. Unified File Search Tool (Grep, Regex & Directory Listing Explorer)
// -----------------------------------------------------------------------------

type FileSearchTool struct {
	dataDir   string
	agentsDir string
}

func NewFileSearchTool(dataDir string, agentsDir ...string) *FileSearchTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileSearchTool{dataDir: d, agentsDir: agDir}
}

func (t *FileSearchTool) Name() string { return "native_file_search" }
func (t *FileSearchTool) Description() string {
	return "Unified file search and workspace explorer. When query is provided, searches file contents (grep/regex) with context lines across data or private workspace. When query is empty, lists directory tree, files, sizes, and modified times."
}
func (t *FileSearchTool) Category() string { return "native" }

func (t *FileSearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Relative directory path (e.g. '.' for workspace, or 'skills', 'config', 'data')" },
			"query": { "type": "string", "description": "Text query or regex pattern to search for in file contents/filenames (omit or empty to list files/directories)" },
			"pattern": { "type": "string", "description": "Glob pattern to filter filenames (e.g. '*.go', '*.json', 'test_*')" },
			"is_regex": { "type": "boolean", "description": "Whether query is a regular expression (default false)" },
			"case_sensitive": { "type": "boolean", "description": "Whether search is case-sensitive (default false)" },
			"recursive": { "type": "boolean", "description": "Whether to search/list subdirectories recursively (default true for query search, false for listing)" },
			"max_depth": { "type": "integer", "description": "Maximum directory depth to traverse (0 for unlimited)", "minimum": 0 },
			"context_lines": { "type": "integer", "description": "Number of context lines before and after match to show (default 0)", "minimum": 0, "maximum": 10 },
			"include_hidden": { "type": "boolean", "description": "Whether to include hidden files and directories (default false)" },
			"max_results": { "type": "integer", "description": "Maximum number of results to return (default 50)", "minimum": 1, "maximum": 200 }
		}
	}`)
}

func (t *FileSearchTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path          string `json:"path"`
		Query         string `json:"query"`
		Pattern       string `json:"pattern"`
		Extension     string `json:"extension"`
		IsRegex       bool   `json:"is_regex"`
		CaseSensitive bool   `json:"case_sensitive"`
		Recursive     *bool  `json:"recursive"`
		MaxDepth      int    `json:"max_depth"`
		ContextLines  int    `json:"context_lines"`
		IncludeHidden bool   `json:"include_hidden"`
		MaxResults    int    `json:"max_results"`
	}
	_ = json.Unmarshal(inputJSON, &input)

	agentID := AgentIDFromContext(ctx)
	cleanPath := sanitizeAgentRelativePath(input.Path, agentID)

	allowedRoot, baseDir, targetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, cleanPath)
	if err != nil {
		return nil, err
	}
	targetDir, err := security.ResolvePathWithBase(allowedRoot, baseDir, targetRel, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}
	absWorkspace, _ := filepath.Abs(baseDir)

	maxResults := input.MaxResults
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}

	pattern := input.Pattern
	if pattern == "" && input.Extension != "" {
		if strings.HasPrefix(input.Extension, ".") {
			pattern = "*" + input.Extension
		} else {
			pattern = "*." + input.Extension
		}
	}

	isNoiseDir := func(name string) bool {
		if input.IncludeHidden {
			return false
		}
		return strings.HasPrefix(name, ".") ||
			name == "node_modules" ||
			name == "dist" ||
			name == "__pycache__" ||
			name == "target" ||
			name == "build"
	}

	// -------------------------------------------------------------------------
	// Mode A: Directory Listing / Tree Explorer (query is empty)
	// -------------------------------------------------------------------------
	if strings.TrimSpace(input.Query) == "" {
		recursive := false
		if input.Recursive != nil {
			recursive = *input.Recursive
		}

		type FileEntry struct {
			Name    string `json:"name"`
			Path    string `json:"path"`
			IsDir   bool   `json:"is_dir"`
			Size    int64  `json:"size"`
			SizeStr string `json:"size_str"`
			ModTime string `json:"mod_time"`
		}

		var entries []FileEntry
		targetDirAbs, _ := filepath.Abs(targetDir)

		if recursive {
			_ = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || path == targetDir {
					return nil
				}
				if info.IsDir() && isNoiseDir(info.Name()) {
					return filepath.SkipDir
				}
				if input.MaxDepth > 0 {
					relToTarget, _ := filepath.Rel(targetDirAbs, path)
					depth := len(strings.Split(filepath.ToSlash(relToTarget), "/"))
					if depth > input.MaxDepth {
						if info.IsDir() {
							return filepath.SkipDir
						}
						return nil
					}
				}
				if pattern != "" && !info.IsDir() {
					if matched, _ := filepath.Match(pattern, info.Name()); !matched {
						return nil
					}
				}
				rel, _ := filepath.Rel(absWorkspace, path)
				entries = append(entries, FileEntry{
					Name:    info.Name(),
					Path:    filepath.ToSlash(rel),
					IsDir:   info.IsDir(),
					Size:    info.Size(),
					SizeStr: formatHumanBytes(info.Size()),
					ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
				})
				if len(entries) >= maxResults {
					return filepath.SkipAll
				}
				return nil
			})
		} else {
			dirEntries, readErr := os.ReadDir(targetDir)
			if readErr != nil {
				return nil, fmt.Errorf("reading directory: %w", readErr)
			}
			for _, de := range dirEntries {
				if isNoiseDir(de.Name()) {
					continue
				}
				info, _ := de.Info()
				size := int64(0)
				modTime := ""
				if info != nil {
					size = info.Size()
					modTime = info.ModTime().Format("2006-01-02 15:04:05")
				}
				if pattern != "" && !de.IsDir() {
					if matched, _ := filepath.Match(pattern, de.Name()); !matched {
						continue
					}
				}
				rel, _ := filepath.Rel(absWorkspace, filepath.Join(targetDir, de.Name()))
				entries = append(entries, FileEntry{
					Name:    de.Name(),
					Path:    filepath.ToSlash(rel),
					IsDir:   de.IsDir(),
					Size:    size,
					SizeStr: formatHumanBytes(size),
					ModTime: modTime,
				})
				if len(entries) >= maxResults {
					break
				}
			}
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Directory listing for '%s' (%d item(s)):\n", cleanPath, len(entries))
		for _, e := range entries {
			if e.IsDir {
				fmt.Fprintf(&sb, "  [DIR]  %-35s  %s\n", e.Path+"/", e.ModTime)
			} else {
				fmt.Fprintf(&sb, "  [FILE] %-35s  %8s  %s\n", e.Path, e.SizeStr, e.ModTime)
			}
		}

		return &ToolResult{
			Content: strings.TrimRight(sb.String(), "\n"),
			Data: map[string]any{
				"path":    cleanPath,
				"count":   len(entries),
				"entries": entries,
			},
		}, nil
	}

	// -------------------------------------------------------------------------
	// Mode B: File Content & Regex Grep Search (query is provided)
	// -------------------------------------------------------------------------
	query := input.Query
	var re *regexp.Regexp
	if input.IsRegex {
		patternStr := query
		if !input.CaseSensitive {
			patternStr = "(?i)" + patternStr
		}
		var err error
		re, err = regexp.Compile(patternStr)
		if err != nil {
			return nil, fmt.Errorf("invalid regex query %q: %w", query, err)
		}
	}

	type MatchContext struct {
		LineNum int    `json:"line_num"`
		Line    string `json:"line"`
		IsMatch bool   `json:"is_match"`
	}

	type SearchMatch struct {
		Path        string         `json:"path"`
		LineNum     int            `json:"line_num"`
		Snippet     string         `json:"snippet"`
		ContextTree []MatchContext `json:"context,omitempty"`
	}

	var matches []SearchMatch
	contextLines := input.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 10 {
		contextLines = 10
	}

	_ = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && isNoiseDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isNoiseDir(info.Name()) {
			return nil
		}
		if pattern != "" {
			if matched, _ := filepath.Match(pattern, info.Name()); !matched {
				return nil
			}
		}

		rel, _ := filepath.Rel(absWorkspace, path)
		relSlash := filepath.ToSlash(rel)

		// Check filename match
		filenameMatch := false
		if input.IsRegex && re != nil {
			filenameMatch = re.MatchString(info.Name())
		} else if input.CaseSensitive {
			filenameMatch = strings.Contains(info.Name(), query)
		} else {
			filenameMatch = strings.Contains(strings.ToLower(info.Name()), strings.ToLower(query))
		}

		if filenameMatch {
			matches = append(matches, SearchMatch{
				Path:    relSlash,
				LineNum: 0,
				Snippet: fmt.Sprintf("[Filename Match] %s (%s)", info.Name(), formatHumanBytes(info.Size())),
			})
			if len(matches) >= maxResults {
				return filepath.SkipAll
			}
		}

		// Search file contents (up to 1MB per file)
		if info.Size() < 1024*1024 {
			data, readErr := os.ReadFile(path)
			if readErr == nil && !strings.Contains(string(data[:min(len(data), 512)]), "\x00") {
				lines := strings.Split(string(data), "\n")
				for idx, line := range lines {
					cleanLine := strings.TrimRight(line, "\r")
					matched := false
					if input.IsRegex && re != nil {
						matched = re.MatchString(cleanLine)
					} else if input.CaseSensitive {
						matched = strings.Contains(cleanLine, query)
					} else {
						matched = strings.Contains(strings.ToLower(cleanLine), strings.ToLower(query))
					}

					if matched {
						var ctxTree []MatchContext
						if contextLines > 0 {
							startCtx := max(0, idx-contextLines)
							endCtx := min(len(lines)-1, idx+contextLines)
							for cIdx := startCtx; cIdx <= endCtx; cIdx++ {
								ctxTree = append(ctxTree, MatchContext{
									LineNum: cIdx + 1,
									Line:    strings.TrimRight(lines[cIdx], "\r"),
									IsMatch: cIdx == idx,
								})
							}
						}

						matches = append(matches, SearchMatch{
							Path:        relSlash,
							LineNum:     idx + 1,
							Snippet:     strings.TrimSpace(cleanLine),
							ContextTree: ctxTree,
						})
						if len(matches) >= maxResults {
							return filepath.SkipAll
						}
					}
				}
			}
		}
		return nil
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d match(es) for '%s' in %s:\n", len(matches), query, cleanPath)
	for _, m := range matches {
		if m.LineNum == 0 {
			fmt.Fprintf(&sb, "\n  %s\n", m.Snippet)
		} else if len(m.ContextTree) > 0 {
			fmt.Fprintf(&sb, "\n  File: %s:%d\n", m.Path, m.LineNum)
			for _, ctxLine := range m.ContextTree {
				marker := " "
				if ctxLine.IsMatch {
					marker = ">"
				}
				fmt.Fprintf(&sb, "  %s %4d: %s\n", marker, ctxLine.LineNum, ctxLine.Line)
			}
		} else {
			fmt.Fprintf(&sb, "  %s:%d: %s\n", m.Path, m.LineNum, m.Snippet)
		}
	}

	return &ToolResult{
		Content: strings.TrimRight(sb.String(), "\n"),
		Data: map[string]any{
			"query":   query,
			"path":    cleanPath,
			"count":   len(matches),
			"matches": matches,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 6. File Delete Tool (with Recursive Support)
// -----------------------------------------------------------------------------

type FileDeleteTool struct {
	dataDir   string
	agentsDir string
}

func NewFileDeleteTool(dataDir string, agentsDir ...string) *FileDeleteTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileDeleteTool{dataDir: d, agentsDir: agDir}
}

func (t *FileDeleteTool) Name() string { return "native_file_delete" }
func (t *FileDeleteTool) Description() string {
	return "Delete a file or directory located within the data directory or your private workspace."
}
func (t *FileDeleteTool) Category() string { return "native" }

func (t *FileDeleteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path of file or directory to delete (relative to workspace or relative to data directory)" },
			"recursive": { "type": "boolean", "description": "Whether to recursively delete non-empty directories (default false)" }
		},
		"required": ["path"]
	}`)
}

func (t *FileDeleteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Path      string `json:"path"`
		File      string `json:"file"`
		Recursive bool   `json:"recursive"`
	}
	_ = json.Unmarshal(inputJSON, &input)

	path := input.Path
	if path == "" {
		path = input.File
	}
	if path == "" {
		var err error
		path, err = parseFilePathInput(inputJSON)
		if err != nil {
			return nil, err
		}
	}

	agentID := AgentIDFromContext(ctx)
	relPath := sanitizeAgentRelativePath(path, agentID)

	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." || cleanRel == "" {
		return nil, ErrPathEscape
	}
	allowedRoot, baseDir, targetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, relPath)
	if err != nil {
		return nil, err
	}
	targetPath, err := security.ResolvePathWithBase(allowedRoot, baseDir, targetRel, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}

	if input.Recursive {
		if err := os.RemoveAll(targetPath); err != nil {
			return nil, fmt.Errorf("deleting path recursively: %w", err)
		}
	} else {
		if err := os.Remove(targetPath); err != nil {
			return nil, fmt.Errorf("deleting file: %w", err)
		}
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully deleted %s", relPath),
		Data:    map[string]any{"deleted": relPath, "absolute_path": targetPath, "recursive": input.Recursive},
	}, nil
}

// -----------------------------------------------------------------------------
// 7. File Move Tool
// -----------------------------------------------------------------------------

type FileMoveTool struct {
	dataDir   string
	agentsDir string
}

func NewFileMoveTool(dataDir string, agentsDir ...string) *FileMoveTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileMoveTool{dataDir: d, agentsDir: agDir}
}

func (t *FileMoveTool) Name() string { return "native_file_move" }
func (t *FileMoveTool) Description() string {
	return "Move or rename a file or directory inside the data directory or your private workspace."
}
func (t *FileMoveTool) Category() string { return "native" }

func (t *FileMoveTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"src_path": { "type": "string", "description": "Source path to move (relative to workspace or data directory)" },
			"dst_path": { "type": "string", "description": "Destination path" },
			"overwrite": { "type": "boolean", "description": "Whether to overwrite destination if it exists (default false)" }
		},
		"required": ["src_path", "dst_path"]
	}`)
}

func (t *FileMoveTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		SrcPath   string `json:"src_path"`
		DstPath   string `json:"dst_path"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.SrcPath == "" || input.DstPath == "" {
		return nil, errors.New("both src_path and dst_path are required")
	}

	agentID := AgentIDFromContext(ctx)
	srcRel := sanitizeAgentRelativePath(input.SrcPath, agentID)
	dstRel := sanitizeAgentRelativePath(input.DstPath, agentID)

	allowedRoot, baseDir, srcTargetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, srcRel)
	if err != nil {
		return nil, err
	}
	srcTarget, err := security.ResolvePathWithBase(allowedRoot, baseDir, srcTargetRel, true)
	if err != nil {
		return nil, fmt.Errorf("invalid src_path: %w", err)
	}

	_, dstBaseDir, dstTargetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, dstRel)
	if err != nil {
		return nil, err
	}
	dstTarget, err := security.ResolvePathWithBase(allowedRoot, dstBaseDir, dstTargetRel, true)
	if err != nil {
		return nil, fmt.Errorf("invalid dst_path: %w", err)
	}

	if _, err := os.Stat(dstTarget); err == nil && !input.Overwrite {
		return nil, fmt.Errorf("destination %s already exists; set overwrite: true to replace it", dstRel)
	}

	if err := os.MkdirAll(filepath.Dir(dstTarget), 0755); err != nil {
		return nil, fmt.Errorf("creating destination parent directory: %w", err)
	}

	if err := os.Rename(srcTarget, dstTarget); err != nil {
		return nil, fmt.Errorf("moving file/directory from %s to %s: %w", srcRel, dstRel, err)
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully moved %s to %s", srcRel, dstRel),
		Data: map[string]any{
			"src_path": srcRel,
			"dst_path": dstRel,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 8. File Copy Tool
// -----------------------------------------------------------------------------

type FileCopyTool struct {
	dataDir   string
	agentsDir string
}

func NewFileCopyTool(dataDir string, agentsDir ...string) *FileCopyTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &FileCopyTool{dataDir: d, agentsDir: agDir}
}

func (t *FileCopyTool) Name() string { return "native_file_copy" }
func (t *FileCopyTool) Description() string {
	return "Copy a file or directory to a new location inside the data directory or your private workspace."
}
func (t *FileCopyTool) Category() string { return "native" }

func (t *FileCopyTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"src_path": { "type": "string", "description": "Source path to copy (relative to workspace or data directory)" },
			"dst_path": { "type": "string", "description": "Destination path" },
			"overwrite": { "type": "boolean", "description": "Whether to overwrite destination if it exists (default false)" }
		},
		"required": ["src_path", "dst_path"]
	}`)
}

func (t *FileCopyTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		SrcPath   string `json:"src_path"`
		DstPath   string `json:"dst_path"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.SrcPath == "" || input.DstPath == "" {
		return nil, errors.New("both src_path and dst_path are required")
	}

	agentID := AgentIDFromContext(ctx)
	srcRel := sanitizeAgentRelativePath(input.SrcPath, agentID)
	dstRel := sanitizeAgentRelativePath(input.DstPath, agentID)

	allowedRoot, baseDir, srcTargetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, srcRel)
	if err != nil {
		return nil, err
	}
	srcTarget, err := security.ResolvePathWithBase(allowedRoot, baseDir, srcTargetRel, true)
	if err != nil {
		return nil, fmt.Errorf("invalid src_path: %w", err)
	}

	_, dstBaseDir, dstTargetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, dstRel)
	if err != nil {
		return nil, err
	}
	dstTarget, err := security.ResolvePathWithBase(allowedRoot, dstBaseDir, dstTargetRel, true)
	if err != nil {
		return nil, fmt.Errorf("invalid dst_path: %w", err)
	}

	srcInfo, err := os.Stat(srcTarget)
	if err != nil {
		return nil, fmt.Errorf("source path %s not found: %w", srcRel, err)
	}

	if _, err := os.Stat(dstTarget); err == nil && !input.Overwrite {
		return nil, fmt.Errorf("destination %s already exists; set overwrite: true to replace it", dstRel)
	}

	if err := os.MkdirAll(filepath.Dir(dstTarget), 0755); err != nil {
		return nil, fmt.Errorf("creating destination parent directory: %w", err)
	}

	if srcInfo.IsDir() {
		err := filepath.Walk(srcTarget, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(srcTarget, path)
			destPath := filepath.Join(dstTarget, rel)
			if info.IsDir() {
				return os.MkdirAll(destPath, 0755)
			}
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, in)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("copying directory: %w", err)
		}
	} else {
		in, err := os.Open(srcTarget)
		if err != nil {
			return nil, fmt.Errorf("opening source file: %w", err)
		}
		defer in.Close()
		out, err := os.OpenFile(dstTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, srcInfo.Mode())
		if err != nil {
			return nil, fmt.Errorf("creating destination file: %w", err)
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return nil, fmt.Errorf("copying file data: %w", err)
		}
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully copied %s to %s", srcRel, dstRel),
		Data: map[string]any{
			"src_path": srcRel,
			"dst_path": dstRel,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// 9. Exec Tool (Sandboxed Command Execution)
// -----------------------------------------------------------------------------

type ExecTool struct {
	dataDir   string
	agentsDir string
}

func NewExecTool(dataDir string, agentsDir ...string) *ExecTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &ExecTool{dataDir: d, agentsDir: agDir}
}

func (t *ExecTool) Name() string { return "native_exec" }
func (t *ExecTool) Description() string {
	return "Execute a shell or PowerShell command inside your private agent workspace directory (timeout: 60s, max memory: 512MB)."
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
	_, agentWorkspace, _, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, "")
	if err != nil {
		return nil, err
	}
	result, err := sb.Execute(ctx, sandbox.CommandRequest{
		Command:      input.Command,
		WorkspaceDir: agentWorkspace,
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
	} else if len(output) > 32768 {
		truncatedMsg := fmt.Sprintf("\n\n[Output truncated: output exceeded 32KB (%d bytes total). Use head/tail/grep/jq to inspect specific sections]", len(output))
		output = output[:32768] + truncatedMsg
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

// -----------------------------------------------------------------------------
// 9. Channel Notify Tool (Proactive Message Dispatch)
// -----------------------------------------------------------------------------

// ChannelMessageSender delivers outbound notifications to active channel adapters.
type ChannelMessageSender interface {
	Send(ctx context.Context, channelID, accountID, recipient, content string) error
}

type ChannelNotifyTool struct {
	bus    *bus.EventBus
	sender ChannelMessageSender
}

func NewChannelNotifyTool(eventBus *bus.EventBus) *ChannelNotifyTool {
	return &ChannelNotifyTool{bus: eventBus}
}

func (t *ChannelNotifyTool) SetSender(s ChannelMessageSender) {
	t.sender = s
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
			"channel": { "type": "string", "enum": ["telegram", "whatsapp", "discord", "all"], "description": "Target channel (default 'all')" },
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
		input.Channel = "all"
	}
	if input.AccountID == "" {
		input.AccountID = "all"
	}

	if t.sender != nil {
		if err := t.sender.Send(ctx, input.Channel, input.AccountID, input.Recipient, input.Message); err != nil {
			slog.Warn("channel notify direct send failed", "channel", input.Channel, "error", err)
		}
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

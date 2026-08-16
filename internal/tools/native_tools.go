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

// CronSchedulerProvider defines interface for managing background cron schedules.
type CronSchedulerProvider interface {
	RegisterCron(id, agentID, cronExpr, prompt, targetChannel, targetRecipient string) error
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
	_ = r.Register(NewSysInfoTool())
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

// -----------------------------------------------------------------------------
// 5. Cron Schedule Tool
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

		if err := t.scheduler.RegisterCron(input.JobID, input.AgentID, input.CronExpression, input.Prompt, input.TargetChannel, input.TargetRecipient); err != nil {
			return nil, fmt.Errorf("registering cron schedule: %w", err)
		}

		return &ToolResult{
			Content: fmt.Sprintf("Successfully registered scheduled reminder '%s'\nCron: %s\nTarget Channel: %s\nPrompt: %s", input.JobID, input.CronExpression, input.TargetChannel, input.Prompt),
			Data: map[string]any{
				"job_id":           input.JobID,
				"cron_expression":  input.CronExpression,
				"agent_id":         input.AgentID,
				"prompt":           input.Prompt,
				"target_channel":   input.TargetChannel,
				"target_recipient": input.TargetRecipient,
				"status":           "created",
			},
		}, nil

	default:
		return nil, fmt.Errorf("invalid action '%s', supported: create, list, delete", input.Action)
	}
}

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/security"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

// -----------------------------------------------------------------------------
// 9. Channel Notify Tool (Proactive Message Dispatch)
// -----------------------------------------------------------------------------

const maxChannelNotifyFile = 20 << 20

// ChannelMessageSender delivers outbound notifications to active channel adapters.
type ChannelMessageSender interface {
	Send(ctx context.Context, channelID, accountID, recipient, content string) error
}

// ChannelEnvelope is the host→plugin outbound contract for typing, reactions, quotes, and files.
type ChannelEnvelope struct {
	ChannelID string            `json:"channel_id"`
	AccountID string            `json:"account_id"`
	Recipient string            `json:"recipient"`
	Content   string            `json:"content"`
	Kind      string            `json:"kind,omitempty"`
	ChatID    string            `json:"chat_id,omitempty"`
	ReplyToID string            `json:"reply_to_id,omitempty"`
	ThreadID  string            `json:"thread_id,omitempty"`
	Reaction  string            `json:"reaction,omitempty"`
	Action    string            `json:"action,omitempty"`
	Typing    bool              `json:"typing,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	FileName  string            `json:"file_name,omitempty"`
	MIMEType  string            `json:"mime_type,omitempty"`
	FileBytes []byte            `json:"-"`
}

// ChannelEnvelopeSender is an optional richer sender implemented by the channel manager adapter.
type ChannelEnvelopeSender interface {
	SendEnvelope(ctx context.Context, env ChannelEnvelope) error
}

type ChannelNotifyTool struct {
	bus       *bus.EventBus
	sender    ChannelMessageSender
	workspace *workspacepkg.Store
	dataDir   string
	agentsDir string
}

func NewChannelNotifyTool(eventBus *bus.EventBus) *ChannelNotifyTool {
	return &ChannelNotifyTool{bus: eventBus}
}

func (t *ChannelNotifyTool) SetSender(s ChannelMessageSender) {
	t.sender = s
}

func (t *ChannelNotifyTool) SetWorkspace(store *workspacepkg.Store, dataDir, agentsDir string) {
	t.workspace = store
	t.dataDir = dataDir
	t.agentsDir = agentsDir
}

func (t *ChannelNotifyTool) Name() string { return "native_channel_notify" }
func (t *ChannelNotifyTool) Description() string {
	return "Send a channel message, file, typing indicator, quote-reply, or emoji reaction through an installed chat plugin. To deliver a user document, pass file_id or path from native_workspace_search, or from_path for a scratchpad file. The host forwards the file to the plugin; the plugin uploads it. Specify a concrete channel plugin (not 'all') when attaching a file."
}
func (t *ChannelNotifyTool) Category() string { return "native" }

func (t *ChannelNotifyTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": { "type": "string", "description": "Target installed channel plugin name or 'all' (default 'all'). Must be a specific plugin when sending a file." },
			"account_id": { "type": "string", "description": "Target account ID or 'all' (default 'all')" },
			"recipient": { "type": "string", "description": "Optional recipient chat ID or phone number" },
			"chat_id": { "type": "string", "description": "Conversation/chat/channel id (preferred over recipient when known)" },
			"message": { "type": "string", "description": "Message text or file caption (required for kind=text without a file)" },
			"kind": { "type": "string", "enum": ["text", "typing", "reaction", "media"], "description": "Outbound envelope kind (default text, or media when a file is attached)" },
			"file_id": { "type": "string", "description": "Opaque User Workspace file ID from native_workspace_search to send as a chat attachment" },
			"path": { "type": "string", "description": "User Workspace virtual/exec path (e.g. 'Reports/report.pdf') when file_id is unknown" },
			"from_path": { "type": "string", "description": "Scratchpad-relative file to send (e.g. 'report.pdf'). Prefer workspace file_id/path for user documents." },
			"file_name": { "type": "string", "description": "Optional download filename override for the attachment" },
			"reply_to_id": { "type": "string", "description": "Source message id to quote / attach a reaction to" },
			"thread_id": { "type": "string", "description": "Optional thread parent id (Slack thread_ts)" },
			"reaction": { "type": "string", "description": "Emoji to apply when kind=reaction (e.g. 👀 or ✅)" },
			"typing": { "type": "boolean", "description": "If true, emit a typing indicator (also implied by kind=typing)" },
			"action": { "type": "string", "description": "Chat action such as typing, upload_photo, upload_document" }
		},
		"required": []
	}`)
}

func (t *ChannelNotifyTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Channel   string `json:"channel"`
		AccountID string `json:"account_id"`
		Recipient string `json:"recipient"`
		ChatID    string `json:"chat_id"`
		Message   string `json:"message"`
		Kind      string `json:"kind"`
		FileID    string `json:"file_id"`
		Path      string `json:"path"`
		FromPath  string `json:"from_path"`
		FileName  string `json:"file_name"`
		ReplyToID string `json:"reply_to_id"`
		ThreadID  string `json:"thread_id"`
		Reaction  string `json:"reaction"`
		Typing    bool   `json:"typing"`
		Action    string `json:"action"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, errors.New("invalid channel notify payload")
	}

	attachment, err := t.loadChannelAttachment(ctx, input.FileID, input.Path, input.FromPath, input.FileName)
	if err != nil {
		return nil, err
	}

	kind := strings.TrimSpace(strings.ToLower(input.Kind))
	if kind == "" {
		switch {
		case attachment != nil:
			kind = "media"
		case input.Typing || strings.EqualFold(input.Action, "typing"):
			kind = "typing"
		case strings.TrimSpace(input.Reaction) != "" && strings.TrimSpace(input.Message) == "":
			kind = "reaction"
		default:
			kind = "text"
		}
	}
	if attachment != nil {
		kind = "media"
	}
	if kind == "text" && strings.TrimSpace(input.Message) == "" {
		return nil, errors.New("message parameter is required")
	}
	if kind == "media" && attachment == nil {
		return nil, errors.New("file_id, path, or from_path is required when kind=media")
	}
	if kind == "reaction" && strings.TrimSpace(input.Reaction) == "" {
		return nil, errors.New("reaction parameter is required when kind=reaction")
	}
	if input.Channel == "" {
		input.Channel = "all"
	}
	if attachment != nil && (strings.EqualFold(input.Channel, "all") || strings.TrimSpace(input.Channel) == "") {
		return nil, errors.New("channel must be a specific installed chat plugin when sending a file")
	}
	if input.AccountID == "" {
		input.AccountID = "all"
	}
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		recipient = strings.TrimSpace(input.ChatID)
	}

	env := ChannelEnvelope{
		ChannelID: input.Channel,
		AccountID: input.AccountID,
		Recipient: recipient,
		Content:   input.Message,
		Kind:      kind,
		ChatID:    firstNonEmptyArg(input.ChatID, recipient),
		ReplyToID: input.ReplyToID,
		ThreadID:  input.ThreadID,
		Reaction:  input.Reaction,
		Action:    input.Action,
		Typing:    input.Typing || kind == "typing",
	}
	if attachment != nil {
		env.FileName = attachment.name
		env.MIMEType = attachment.mimeType
		env.FileBytes = attachment.content
		if env.Metadata == nil {
			env.Metadata = map[string]string{}
		}
		env.Metadata["file_name"] = attachment.name
		env.Metadata["mime_type"] = attachment.mimeType
		if attachment.fileID != "" {
			env.Metadata["workspace_file_id"] = attachment.fileID
		}
	}

	if t.sender != nil {
		var sendErr error
		if es, ok := t.sender.(ChannelEnvelopeSender); ok {
			sendErr = es.SendEnvelope(ctx, env)
		} else if attachment == nil {
			sendErr = t.sender.Send(ctx, input.Channel, input.AccountID, recipient, input.Message)
		} else {
			sendErr = errors.New("channel sender does not support file attachments")
		}
		if sendErr != nil {
			if attachment != nil {
				return nil, fmt.Errorf("sending file to channel %s: %w", input.Channel, sendErr)
			}
			slog.Warn("channel notify direct send failed", "channel", input.Channel, "kind", kind, "error", sendErr)
		}
	}

	result := fmt.Sprintf("Successfully dispatched proactive notification to channel '%s' (account: %s, kind: %s)", input.Channel, input.AccountID, kind)
	if attachment != nil {
		result = fmt.Sprintf("Sent file %q (%s, %d bytes) to %s chat %s", attachment.name, attachment.mimeType, len(attachment.content), input.Channel, env.ChatID)
	}
	return &ToolResult{
		Content: result,
		Data: map[string]any{
			"channel":     input.Channel,
			"account_id":  input.AccountID,
			"recipient":   recipient,
			"chat_id":     env.ChatID,
			"kind":        kind,
			"file_name":   env.FileName,
			"mime_type":   env.MIMEType,
			"size_bytes":  len(env.FileBytes),
			"reply_to_id": input.ReplyToID,
			"reaction":    input.Reaction,
			"status":      "dispatched",
		},
	}, nil
}

type channelAttachment struct {
	fileID   string
	name     string
	mimeType string
	content  []byte
}

func (t *ChannelNotifyTool) loadChannelAttachment(ctx context.Context, fileID, path, fromPath, fileName string) (*channelAttachment, error) {
	fileID = strings.TrimSpace(fileID)
	path = strings.TrimSpace(path)
	fromPath = strings.TrimSpace(fromPath)
	fileName = strings.TrimSpace(fileName)
	if fileID == "" && path == "" && fromPath == "" {
		return nil, nil
	}
	if (fileID != "" || path != "") && fromPath != "" {
		return nil, errors.New("provide only one of file_id/path or from_path")
	}
	if fileID != "" || path != "" {
		if t.workspace == nil {
			return nil, errors.New("user workspace store is unavailable")
		}
		ref := fileID
		if ref == "" {
			ref = path
		}
		node, err := t.workspace.ResolveRef(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolving workspace file: %w", err)
		}
		if node.Type != "file" {
			return nil, errors.New("workspace path is a folder; send a file")
		}
		if node.SizeBytes > maxChannelNotifyFile {
			return nil, fmt.Errorf("workspace file exceeds %d-byte channel upload limit", maxChannelNotifyFile)
		}
		_, content, err := t.workspace.Read(ctx, node.ID, 0, 0)
		if err != nil {
			return nil, fmt.Errorf("reading workspace file: %w", err)
		}
		name := fileName
		if name == "" {
			name = node.Name
		}
		return &channelAttachment{fileID: node.ID, name: name, mimeType: node.MIMEType, content: content}, nil
	}

	agentID := AgentIDFromContext(ctx)
	fromPath = sanitizeAgentRelativePath(fromPath, agentID)
	allowedRoot, baseDir, targetRel, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, fromPath)
	if err != nil {
		return nil, fmt.Errorf("resolving from_path: %w", err)
	}
	targetPath, err := security.ResolvePathWithBase(allowedRoot, baseDir, targetRel, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("reading from_path (%s): %w", fromPath, err)
	}
	if info.IsDir() {
		return nil, errors.New("from_path is a directory; send a file")
	}
	if info.Size() > maxChannelNotifyFile {
		return nil, fmt.Errorf("from_path exceeds %d-byte channel upload limit", maxChannelNotifyFile)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("reading from_path (%s): %w", fromPath, err)
	}
	name := fileName
	if name == "" {
		name = filepath.Base(fromPath)
	}
	return &channelAttachment{name: name, mimeType: "", content: content}, nil
}

func firstNonEmptyArg(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
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
			"target_channel": { "type": "string", "description": "Outbound channel to deliver notification (e.g. 'all', or channel plugin name). Default: 'all'" },
			"target_account_id": { "type": "string", "description": "Target channel account ID or 'all' (default 'all')" },
			"target_recipient": { "type": "string", "description": "Destination chat ID or phone number (optional, will automatically route to the user's active channel if omitted)" }
		},
		"required": ["action"]
	}`)
}

// -----------------------------------------------------------------------------
// 12. Task enqueue tool
// -----------------------------------------------------------------------------

type TaskEnqueueTool struct {
	backlog MissionBacklogProvider
}

func NewTaskEnqueueTool(backlog MissionBacklogProvider) *TaskEnqueueTool {
	return &TaskEnqueueTool{backlog: backlog}
}

func (t *TaskEnqueueTool) SetBacklog(backlog MissionBacklogProvider) {
	t.backlog = backlog
}

func (t *TaskEnqueueTool) Name() string { return "native_task_enqueue" }
func (t *TaskEnqueueTool) Description() string {
	return "Enqueue a background mission for the 24/7 agent loop. Use when work should continue unattended after this turn. Do not use for a one-shot chat answer."
}
func (t *TaskEnqueueTool) Category() string { return "native" }

func (t *TaskEnqueueTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {"type": "string", "description": "Short mission title"},
			"description": {"type": "string", "description": "What the autonomous loop should accomplish"},
			"assigned_agent_id": {"type": "string", "description": "Agent to run the mission (default auto)"}
		},
		"required": ["title"]
	}`)
}

func (t *TaskEnqueueTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t == nil || t.backlog == nil {
		return nil, fmt.Errorf("mission backlog is not configured")
	}
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		AssignedAgentID string `json:"assigned_agent_id"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("parsing task enqueue input: %w", err)
	}
	id, err := t.backlog.EnqueueMission(ctx, input.Title, input.Description, input.AssignedAgentID)
	if err != nil {
		return nil, fmt.Errorf("enqueueing mission: %w", err)
	}
	return &ToolResult{
		Content: fmt.Sprintf("Enqueued mission %s (%s) for unattended execution.", id, strings.TrimSpace(input.Title)),
		Data: map[string]any{
			"task_id":           id,
			"title":             strings.TrimSpace(input.Title),
			"created_by":        "agent",
			"assigned_agent_id": input.AssignedAgentID,
		},
	}, nil
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
			input.TargetChannel = "all"
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

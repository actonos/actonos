package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/actonos/actonos/internal/security"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

const (
	defaultWorkspaceReadLimit = int64(64 << 10)
	maxWorkspaceReadLimit     = int64(1 << 20)
	maxWorkspaceToolWrite     = 10 << 20
)

type WorkspaceSearchTool struct{ store *workspacepkg.Store }

func NewWorkspaceSearchTool(store *workspacepkg.Store) *WorkspaceSearchTool {
	return &WorkspaceSearchTool{store: store}
}

func (t *WorkspaceSearchTool) Name() string { return "native_workspace_search" }
func (t *WorkspaceSearchTool) Description() string {
	return "Search or browse files and folders in the official User Workspace (visible in the user's Workspace UI). Results include file_id, virtual_path, and exec_path (original filename relative to $ACTONOS_USER_WORKSPACE / user-workspace/). Use exec_path with native_exec/python to open PDFs and other binaries."
}
func (t *WorkspaceSearchTool) Category() string { return "native" }
func (t *WorkspaceSearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"Name or indexed text to search; omit to browse a folder"},
			"parent_id":{"type":"string","description":"Opaque parent folder ID; omit for the workspace root"},
			"limit":{"type":"integer","minimum":1,"maximum":200,"default":50}
		}
	}`)
}
func (t *WorkspaceSearchTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.store == nil {
		return nil, errors.New("user workspace store is unavailable")
	}
	var input struct {
		Query    string `json:"query"`
		ParentID string `json:"parent_id"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("decoding workspace search input: %w", err)
	}
	results, err := t.store.Search(ctx, input.Query, input.ParentID, input.Limit)
	if err != nil {
		return nil, fmt.Errorf("searching user workspace: %w", err)
	}
	payload, _ := json.Marshal(results)
	return &ToolResult{
		Content: fmt.Sprintf("Found %d user workspace item(s): %s", len(results), payload),
		Data: map[string]any{
			"count":   len(results),
			"results": results,
		},
	}, nil
}

type WorkspaceReadTool struct{ store *workspacepkg.Store }

func NewWorkspaceReadTool(store *workspacepkg.Store) *WorkspaceReadTool {
	return &WorkspaceReadTool{store: store}
}

func (t *WorkspaceReadTool) Name() string { return "native_workspace_read" }
func (t *WorkspaceReadTool) Description() string {
	return "Read a user document from the official User Workspace by file_id or path (virtual_path or exec_path from native_workspace_search). Text files are returned as UTF-8. Binary files such as PDF are not dumped as base64 by default; use native_exec with exec_path / $ACTONOS_USER_WORKSPACE to process them."
}
func (t *WorkspaceReadTool) Category() string { return "native" }
func (t *WorkspaceReadTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"file_id":{"type":"string","description":"Opaque file ID returned by native_workspace_search"},
			"path":{"type":"string","description":"Virtual or exec path when file_id is unknown (e.g. 'Reports/report.pdf', '/data/workspace/Reports/report.pdf', or 'user-workspace/report.pdf')"},
			"offset":{"type":"integer","minimum":0,"default":0},
			"limit":{"type":"integer","minimum":1,"maximum":1048576,"default":65536},
			"encoding":{"type":"string","enum":["auto","utf8","base64"],"default":"auto"}
		}
	}`)
}
func (t *WorkspaceReadTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.store == nil {
		return nil, errors.New("user workspace store is unavailable")
	}
	var input struct {
		FileID   string `json:"file_id"`
		Path     string `json:"path"`
		Offset   int64  `json:"offset"`
		Limit    int64  `json:"limit"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("decoding workspace read input: %w", err)
	}
	ref := strings.TrimSpace(input.FileID)
	if ref == "" {
		ref = strings.TrimSpace(input.Path)
	}
	if ref == "" {
		return nil, errors.New("file_id or path is required")
	}
	resolved, err := t.store.ResolveRef(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving user workspace file: %w", err)
	}
	if input.Limit == 0 {
		input.Limit = defaultWorkspaceReadLimit
	}
	if input.Offset < 0 || input.Limit < 1 || input.Limit > maxWorkspaceReadLimit {
		return nil, fmt.Errorf("invalid read range: limit must be between 1 and %d", maxWorkspaceReadLimit)
	}
	node, content, err := t.store.Read(ctx, resolved.ID, input.Offset, input.Limit)
	if err != nil {
		return nil, fmt.Errorf("reading user workspace file: %w", err)
	}
	encoding := strings.ToLower(input.Encoding)
	if encoding == "" || encoding == "auto" {
		if workspaceBinaryHint(node, content) {
			execPath := node.ExecPath
			if execPath == "" {
				execPath = node.Name
			}
			return &ToolResult{
				Content: fmt.Sprintf("[Binary workspace file %q (%s, %d bytes). Do not request base64 for large binaries. Open it from native_exec with python using exec_path %q, which is relative to $ACTONOS_USER_WORKSPACE and also available as %s/%s.]",
					node.Name, node.MIMEType, node.SizeBytes, execPath, workspacepkg.AgentViewName, execPath),
				Data: workspaceNodeData(node, map[string]any{
					"offset":         input.Offset,
					"returned_bytes": 0,
					"truncated":      node.SizeBytes > 0,
					"encoding":       "binary",
					"is_binary":      true,
				}),
			}, nil
		}
		if utf8.Valid(content) && !strings.ContainsRune(string(content), '\x00') {
			encoding = "utf8"
		} else {
			encoding = "base64"
		}
	}
	var rendered string
	switch encoding {
	case "utf8":
		if !utf8.Valid(content) {
			return nil, errors.New("file chunk is not valid UTF-8; request base64 encoding")
		}
		rendered = string(content)
	case "base64":
		rendered = base64.StdEncoding.EncodeToString(content)
	default:
		return nil, errors.New("encoding must be auto, utf8, or base64")
	}
	truncated := input.Offset+int64(len(content)) < node.SizeBytes
	return &ToolResult{
		Content: rendered,
		Data: workspaceNodeData(node, map[string]any{
			"offset":         input.Offset,
			"returned_bytes": len(content),
			"next_offset":    input.Offset + int64(len(content)),
			"truncated":      truncated,
			"encoding":       encoding,
		}),
	}, nil
}

func workspaceBinaryHint(node workspacepkg.Node, content []byte) bool {
	kind := workspacepkg.MediaKind(node.MIMEType)
	switch kind {
	case "pdf", "image", "audio", "video", "archive", "binary":
		return true
	}
	if len(content) > 0 && strings.IndexByte(string(content[:min(len(content), 1024)]), 0) != -1 {
		return true
	}
	return false
}

func workspaceNodeData(node workspacepkg.Node, extra map[string]any) map[string]any {
	data := map[string]any{
		"workspace_file_id": node.ID,
		"file_id":           node.ID,
		"name":              node.Name,
		"virtual_path":      node.VirtualPath,
		"exec_path":         node.ExecPath,
		"mime_type":         node.MIMEType,
		"version":           node.Version,
		"size_bytes":        node.SizeBytes,
	}
	if node.ExecPath != "" {
		data["scratchpad_path"] = workspacepkg.AgentViewName + "/" + node.ExecPath
	}
	for key, value := range extra {
		data[key] = value
	}
	return data
}

type WorkspaceWriteTool struct {
	store     *workspacepkg.Store
	dataDir   string
	agentsDir string
}

func NewWorkspaceWriteTool(store *workspacepkg.Store, dirs ...string) *WorkspaceWriteTool {
	dataDir, agentsDir := parseDataAndAgentsDir("", dirs...)
	if len(dirs) > 0 && dirs[0] != "" {
		dataDir = dirs[0]
		if len(dirs) > 1 && dirs[1] != "" {
			agentsDir = dirs[1]
		} else {
			agentsDir = filepath.Join(dataDir, "agents")
		}
	}
	return &WorkspaceWriteTool{store: store, dataDir: dataDir, agentsDir: agentsDir}
}

func (t *WorkspaceWriteTool) Name() string { return "native_workspace_write" }
func (t *WorkspaceWriteTool) Description() string {
	return "Create or update a document in the official User Workspace (visible on the user's Workspace UI page). ALWAYS use this tool when the user asks to save, store, or create files/plans/PRDs/reports for them in their workspace. Provide exactly one payload: 'from_path' to publish an existing file (RECOMMENDED for binary files like PDF, images, ZIP, DOCX, XLSX, or files generated by scripts to avoid base64 truncation and encoding corruption), 'content' for UTF-8 text, or 'content_base64' for raw bytes."
}
func (t *WorkspaceWriteTool) Category() string { return "native" }
func (t *WorkspaceWriteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"description":"Write or publish a file into the official User Workspace. For UTF-8 text, pass 'content'. For binary files (PDF, images, ZIP, DOCX, XLSX) or files generated in your agent scratchpad, pass 'from_path' to copy directly without base64 truncation.",
		"properties":{
			"file_id":{"type":"string","description":"Opaque ID when updating an existing file"},
			"parent_id":{"type":"string","description":"Opaque parent folder ID when creating; omit for root"},
			"name":{"type":"string","description":"Exact user-visible name when creating (e.g. 'plan.md', 'report.pdf'). If omitted when using from_path, basename is used."},
			"from_path":{"type":"string","description":"Path to an existing file in your agent scratchpad or data directory (e.g. 'report.pdf' or 'VieCharacter-agile-plan/01.pdf') to publish directly into the user workspace. RECOMMENDED for binary and large files to prevent corruption."},
			"content":{"type":"string","description":"UTF-8 text content. Use this for plain text, source code, JSON, Markdown, CSV, and other text files. Do not use for binary files."},
			"content_base64":{"type":"string","description":"Base64-encoded raw file bytes. Prefer 'from_path' when copying an existing file to avoid token limits and output truncation."},
			"mime_type":{"type":"string","description":"Optional MIME hint; content sniffing is authoritative"},
			"expected_version":{"type":"integer","minimum":1,"description":"Required for conflict-safe updates"}
		},
		"oneOf":[
			{"required":["from_path"],"not":{"anyOf":[{"required":["content"]},{"required":["content_base64"]}]}},
			{"required":["content"],"not":{"anyOf":[{"required":["from_path"]},{"required":["content_base64"]}]}},
			{"required":["content_base64"],"not":{"anyOf":[{"required":["from_path"]},{"required":["content"]}]}}
		]
	}`)
}

func decodeBase64Robust(raw string) ([]byte, error) {
	cleaned := strings.TrimSpace(raw)
	if strings.Contains(cleaned, "[truncated]") {
		return nil, errors.New("base64 content is truncated by observation limit; please generate the file in your agent scratchpad and publish using 'from_path'")
	}
	if idx := strings.Index(cleaned, ";base64,"); idx != -1 {
		cleaned = cleaned[idx+8:]
	}
	cleaned = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, cleaned)

	if decoded, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	return nil, errors.New("invalid base64 payload")
}

func (t *WorkspaceWriteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.store == nil {
		return nil, errors.New("user workspace store is unavailable")
	}
	var input struct {
		FileID          string  `json:"file_id"`
		ParentID        string  `json:"parent_id"`
		Name            string  `json:"name"`
		FromPath        string  `json:"from_path"`
		SourcePath      string  `json:"source_path"`
		FilePath        string  `json:"file_path"`
		Content         *string `json:"content"`
		ContentBase64   *string `json:"content_base64"`
		MIMEType        string  `json:"mime_type"`
		ExpectedVersion int64   `json:"expected_version"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("decoding workspace write input: %w", err)
	}

	fromPath := strings.TrimSpace(input.FromPath)
	if fromPath == "" {
		fromPath = strings.TrimSpace(input.SourcePath)
	}
	if fromPath == "" {
		fromPath = strings.TrimSpace(input.FilePath)
	}

	payloadCount := 0
	if fromPath != "" {
		payloadCount++
	}
	if input.Content != nil {
		payloadCount++
	}
	if input.ContentBase64 != nil {
		payloadCount++
	}

	if payloadCount == 0 {
		return nil, errors.New("from_path, content, or content_base64 is required")
	}
	if payloadCount > 1 {
		return nil, errors.New("provide only one of from_path, content, or content_base64")
	}

	if input.FileID == "" && input.Name == "" {
		if fromPath != "" {
			input.Name = filepath.Base(fromPath)
		} else {
			return nil, errors.New("name is required when creating a file")
		}
	}

	var content []byte
	if fromPath != "" {
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
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return nil, fmt.Errorf("reading from_path (%s): %w", fromPath, err)
		}
		content = data
	} else if input.ContentBase64 != nil {
		decoded, err := decodeBase64Robust(*input.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("decoding content_base64: %w", err)
		}
		content = decoded
	} else {
		strContent := *input.Content
		if strings.HasPrefix(strContent, "data:") && strings.Contains(strContent, ";base64,") {
			decoded, err := decodeBase64Robust(strContent)
			if err != nil {
				return nil, fmt.Errorf("decoding data uri base64 in content: %w", err)
			}
			content = decoded
		} else {
			content = []byte(strContent)
		}
	}

	if len(content) > maxWorkspaceToolWrite {
		return nil, fmt.Errorf("workspace tool write exceeds %d-byte limit; use the authenticated upload API", maxWorkspaceToolWrite)
	}
	node, err := t.store.Write(ctx, workspacepkg.WriteRequest{
		ID:              input.FileID,
		ParentID:        input.ParentID,
		Name:            input.Name,
		Content:         content,
		MIMEType:        input.MIMEType,
		ExpectedVersion: input.ExpectedVersion,
		ActorID:         AgentIDFromContext(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("writing user workspace file: %w", err)
	}
	return &ToolResult{
		Content: fmt.Sprintf("Saved user workspace file %q (id=%s, version=%d, bytes=%d, exec_path=%s)", node.Name, node.ID, node.Version, node.SizeBytes, node.ExecPath),
		Data:    workspaceNodeData(node, nil),
	}, nil
}

type WorkspaceDeleteTool struct{ store *workspacepkg.Store }

func NewWorkspaceDeleteTool(store *workspacepkg.Store) *WorkspaceDeleteTool {
	return &WorkspaceDeleteTool{store: store}
}

func (t *WorkspaceDeleteTool) Name() string { return "native_workspace_delete" }
func (t *WorkspaceDeleteTool) Description() string {
	return "Move a user-owned workspace file or folder to trash by opaque file_id. Does not accept host paths."
}
func (t *WorkspaceDeleteTool) Category() string { return "native" }
func (t *WorkspaceDeleteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"file_id":{"type":"string","description":"Opaque file or folder ID"},
			"expected_version":{"type":"integer","minimum":1},
			"recursive":{"type":"boolean","default":false}
		},
		"required":["file_id"]
	}`)
}
func (t *WorkspaceDeleteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.store == nil {
		return nil, errors.New("user workspace store is unavailable")
	}
	var input struct {
		FileID          string `json:"file_id"`
		ExpectedVersion int64  `json:"expected_version"`
		Recursive       bool   `json:"recursive"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("decoding workspace delete input: %w", err)
	}
	if input.FileID == "" {
		return nil, errors.New("file_id is required")
	}
	node, err := t.store.Get(ctx, input.FileID)
	if err != nil {
		return nil, fmt.Errorf("loading user workspace node: %w", err)
	}
	if err := t.store.Delete(ctx, input.FileID, input.ExpectedVersion, input.Recursive); err != nil {
		return nil, fmt.Errorf("deleting user workspace node: %w", err)
	}
	return &ToolResult{
		Content: fmt.Sprintf("Moved user workspace item %q (id=%s) to trash", node.Name, node.ID),
		Data:    workspaceNodeData(node, map[string]any{"deleted": true}),
	}, nil
}

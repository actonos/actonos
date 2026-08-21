package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

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
	return "Search or browse user-owned workspace files. SQLite stores metadata only; file bytes remain on the workspace filesystem."
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
	return "Read a user-owned workspace file by opaque file_id. Never accepts or returns a host filesystem path."
}
func (t *WorkspaceReadTool) Category() string { return "native" }
func (t *WorkspaceReadTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"file_id":{"type":"string","description":"Opaque file ID returned by native_workspace_search"},
			"offset":{"type":"integer","minimum":0,"default":0},
			"limit":{"type":"integer","minimum":1,"maximum":1048576,"default":65536},
			"encoding":{"type":"string","enum":["auto","utf8","base64"],"default":"auto"}
		},
		"required":["file_id"]
	}`)
}
func (t *WorkspaceReadTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.store == nil {
		return nil, errors.New("user workspace store is unavailable")
	}
	var input struct {
		FileID   string `json:"file_id"`
		Offset   int64  `json:"offset"`
		Limit    int64  `json:"limit"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("decoding workspace read input: %w", err)
	}
	if input.FileID == "" {
		return nil, errors.New("file_id is required")
	}
	if input.Limit == 0 {
		input.Limit = defaultWorkspaceReadLimit
	}
	if input.Offset < 0 || input.Limit < 1 || input.Limit > maxWorkspaceReadLimit {
		return nil, fmt.Errorf("invalid read range: limit must be between 1 and %d", maxWorkspaceReadLimit)
	}
	node, content, err := t.store.Read(ctx, input.FileID, input.Offset, input.Limit)
	if err != nil {
		return nil, fmt.Errorf("reading user workspace file: %w", err)
	}
	encoding := strings.ToLower(input.Encoding)
	if encoding == "" || encoding == "auto" {
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
		Data: map[string]any{
			"file_id":        node.ID,
			"name":           node.Name,
			"virtual_path":   node.VirtualPath,
			"mime_type":      node.MIMEType,
			"version":        node.Version,
			"size_bytes":     node.SizeBytes,
			"offset":         input.Offset,
			"returned_bytes": len(content),
			"next_offset":    input.Offset + int64(len(content)),
			"truncated":      truncated,
			"encoding":       encoding,
		},
	}, nil
}

type WorkspaceWriteTool struct{ store *workspacepkg.Store }

func NewWorkspaceWriteTool(store *workspacepkg.Store) *WorkspaceWriteTool {
	return &WorkspaceWriteTool{store: store}
}

func (t *WorkspaceWriteTool) Name() string { return "native_workspace_write" }
func (t *WorkspaceWriteTool) Description() string {
	return "Create or update a user-owned workspace file. Provide exactly one payload: content for UTF-8 text (including code, JSON, Markdown, and CSV), or content_base64 only for binary/non-UTF-8 bytes such as PDF, images, archives, and office files. Never send both fields. Create with parent_id and name; update with file_id and expected_version."
}
func (t *WorkspaceWriteTool) Category() string { return "native" }
func (t *WorkspaceWriteTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"description":"Write exactly one content representation. Use content for UTF-8 text. Use content_base64 only when the file bytes are binary or not valid UTF-8 (for example PDF, image, ZIP, or DOCX).",
		"properties":{
			"file_id":{"type":"string","description":"Opaque ID when updating an existing file"},
			"parent_id":{"type":"string","description":"Opaque parent folder ID when creating; omit for root"},
			"name":{"type":"string","description":"Exact user-visible name when creating; extension is optional"},
			"content":{"type":"string","description":"UTF-8 text content. Use this for plain text, source code, JSON, Markdown, CSV, and other text files. Do not use for binary files."},
			"content_base64":{"type":"string","description":"Base64-encoded raw file bytes. Use this only for binary or non-UTF-8 files such as PDF, images, ZIP, DOCX, and XLSX. Do not use for ordinary text."},
			"mime_type":{"type":"string","description":"Optional MIME hint; content sniffing is authoritative"},
			"expected_version":{"type":"integer","minimum":1,"description":"Required for conflict-safe updates"}
		},
		"oneOf":[
			{"required":["content"],"not":{"required":["content_base64"]}},
			{"required":["content_base64"],"not":{"required":["content"]}}
		]
	}`)
}
func (t *WorkspaceWriteTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.store == nil {
		return nil, errors.New("user workspace store is unavailable")
	}
	var input struct {
		FileID          string  `json:"file_id"`
		ParentID        string  `json:"parent_id"`
		Name            string  `json:"name"`
		Content         *string `json:"content"`
		ContentBase64   *string `json:"content_base64"`
		MIMEType        string  `json:"mime_type"`
		ExpectedVersion int64   `json:"expected_version"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("decoding workspace write input: %w", err)
	}
	if input.FileID == "" && input.Name == "" {
		return nil, errors.New("name is required when creating a file")
	}
	if input.Content == nil && input.ContentBase64 == nil {
		return nil, errors.New("content or content_base64 is required")
	}
	if input.Content != nil && input.ContentBase64 != nil {
		return nil, errors.New("provide only one of content or content_base64")
	}
	var content []byte
	if input.ContentBase64 != nil {
		decoded, err := base64.StdEncoding.DecodeString(*input.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("decoding content_base64: %w", err)
		}
		content = decoded
	} else {
		content = []byte(*input.Content)
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
		Content: fmt.Sprintf("Saved user workspace file %q (id=%s, version=%d, bytes=%d)", node.Name, node.ID, node.Version, node.SizeBytes),
		Data: map[string]any{
			"workspace_file_id": node.ID,
			"file_id":           node.ID,
			"name":              node.Name,
			"virtual_path":      node.VirtualPath,
			"mime_type":         node.MIMEType,
			"version":           node.Version,
			"size_bytes":        node.SizeBytes,
		},
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
		Data: map[string]any{
			"workspace_file_id": node.ID,
			"file_id":           node.ID,
			"name":              node.Name,
			"virtual_path":      node.VirtualPath,
			"deleted":           true,
		},
	}, nil
}

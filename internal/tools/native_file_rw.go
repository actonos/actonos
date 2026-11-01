package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/actonos/actonos/internal/security"
)

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
	return "Read internal scratchpad files, local logs, or skill files inside the agent's private directory or system data directory."
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
	return "Write internal scratchpad files, local logs, build artifacts, or temporary scripts inside the agent's private directory. NOTE: Files written here are internal agent scratchpads and are NOT visible to the user on their Workspace page; use native_workspace_write to save user-facing files."
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

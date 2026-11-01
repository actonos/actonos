package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/actonos/actonos/internal/security"
)

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

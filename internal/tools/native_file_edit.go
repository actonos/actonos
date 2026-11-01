package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/actonos/actonos/internal/security"
)

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

	normFile := strings.ReplaceAll(fileContent, "\r\n", "\n")
	normTarget := strings.ReplaceAll(target, "\r\n", "\n")
	normRepl := strings.ReplaceAll(replacement, "\r\n", "\n")

	edited, err := performFuzzyLineEdit(normFile, normTarget, normRepl, input.StartLine, input.EndLine, input.AllowMultiple, relPath)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(targetPath, []byte(edited), 0644); err != nil {
		return nil, fmt.Errorf("writing edited file: %w", err)
	}

	return &ToolResult{
		Content: fmt.Sprintf("Successfully edited %s. Target content replaced with new content (%d bytes).", relPath, len(edited)),
		Data: map[string]any{
			"path":  relPath,
			"bytes": len(edited),
		},
	}, nil
}

// performFuzzyLineEdit replaces normTarget with normRepl in normFile with multiple progressive matching strategies.
func performFuzzyLineEdit(normFile, normTarget, normRepl string, startLine, endLine int, allowMultiple bool, relPath string) (string, error) {
	lines := strings.Split(normFile, "\n")
	start := startLine
	if start <= 0 {
		start = 1
	}
	end := endLine
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return "", fmt.Errorf("start_line %d exceeds total file lines %d", start, len(lines))
	}

	subLines := lines[start-1 : end]
	subContent := strings.Join(subLines, "\n")

	// Strategy 1: Exact substring match
	count := strings.Count(subContent, normTarget)
	if count > 0 {
		if count > 1 && !allowMultiple {
			if startLine > 0 || endLine > 0 {
				return "", fmt.Errorf("found %d occurrences of target_content between lines %d and %d in %s; specify a narrower range or set allow_multiple: true", count, start, end, relPath)
			}
			return "", fmt.Errorf("found %d occurrences of target_content in %s; specify start_line/end_line or set allow_multiple: true", count, relPath)
		}
		newSub := subContent
		if allowMultiple {
			newSub = strings.ReplaceAll(subContent, normTarget, normRepl)
		} else {
			newSub = strings.Replace(subContent, normTarget, normRepl, 1)
		}
		return rebuildFullContent(lines, start, end, newSub), nil
	}

	// Helper to split lines cleanly (stripping a trailing single newline if present)
	splitCleanLines := func(s string) []string {
		trimmed := strings.TrimSuffix(s, "\n")
		trimmed = strings.TrimSuffix(trimmed, "\r")
		return strings.Split(trimmed, "\n")
	}

	targetLines := splitCleanLines(normTarget)
	targetLen := len(targetLines)

	// Strategy 2: Line-by-line trailing whitespace trimmed match
	if targetLen > 0 && len(subLines) >= targetLen {
		var matches []int
		for i := 0; i <= len(subLines)-targetLen; i++ {
			match := true
			for j := 0; j < targetLen; j++ {
				if strings.TrimRight(subLines[i+j], " \t\r") != strings.TrimRight(targetLines[j], " \t\r") {
					match = false
					break
				}
			}
			if match {
				matches = append(matches, i)
			}
		}

		if len(matches) > 0 {
			if len(matches) > 1 && !allowMultiple {
				return "", fmt.Errorf("found %d occurrences of target_content in %s (ignoring trailing whitespace); specify start_line/end_line or set allow_multiple: true", len(matches), relPath)
			}
			newLines := make([]string, len(subLines))
			copy(newLines, subLines)
			if !allowMultiple {
				matches = matches[:1]
			}
			replLines := splitCleanLines(normRepl)
			for k := len(matches) - 1; k >= 0; k-- {
				idx := matches[k]
				prefix := newLines[:idx]
				suffix := newLines[idx+targetLen:]
				newLines = append(append(prefix, replLines...), suffix...)
			}
			return rebuildFullContent(lines, start, end, strings.Join(newLines, "\n")), nil
		}
	}

	// Strategy 3: TrimSpace match (matching trimmed lines, ignoring indentation & trailing whitespace)
	if targetLen > 0 && len(subLines) >= targetLen {
		var matches []int
		for i := 0; i <= len(subLines)-targetLen; i++ {
			match := true
			for j := 0; j < targetLen; j++ {
				if strings.TrimSpace(subLines[i+j]) != strings.TrimSpace(targetLines[j]) {
					match = false
					break
				}
			}
			if match {
				matches = append(matches, i)
			}
		}

		if len(matches) > 0 {
			if len(matches) > 1 && !allowMultiple {
				return "", fmt.Errorf("found %d occurrences of target_content in %s (approximate whitespace); specify start_line/end_line or set allow_multiple: true", len(matches), relPath)
			}
			newLines := make([]string, len(subLines))
			copy(newLines, subLines)
			if !allowMultiple {
				matches = matches[:1]
			}
			replLines := splitCleanLines(normRepl)
			for k := len(matches) - 1; k >= 0; k-- {
				idx := matches[k]
				prefix := newLines[:idx]
				suffix := newLines[idx+targetLen:]
				newLines = append(append(prefix, replLines...), suffix...)
			}
			return rebuildFullContent(lines, start, end, strings.Join(newLines, "\n")), nil
		}
	}

	// Strategy 4: Trimmed outer blank lines
	trimmedTarget := strings.Trim(normTarget, "\n\r")
	if trimmedTarget != "" && trimmedTarget != normTarget {
		count := strings.Count(subContent, trimmedTarget)
		if count > 0 {
			if count > 1 && !allowMultiple {
				return "", fmt.Errorf("found %d occurrences of target_content in %s; specify start_line/end_line or set allow_multiple: true", count, relPath)
			}
			trimmedRepl := strings.Trim(normRepl, "\n\r")
			newSub := subContent
			if allowMultiple {
				newSub = strings.ReplaceAll(subContent, trimmedTarget, trimmedRepl)
			} else {
				newSub = strings.Replace(subContent, trimmedTarget, trimmedRepl, 1)
			}
			return rebuildFullContent(lines, start, end, newSub), nil
		}
	}

	// Strategy 5: Helpful diagnostics if not found
	firstTargetLine := ""
	for _, tl := range targetLines {
		if strings.TrimSpace(tl) != "" {
			firstTargetLine = strings.TrimSpace(tl)
			break
		}
	}
	closestLine := 0
	if firstTargetLine != "" {
		for i, l := range subLines {
			if strings.Contains(strings.TrimSpace(l), firstTargetLine) || strings.Contains(firstTargetLine, strings.TrimSpace(l)) {
				closestLine = start + i
				break
			}
		}
	}

	if closestLine > 0 {
		return "", fmt.Errorf("target_content not found in %s; a similar line was found near line %d: %q. Please inspect the exact file contents using native_file_read before editing", relPath, closestLine, strings.TrimSpace(lines[closestLine-1]))
	}

	if startLine > 0 || endLine > 0 {
		return "", fmt.Errorf("target_content not found between lines %d and %d in %s; please use native_file_read to inspect the range", start, end, relPath)
	}

	return "", fmt.Errorf("target_content not found in %s; please check exact line numbers and contents with native_file_read before editing", relPath)
}

func rebuildFullContent(lines []string, start, end int, newSubContent string) string {
	prefix := ""
	if start > 1 {
		prefix = strings.Join(lines[:start-1], "\n") + "\n"
	}
	suffix := ""
	if end < len(lines) {
		suffix = "\n" + strings.Join(lines[end:], "\n")
	}
	return prefix + newSubContent + suffix
}

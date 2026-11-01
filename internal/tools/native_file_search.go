package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/actonos/actonos/internal/security"
)

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

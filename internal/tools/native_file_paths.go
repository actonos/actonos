package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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

// resolveTargetBaseDir returns the allowed root (dataDir), base directory, and target relative path
// for path resolution. Paths targeting known system directories (skills/, config/, plugins/, logs/,
// storage/, agents/, models/, overrides/, vectors/) resolve relative to the data root. All other
// paths are automatically scoped to the calling agent's private workspace directory
// (agents/{agentID}/workspace/) so that agents write to their own workspace by default.
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
		// Empty or root path: resolve to agent workspace
		agentID := AgentIDFromContext(ctx)
		if validAgentWorkspaceSlug(agentID) {
			agentWs := filepath.Join(absDataDir, "agents", agentID, "workspace")
			if err := os.MkdirAll(agentWs, 0750); err != nil {
				return "", "", "", fmt.Errorf("creating agent workspace: %w", err)
			}
			return absDataDir, agentWs, ".", nil
		}
		return absDataDir, absDataDir, ".", nil
	}

	// Check if the path explicitly targets a known system directory that lives
	// directly under the data root. These are passed through verbatim.
	systemPrefixes := []string{
		"skills/", "config/", "plugins/", "logs/", "storage/",
		"agents/", "models/", "overrides/", "vectors/", "workspace/",
	}
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(cleanReq, prefix) || cleanReq == strings.TrimSuffix(prefix, "/") {
			return absDataDir, absDataDir, cleanReq, nil
		}
	}

	// All other paths are scoped to the calling agent's private workspace.
	agentID := AgentIDFromContext(ctx)
	if validAgentWorkspaceSlug(agentID) {
		agentWs := filepath.Join(absDataDir, "agents", agentID, "workspace")
		if err := os.MkdirAll(agentWs, 0750); err != nil {
			return "", "", "", fmt.Errorf("creating agent workspace: %w", err)
		}
		return absDataDir, agentWs, cleanReq, nil
	}

	// No agent context: fall back to data root
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

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ToolCapability represents a detected system CLI tool or runtime on the host.
type ToolCapability struct {
	Name        string
	Binary      string
	Description string
	Category    string
	Installed   bool
}

// ProbeSystemCapabilities checks which batteries-included CLI tools and runtimes are installed on the host.
func ProbeSystemCapabilities() []ToolCapability {
	candidates := []ToolCapability{
		{Name: "ripgrep", Binary: "rg", Category: "Search & Code", Description: "Ultra-fast regex code & text search across directories"},
		{Name: "jq", Binary: "jq", Category: "Data Processing", Description: "Lightweight command-line JSON processor & parser"},
		{Name: "sqlite3", Binary: "sqlite3", Category: "Database", Description: "CLI interface for SQLite database inspection & queries"},
		{Name: "python3", Binary: "python3", Category: "Runtime", Description: "Python 3 runtime for math, scripting, data parsing & automation"},
		{Name: "node", Binary: "node", Category: "Runtime", Description: "Node.js JavaScript/TypeScript runtime"},
		{Name: "npm", Binary: "npm", Category: "Package Manager", Description: "Node package manager for running scripts & tooling"},
		{Name: "chromium", Binary: "chromium", Category: "Browser Automation", Description: "Headless Chromium browser engine for web scraping & screenshots"},
		{Name: "curl", Binary: "curl", Category: "Networking", Description: "Command-line tool for HTTP/HTTPS requests & API calls"},
		{Name: "wget", Binary: "wget", Category: "Networking", Description: "File retrieval utility over HTTP/HTTPS/FTP"},
		{Name: "git", Binary: "git", Category: "VCS", Description: "Git version control and repository management"},
		{Name: "bubblewrap", Binary: "bwrap", Category: "Sandboxing", Description: "Unprivileged user namespace sandboxing container engine"},
		{Name: "tree", Binary: "tree", Category: "Filesystem", Description: "Recursive directory structure visualization"},
		{Name: "tar", Binary: "tar", Category: "Archive", Description: "Tape archive utility for tar/gzip compression & extraction"},
		{Name: "unzip", Binary: "unzip", Category: "Archive", Description: "Zip file extraction utility"},
	}

	for i := range candidates {
		if _, err := exec.LookPath(candidates[i].Binary); err == nil {
			candidates[i].Installed = true
		} else if candidates[i].Binary == "chromium" {
			if _, err := exec.LookPath("google-chrome"); err == nil {
				candidates[i].Installed = true
				candidates[i].Binary = "google-chrome"
			} else if _, err := exec.LookPath("chromium-browser"); err == nil {
				candidates[i].Installed = true
				candidates[i].Binary = "chromium-browser"
			}
		} else if candidates[i].Binary == "python3" {
			if _, err := exec.LookPath("python"); err == nil {
				candidates[i].Installed = true
				candidates[i].Binary = "python"
			}
		}
	}

	return candidates
}

// scanWorkspaceOverview builds a shallow tree summary (depth 1-2) of the workspace to orient the agent.
func scanWorkspaceOverview(workspaceDir string) string {
	if workspaceDir == "" {
		return ""
	}
	absDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(absDir)
	if err != nil || len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		// Filter noise
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" || name == "target" || name == "tmp" {
			continue
		}
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("  - %s/ (directory)\n", name))
			// Sub-entries (depth 2)
			subEntries, _ := os.ReadDir(filepath.Join(absDir, name))
			subCount := 0
			for _, sub := range subEntries {
				subName := sub.Name()
				if strings.HasPrefix(subName, ".") || subName == "node_modules" {
					continue
				}
				if sub.IsDir() {
					sb.WriteString(fmt.Sprintf("    - %s/\n", subName))
				} else {
					sb.WriteString(fmt.Sprintf("    - %s\n", subName))
				}
				subCount++
				if subCount >= 8 {
					sb.WriteString("    - ...\n")
					break
				}
			}
		} else {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		count++
		if count >= 15 {
			sb.WriteString("  - ... (more files in workspace)\n")
			break
		}
	}

	return sb.String()
}

// BuildHostEnvironmentPrompt generates a structured XML-tagged prompt block informing the Agent
// of its exact Host Operating System, Shell syntax, Workspace layout, and Available CLI tools.
func BuildHostEnvironmentPrompt(workspaceDir string) string {
	dataDir := filepath.Dir(workspaceDir)
	if filepath.Base(workspaceDir) != "workspace" {
		dataDir = workspaceDir
		workspaceDir = filepath.Join(dataDir, "workspace")
	}
	return BuildAgentEnvironmentPrompt(dataDir, workspaceDir, DefaultSystemAgentID)
}

// BuildAgentEnvironmentPrompt describes the metadata-backed user workspace and
// the calling agent's isolated filesystem workspace. workspaceDir is retained
// for call-site compatibility but is never exposed as a host filesystem path.
func BuildAgentEnvironmentPrompt(dataDir, workspaceDir, agentSlug string) string {
	if dataDir == "" && workspaceDir == "" {
		dataDir = "./data"
		workspaceDir = "./data/workspace"
	} else if dataDir == "" {
		if filepath.Base(workspaceDir) == "workspace" {
			dataDir = filepath.Dir(workspaceDir)
		} else {
			dataDir = workspaceDir
			workspaceDir = filepath.Join(dataDir, "workspace")
		}
	} else if workspaceDir == "" {
		workspaceDir = filepath.Join(dataDir, "workspace")
	}

	if agentSlug == "" {
		agentSlug = DefaultSystemAgentID
	}

	var sb strings.Builder

	// 1. Operating System & Architecture
	osName := runtime.GOOS
	arch := runtime.GOARCH
	runtimeType := "Bare-metal Linux Kernel Appliance"
	if _, err := os.Stat("/.dockerenv"); err == nil {
		runtimeType = "Docker Container"
	} else if osName == "windows" {
		runtimeType = "Windows Host"
	} else if osName == "darwin" {
		runtimeType = "macOS Host"
	}

	sb.WriteString("<environment>\n")
	fmt.Fprintf(&sb, "  <os name=\"%s\" arch=\"%s\" runtime=\"%s\" />\n", strings.Title(osName), arch, runtimeType)

	if osName == "windows" {
		sb.WriteString("  <shell path=\"PowerShell\" syntax=\"Windows / PowerShell (cmdlets, backslashes '\\')\" />\n")
	} else {
		sb.WriteString("  <shell path=\"/bin/bash\" syntax=\"POSIX / Bash (pipes '|', forward slashes '/', redirection)\" />\n")
	}

	configDir := filepath.Join(dataDir, "config")
	logsDir := filepath.Join(dataDir, "logs")
	skillsDir := filepath.Join(dataDir, "skills")
	storageDir := filepath.Join(dataDir, "storage")
	pluginsDir := filepath.Join(dataDir, "plugins")
	agentWorkspaceRel := filepath.ToSlash(filepath.Join("agents", agentSlug, "workspace"))
	agentWorkspaceAbs := filepath.Join(dataDir, "agents", agentSlug, "workspace")

	sb.WriteString("  <filesystem_layout>\n")
	fmt.Fprintf(&sb, "    <user_workspace virtual_root=\"/data/workspace\" storage=\"user_documents_db\">\n")
	sb.WriteString("      OFFICIAL USER WORKSPACE (VISIBLE TO USER IN WEB UI): This is the user's primary document repository shown on their Workspace page. Whenever the user asks to save, store, create, read, search, or delete files in their workspace (e.g. 'lưu vào workspace', 'tạo file trong workspace', 'lưu tài liệu cho tôi', 'đọc workspace'), you MUST use the `native_workspace_*` tools (`native_workspace_write`, `native_workspace_read`, `native_workspace_search`, `native_workspace_delete`).\n")
	sb.WriteString("    </user_workspace>\n")
	fmt.Fprintf(&sb, "    <agent_private_scratchpad path=\"%s\" host_dir=\"%s\">\n", agentWorkspaceRel, agentWorkspaceAbs)
	sb.WriteString("      AGENT INTERNAL SCRATCHPAD (HIDDEN FROM USER UI): This is your private internal scratchpad for temporary build scripts, intermediate calculation files, or raw CLI tools. Use `native_file_*` and `native_exec` for internal working steps only. Final deliverables meant for the user MUST be saved to the User Workspace via `native_workspace_write`.\n")
	sb.WriteString("    </agent_private_scratchpad>\n")
	fmt.Fprintf(&sb, "    <system_data_root path=\".\" host_dir=\"%s\" config=\"%s\" logs=\"%s\" skills=\"%s\" storage=\"%s\" plugins=\"%s\">\n",
		dataDir, configDir, logsDir, skillsDir, storageDir, pluginsDir)
	sb.WriteString("      System configuration directories exist under data root. Access them explicitly with their directory prefix (e.g. 'skills/my_skill/...'). Do NOT put arbitrary user project files directly in the data root.\n")
	sb.WriteString("    </system_data_root>\n")
	sb.WriteString("  </filesystem_layout>\n")

	// 2. Probed System Capabilities & Batteries-Included CLI Tools
	caps := ProbeSystemCapabilities()
	sb.WriteString("  <installed_runtimes_and_tools>\n")
	for _, cap := range caps {
		if cap.Installed {
			fmt.Fprintf(&sb, "    <tool name=\"%s\" binary=\"%s\" category=\"%s\" usage=\"%s\" />\n",
				cap.Name, cap.Binary, cap.Category, cap.Description)
		}
	}
	sb.WriteString("  </installed_runtimes_and_tools>\n")

	// 3. Workspace Overview State of the Agent's private workspace
	wsOverview := scanWorkspaceOverview(agentWorkspaceAbs)
	if wsOverview != "" {
		sb.WriteString("  <workspace_state path=\"" + agentWorkspaceRel + "\">\n")
		sb.WriteString(wsOverview)
		sb.WriteString("  </workspace_state>\n")
	}

	sb.WriteString("</environment>\n\n")

	// 4. Dedicated Storage Policy & Global Data Access
	sb.WriteString("<workspace_storage_policy>\n")
	sb.WriteString("  <rule id=\"user_workspace_mandate\">USER WORKSPACE (MANDATORY): Whenever interacting with user documents, or when the user says 'lưu vào workspace', 'save to workspace', 'tạo tài liệu', or asks to access their workspace files, you MUST use `native_workspace_*` tools (`native_workspace_write`, `native_workspace_read`, `native_workspace_search`, `native_workspace_delete`). Files saved here appear directly on the user's Workspace Page UI.</rule>\n")
	fmt.Fprintf(&sb, "  <rule id=\"agent_internal_scratchpad\">AGENT SCRATCHPAD: Use `native_file_*` tools and `native_exec` ONLY for internal temporary scripts, building/compiling code, or running CLI tools in `%s/`. Do NOT expect the user to see files created by `native_file_write` unless they are published to the user workspace with `native_workspace_write`.</rule>\n", agentWorkspaceRel)
	sb.WriteString("  <rule id=\"binary_files_publishing\">BINARY & LARGE FILES: When generating binary files (PDF, images, ZIP, DOCX, XLSX, audio) or multi-step generated documents, build the file in your private scratchpad (`" + agentWorkspaceRel + "/`), then publish it to the User Workspace using `native_workspace_write` with `from_path: \"<relative_path>\"` (e.g. `native_workspace_write(name=\"plan.pdf\", from_path=\"plan.pdf\")`). This guarantees 100% lossless binary publishing without base64 truncation or encoding corruption.</rule>\n")
	sb.WriteString("  <rule id=\"system_structure\">SYSTEM STRUCTURE: System-level directories ('skills/', 'config/', 'plugins/', 'logs/', 'storage/') exist under data root. Only write to 'skills/' when explicitly creating/managing skills, and 'config/' for system configurations.</rule>\n")
	sb.WriteString("</workspace_storage_policy>\n\n")

	// 5. Dynamic Execution Best Practices based on detected environment tools
	hasTool := func(bin string) bool {
		for _, c := range caps {
			if c.Installed && (c.Binary == bin || c.Name == bin) {
				return true
			}
		}
		return false
	}

	sb.WriteString("<execution_best_practices>\n")
	sb.WriteString("  <practice>Editing Code: ALWAYS prefer `native_file_edit` for modifying existing code or text rather than rewriting the entire file with `native_file_write`.</practice>\n")
	sb.WriteString("  <practice>Inspecting Code: Use `native_file_read` with `start_line` and `end_line` to inspect specific sections with line numbers, avoiding context window bloat.</practice>\n")
	sb.WriteString("  <practice>File Search & Discovery: Use `native_file_search` without query to explore directory trees, and with `query` (plus optional `context_lines` and `is_regex`) to grep code instantly.</practice>\n")
	if hasTool("rg") || hasTool("ripgrep") {
		sb.WriteString("  <practice>Search Efficiency: ALWAYS prioritize `ripgrep` (`rg`) for rapid regex/text searches over iterating through files manually.</practice>\n")
	} else {
		sb.WriteString("  <practice>Search Efficiency: Utilize native file search tools or concise Python/PowerShell scripts to search and inspect files efficiently.</practice>\n")
	}
	if hasTool("jq") {
		sb.WriteString("  <practice>JSON Extraction: Utilize `jq` for filtering and extracting structured data from API responses.</practice>\n")
	} else if hasTool("python") || hasTool("python3") || hasTool("node") {
		sb.WriteString("  <practice>JSON Extraction: Utilize concise Python (`python3 -c \"import sys, json...\"`) or Node.js scripts for filtering and extracting structured data.</practice>\n")
	}
	if hasTool("python") || hasTool("python3") || hasTool("node") {
		sb.WriteString("  <practice>Computation & Parsing: Prefer executing concise Python (`python3 -c \"...\"`) or Node.js scripts for mathematical calculations, regex transformations, or data conversions rather than guessing.</practice>\n")
	}
	sb.WriteString("</execution_best_practices>\n")

	return sb.String()
}

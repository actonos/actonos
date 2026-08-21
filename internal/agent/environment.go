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

// BuildAgentEnvironmentPrompt describes the database-backed user workspace and
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

	agentWorkspace := filepath.Join(dataDir, "agents", agentSlug, "workspace")
	_ = os.MkdirAll(agentWorkspace, 0755)

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

	fmt.Fprintf(&sb, "  <workspace user_virtual_root=\"/data/workspace\" storage=\"sqlite\" agent_dir=\"%s\" agent_slug=\"%s\" />\n", agentWorkspace, agentSlug)
	fmt.Fprintf(&sb, "  <data_dir path=\"%s\" config=\"%s\" logs=\"%s\" skills=\"%s\" storage=\"%s\" plugins=\"%s\" />\n",
		dataDir, configDir, logsDir, skillsDir, storageDir, pluginsDir)

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

	// 3. Workspace Overview State (if available)
	wsOverview := scanWorkspaceOverview(agentWorkspace)
	if wsOverview != "" {
		sb.WriteString("  <workspace_state>\n")
		sb.WriteString(wsOverview)
		sb.WriteString("  </workspace_state>\n")
	}

	sb.WriteString("</environment>\n\n")

	// 4. Dedicated Storage Policy & Global Data Access
	sb.WriteString("<workspace_storage_policy>\n")
	fmt.Fprintf(&sb, "  <rule id=\"dedicated_workspace_default\">PRIVATE WORKSPACE: Use native_file_* and native_exec only for your own working files under `%s`. Other agents have separate directories and must never be accessed.</rule>\n", agentWorkspace)
	sb.WriteString("  <rule id=\"user_workspace_database\">USER WORKSPACE: `/data/workspace` is a virtual, SQLite-backed namespace. Use only native_workspace_search, native_workspace_read, native_workspace_write, and native_workspace_delete for user files. These tools accept opaque file IDs; never guess or pass a host path.</rule>\n")
	fmt.Fprintf(&sb, "  <rule id=\"system_data_access\">SYSTEM DATA: Skills and configuration live under `%s` and `%s`; access them only with a tool explicitly authorized for that purpose.</rule>\n", skillsDir, configDir)
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

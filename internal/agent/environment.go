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
	if workspaceDir == "" {
		if _, err := os.Stat("/data/workspace"); err == nil {
			workspaceDir = "/data/workspace"
		} else {
			workspaceDir = "."
		}
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

	fmt.Fprintf(&sb, "  <workspace root=\"%s\" />\n", workspaceDir)
	sb.WriteString("  <data_dir path=\"/data\" config=\"/data/config\" logs=\"/data/logs\" skills=\"/data/skills\" />\n")

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
	wsOverview := scanWorkspaceOverview(workspaceDir)
	if wsOverview != "" {
		sb.WriteString("  <workspace_state>\n")
		sb.WriteString(wsOverview)
		sb.WriteString("  </workspace_state>\n")
	}

	sb.WriteString("</environment>\n\n")

	// 4. Operational Constraints & Anti-Hanging Safety Rules
	sb.WriteString("<operational_constraints>\n")
	sb.WriteString("  <rule id=\"no_interactive_commands\">NEVER execute interactive blocking commands (e.g. `vim`, `nano`, `vi`, `top`, `htop`, `less` without `-F`, `more`, or interactive `python` REPL without `-c`). They will hang the subshell execution.</rule>\n")
	sb.WriteString("  <rule id=\"limit_output_volume\">When inspecting files or command outputs, ALWAYS limit volume (e.g. `head -n 50`, `tail -n 50`, `rg -m 20`, `jq '.[:5]'`). Do NOT cat multi-megabyte files into the context window.</rule>\n")
	sb.WriteString("  <rule id=\"path_consistency\">Always use valid host OS paths matching `<workspace root>` and avoid guessing external package manager states.</rule>\n")
	sb.WriteString("  <rule id=\"verify_modifications\">After modifying a file or executing a build/test script, verify the status code and observations before concluding the task.</rule>\n")
	sb.WriteString("</operational_constraints>\n\n")

	// 5. Execution Best Practices
	sb.WriteString("<execution_best_practices>\n")
	sb.WriteString("  <practice>Search Efficiency: ALWAYS prioritize `ripgrep` (`rg`) for rapid regex/text searches over iterating through files manually.</practice>\n")
	sb.WriteString("  <practice>JSON Extraction: Utilize `jq` for filtering and extracting structured data from API responses.</practice>\n")
	sb.WriteString("  <practice>Computation & Parsing: Prefer executing concise Python (`python3 -c \"...\"`) or Node.js scripts for mathematical calculations, regex transformations, or data conversions rather than guessing.</practice>\n")
	sb.WriteString("</execution_best_practices>\n")

	return sb.String()
}

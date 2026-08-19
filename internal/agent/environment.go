package agent

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var (
	envPromptCache     string
	envPromptCacheOnce sync.Once
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
			// Fallback check for google-chrome or chromium-browser
			if _, err := exec.LookPath("google-chrome"); err == nil {
				candidates[i].Installed = true
				candidates[i].Binary = "google-chrome"
			} else if _, err := exec.LookPath("chromium-browser"); err == nil {
				candidates[i].Installed = true
				candidates[i].Binary = "chromium-browser"
			}
		} else if candidates[i].Binary == "python3" {
			// Fallback check for python on Windows
			if _, err := exec.LookPath("python"); err == nil {
				candidates[i].Installed = true
				candidates[i].Binary = "python"
			}
		}
	}

	return candidates
}

// BuildHostEnvironmentPrompt generates a detailed, structured prompt block informing the Agent
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
	sb.WriteString("## Host Operating System & Execution Environment\n")

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

	fmt.Fprintf(&sb, "- **Host OS & Architecture**: %s (%s) • %s\n", strings.Title(osName), arch, runtimeType)

	// 2. Active Shell & Path Conventions
	if osName == "windows" {
		sb.WriteString("- **Shell & Syntax**: PowerShell / Command Prompt (Use Windows path separators `\\`, PowerShell cmdlets, and standard Windows CLI syntax)\n")
	} else {
		sb.WriteString("- **Shell & Syntax**: Bash / POSIX Shell `/bin/bash` (Use Linux POSIX forward-slash paths `/data/...`, bash pipelines `|`, redirection, and standard Linux CLI syntax)\n")
	}

	fmt.Fprintf(&sb, "- **Workspace Root**: `%s` (Primary directory for user files, coding projects, and persistent artifacts)\n", workspaceDir)
	sb.WriteString("- **Data Directory**: `/data` (Hosts `/data/workspace`, `/data/config`, `/data/logs`, `/data/skills`)\n\n")

	// 3. Probed System Capabilities & Batteries-Included CLI Tools
	caps := ProbeSystemCapabilities()
	var installedTools []string
	for _, cap := range caps {
		if cap.Installed {
			installedTools = append(installedTools, fmt.Sprintf("`%s` (%s)", cap.Binary, cap.Description))
		}
	}

	if len(installedTools) > 0 {
		sb.WriteString("### Pre-installed Batteries-Included CLI Utilities & Runtimes\n")
		for _, toolDesc := range installedTools {
			fmt.Fprintf(&sb, "- %s\n", toolDesc)
		}
		sb.WriteString("\n")
	}

	// 4. Execution Guidelines for Agent
	sb.WriteString("### Execution Strategy Guidelines for the Agent\n")
	sb.WriteString("1. **Instant Code & Text Searching**: When searching across codebases or documents, PREFER `ripgrep` (`rg`) over manually reading files one by one for maximum speed and efficiency.\n")
	sb.WriteString("2. **Structured JSON Parsing**: When handling or transforming large JSON API responses, utilize `jq` for filtering and querying.\n")
	sb.WriteString("3. **Complex Scripting & Math**: Leverage `python3` or `node` for mathematical calculations, complex data transformations, file format conversions, or scraping scripts.\n")
	sb.WriteString("4. **Path & Environment Integrity**: ALWAYS use paths consistent with the host OS (`" + workspaceDir + "/...`) and avoid making assumptions about non-existent external package managers.\n\n")

	return sb.String()
}

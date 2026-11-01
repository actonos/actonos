package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/sandbox"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

// -----------------------------------------------------------------------------
// 9. Exec Tool (Sandboxed Command Execution)
// -----------------------------------------------------------------------------

type ExecTool struct {
	dataDir       string
	agentsDir     string
	userWorkspace *workspacepkg.Store
}

func NewExecTool(dataDir string, agentsDir ...string) *ExecTool {
	d, agDir := parseDataAndAgentsDir(dataDir, agentsDir...)
	return &ExecTool{dataDir: d, agentsDir: agDir}
}

func (t *ExecTool) SetUserWorkspace(store *workspacepkg.Store) {
	t.userWorkspace = store
}

func (t *ExecTool) Name() string { return "native_exec" }
func (t *ExecTool) Description() string {
	return "Execute a shell or PowerShell command inside your private agent workspace directory (timeout: 60s, max memory: 512MB). User Workspace documents are available with original filenames via $ACTONOS_USER_WORKSPACE and the relative folder user-workspace/ (for example python3 can open $ACTONOS_USER_WORKSPACE/report.pdf). Files created or modified there are ingested back into the user's Workspace UI."
}
func (t *ExecTool) Category() string { return "native" }

func (t *ExecTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": { "type": "string", "description": "The shell command to execute" },
			"timeout_seconds": { "type": "integer", "description": "Execution timeout in seconds (default: 60)" }
		},
		"required": ["command"]
	}`)
}

func (t *ExecTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	inputJSON = NormalizeToolInput(inputJSON)
	var input struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil || input.Command == "" {
		return nil, errors.New("command is required")
	}

	timeout := 60 * time.Second
	if input.TimeoutSeconds > 0 && input.TimeoutSeconds <= 300 {
		timeout = time.Duration(input.TimeoutSeconds) * time.Second
	}

	sb := sandbox.AutoDetectSandbox()
	_, agentWorkspace, _, err := resolveTargetBaseDir(ctx, t.dataDir, t.agentsDir, "")
	if err != nil {
		return nil, err
	}

	env := map[string]string{}
	var bindMounts []sandbox.BindMount
	if t.userWorkspace != nil {
		namedRoot := t.userWorkspace.NamedRoot()
		if namedRoot != "" {
			_ = os.MkdirAll(namedRoot, 0750)
			_ = t.userWorkspace.EnsureAgentView(agentWorkspace)
			env["ACTONOS_USER_WORKSPACE"] = namedRoot
			if sb.Name() == "bubblewrap" {
				env["ACTONOS_USER_WORKSPACE"] = workspacepkg.SandboxUserWorkspace
				bindMounts = append(bindMounts, sandbox.BindMount{
					Source: namedRoot,
					Dest:   workspacepkg.SandboxUserWorkspace,
				})
			}
		}
	}

	result, err := sb.Execute(ctx, sandbox.CommandRequest{
		Command:      input.Command,
		WorkspaceDir: agentWorkspace,
		Env:          env,
		BindMounts:   bindMounts,
		Timeout:      timeout,
		MaxMemoryMB:  512,
		MaxProcesses: 30,
	})
	var reconcile workspacepkg.ReconcileReport
	if t.userWorkspace != nil {
		if rec, recErr := t.userWorkspace.ReconcileNamed(ctx, AgentIDFromContext(ctx)); recErr != nil {
			slog.Warn("reconciling named workspace after exec", "error", recErr)
		} else {
			reconcile = rec
		}
	}

	if err != nil {
		return &ToolResult{
			Content: fmt.Sprintf("(Command execution failed: %v)", err),
			Data: map[string]any{
				"error":              err.Error(),
				"workspace_file_ids": append(append([]string{}, reconcile.Updated...), reconcile.Created...),
			},
		}, nil
	}

	var outputParts []string
	if strings.TrimSpace(result.Stdout) != "" {
		outputParts = append(outputParts, strings.TrimSpace(result.Stdout))
	}
	if strings.TrimSpace(result.Stderr) != "" {
		outputParts = append(outputParts, "[STDERR]\n"+strings.TrimSpace(result.Stderr))
	}
	if result.ExitCode != 0 {
		outputParts = append(outputParts, fmt.Sprintf("(Command exited with code %d)", result.ExitCode))
	}

	output := strings.Join(outputParts, "\n\n")
	if output == "" {
		output = fmt.Sprintf("(Command completed with exit code %d, no output)", result.ExitCode)
	} else if len(output) > 32768 {
		truncatedMsg := fmt.Sprintf("\n\n[Output truncated: output exceeded 32KB (%d bytes total). Use head/tail/grep/jq to inspect specific sections]", len(output))
		output = output[:32768] + truncatedMsg
	}

	workspaceIDs := append(append([]string{}, reconcile.Updated...), reconcile.Created...)
	return &ToolResult{
		Content: output,
		Data: map[string]any{
			"exit_code":          result.ExitCode,
			"stderr":             result.Stderr,
			"execution_time":     result.ExecutionTime.String(),
			"killed":             result.Killed,
			"workspace_file_ids": workspaceIDs,
			"workspace_updated":  reconcile.Updated,
			"workspace_created":  reconcile.Created,
		},
	}, nil
}

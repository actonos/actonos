package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// SubshellSandbox executes commands in a restricted subshell with workspace directory boundaries.
type SubshellSandbox struct{}

// NewSubshellSandbox creates a new SubshellSandbox.
func NewSubshellSandbox() *SubshellSandbox {
	return &SubshellSandbox{}
}

func (s *SubshellSandbox) Name() string {
	return "subshell"
}

func (s *SubshellSandbox) Execute(ctx context.Context, req CommandRequest) (*CommandResult, error) {
	if req.Timeout <= 0 {
		req.Timeout = 60 * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	workspace, err := filepath.Abs(req.WorkspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving workspace path: %w", err)
	}

	_ = os.MkdirAll(workspace, 0755)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "powershell", "-NoProfile", "-Command", req.Command)
	} else {
		cmd = exec.CommandContext(execCtx, "sh", "-c", req.Command)
	}

	cmd.Dir = workspace

	cmd.Env = append(os.Environ(),
		"CI=true",
		"DEBIAN_FRONTEND=noninteractive",
		"PAGER=cat",
		"TERM=dumb",
	)
	if len(req.Env) > 0 {
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	result := &CommandResult{
		Stdout:        stdout.String(),
		Stderr:        stderr.String(),
		ExecutionTime: duration,
	}

	if err != nil {
		result.Killed = execCtx.Err() == context.DeadlineExceeded || execCtx.Err() == context.Canceled
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result, nil
}

// isContainerEnvironment checks if ActonOS is running inside a Docker/OCI container or forced docker mode.
func isContainerEnvironment() bool {
	if os.Getenv("RUNTIME_MODE") == "docker" {
		return true
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

// AutoDetectSandbox selects Bubblewrap on Linux if available, or Subshell runner for container / dev.
func AutoDetectSandbox() Sandbox {
	if isContainerEnvironment() || os.Getenv("ACTONOS_ALLOW_INSECURE_EXEC") == "1" {
		return NewSubshellSandbox()
	}
	if strongSandboxAvailable() {
		return newStrongSandbox()
	}
	return &unavailableSandbox{}
}


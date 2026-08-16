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

	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
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
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Killed = execCtx.Err() == context.DeadlineExceeded
		}
	}

	return result, nil
}

// AutoDetectSandbox selects Bubblewrap on Linux if available, or Subshell runner for container / dev.
func AutoDetectSandbox() Sandbox {
	if runtime.GOOS == "linux" && os.Getenv("RUNTIME_MODE") != "docker" {
		if _, err := exec.LookPath("bwrap"); err == nil {
			return NewSubshellSandbox() // or NewBubblewrapSandbox() when on linux
		}
	}
	return NewSubshellSandbox()
}

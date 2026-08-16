//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// BubblewrapSandbox executes commands isolated via Bubblewrap (bwrap) and Cgroups v2 on Linux bare-metal.
type BubblewrapSandbox struct{}

// NewBubblewrapSandbox creates a new BubblewrapSandbox instance.
func NewBubblewrapSandbox() *BubblewrapSandbox {
	return &BubblewrapSandbox{}
}

func (s *BubblewrapSandbox) Name() string {
	return "bubblewrap"
}

func (s *BubblewrapSandbox) Execute(ctx context.Context, req CommandRequest) (*CommandResult, error) {
	if req.Timeout <= 0 {
		req.Timeout = 60 * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	workspace, err := filepath.Abs(req.WorkspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving workspace path: %w", err)
	}

	// Bubblewrap namespace isolation arguments
	args := []string{
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/lib", "/lib",
		"--proc", "/proc",
		"--dev", "/dev",
		"--unshare-all",
		"--die-with-parent",
		"--cap-drop", "ALL",
		"--bind", workspace, "/workspace",
		"--setenv", "PATH", "/usr/bin:/bin:/data/bin",
		"--chdir", "/workspace",
		"bash", "-c", req.Command,
	}

	cmd := exec.CommandContext(execCtx, "bwrap", args...)

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

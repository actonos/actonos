//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		"--new-session",
		"--die-with-parent",
		"--cap-drop", "ALL",
		"--bind", workspace, "/workspace",
		"--setenv", "PATH", "/usr/bin:/bin:/data/bin",
	}
	for _, mount := range req.BindMounts {
		source, err := filepath.Abs(mount.Source)
		if err != nil {
			return nil, fmt.Errorf("resolving bind mount source: %w", err)
		}
		flag := "--bind"
		if mount.ReadOnly {
			flag = "--ro-bind"
		}
		args = append(args, flag, source, mount.Dest)
	}
	for key, value := range req.Env {
		args = append(args, "--setenv", key, value)
	}
	args = append(args, "--chdir", "/workspace", "bash", "-c", req.Command)

	cmd := exec.CommandContext(execCtx, "bwrap", args...)
	if len(req.Env) > 0 {
		for key, value := range req.Env {
			cmd.Env = append(cmd.Environ(), key+"="+value)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting bubblewrap: %w", err)
	}
	cgroupPath, err := attachCgroup(cmd.Process.Pid, req)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}
	defer cleanupCgroup(cgroupPath)
	err = cmd.Wait()
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

func attachCgroup(pid int, req CommandRequest) (string, error) {
	if req.MaxMemoryMB <= 0 {
		req.MaxMemoryMB = 512
	}
	if req.MaxProcesses <= 0 {
		req.MaxProcesses = 30
	}
	base := "/sys/fs/cgroup"
	if _, err := os.Stat(filepath.Join(base, "cgroup.controllers")); err != nil {
		return "", fmt.Errorf("cgroup v2 is required for bare-metal sandboxing: %w", err)
	}
	group := filepath.Join(base, fmt.Sprintf("actonos-%d", pid))
	if err := os.Mkdir(group, 0755); err != nil {
		return "", fmt.Errorf("creating execution cgroup: %w", err)
	}
	writes := map[string]string{
		"memory.max":   strconv.FormatInt(int64(req.MaxMemoryMB)*1024*1024, 10),
		"pids.max":     strconv.Itoa(req.MaxProcesses),
		"cpu.max":      "50000 100000",
		"cgroup.procs": strconv.Itoa(pid),
	}
	for name, value := range writes {
		if err := os.WriteFile(filepath.Join(group, name), []byte(value), 0644); err != nil {
			cleanupCgroup(group)
			return "", fmt.Errorf("configuring cgroup %s: %w", name, err)
		}
	}
	return group, nil
}

func cleanupCgroup(group string) {
	if group == "" || filepath.Dir(group) != "/sys/fs/cgroup" || filepath.Base(group)[:8] != "actonos-" {
		return
	}
	_ = os.Remove(group)
}

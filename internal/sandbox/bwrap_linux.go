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
	"strings"
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
	if cgroupPath != "" {
		defer cleanupCgroup(cgroupPath)
	}
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

// findCgroupV2Base locates a writable cgroup v2 base directory.
func findCgroupV2Base() (string, error) {
	// 1. Explicit override via ACTONOS_CGROUP_BASE
	if custom := os.Getenv("ACTONOS_CGROUP_BASE"); custom != "" {
		if stat, err := os.Stat(custom); err == nil && stat.IsDir() {
			return custom, nil
		}
	}

	// 2. Discover current process cgroup from /proc/self/cgroup (e.g. systemd slice or user slice)
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) == 3 && (parts[0] == "0" || parts[1] == "") {
				rel := strings.TrimSpace(parts[2])
				if rel != "" && rel != "/" {
					candidate := filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(rel, "/"))
					if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
						testFile := filepath.Join(candidate, fmt.Sprintf(".acton_probe_%d", os.Getpid()))
						if err := os.WriteFile(testFile, []byte("1"), 0644); err == nil {
							_ = os.Remove(testFile)
							return candidate, nil
						}
					}
				}
			}
		}
	}

	// 3. Fallback to root /sys/fs/cgroup
	root := "/sys/fs/cgroup"
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err != nil {
		return "", fmt.Errorf("cgroup v2 is not mounted or cgroup.controllers missing at %s: %w", root, err)
	}

	return root, nil
}

func attachCgroup(pid int, req CommandRequest) (string, error) {
	if req.MaxMemoryMB <= 0 {
		req.MaxMemoryMB = 512
	}
	if req.MaxProcesses <= 0 {
		req.MaxProcesses = 30
	}

	base, err := findCgroupV2Base()
	if err != nil {
		if os.Getenv("ACTONOS_CGROUP_OPTIONAL") == "1" || os.Getenv("ACTONOS_ALLOW_INSECURE_EXEC") == "1" {
			return "", nil
		}
		return "", fmt.Errorf("cgroup v2 is required for bare-metal sandboxing: %w", err)
	}

	group := filepath.Join(base, fmt.Sprintf("actonos-%d", pid))
	if err := os.Mkdir(group, 0755); err != nil {
		if os.Getenv("ACTONOS_CGROUP_OPTIONAL") == "1" || os.Getenv("ACTONOS_ALLOW_INSECURE_EXEC") == "1" {
			return "", nil
		}
		return "", fmt.Errorf("creating execution cgroup in %s: %w (ensure /sys/fs/cgroup is writable or set Delegate=yes in systemd)", base, err)
	}

	writes := map[string]string{
		"memory.max":   strconv.FormatInt(int64(req.MaxMemoryMB)*1024*1024, 10),
		"pids.max":     strconv.Itoa(req.MaxProcesses),
		"cpu.max":      "50000 100000",
		"cgroup.procs": strconv.Itoa(pid),
	}
	for name, value := range writes {
		if err := os.WriteFile(filepath.Join(group, name), []byte(value), 0644); err != nil {
			if name == "cgroup.procs" {
				cleanupCgroup(group)
				if os.Getenv("ACTONOS_CGROUP_OPTIONAL") == "1" || os.Getenv("ACTONOS_ALLOW_INSECURE_EXEC") == "1" {
					return "", nil
				}
				return "", fmt.Errorf("attaching process to cgroup %s: %w", name, err)
			}
			// Non-critical limit write errors can be tolerated if parent subtree doesn't expose controller
		}
	}
	return group, nil
}

func cleanupCgroup(group string) {
	if group == "" || !strings.HasPrefix(filepath.Base(group), "actonos-") {
		return
	}
	_ = os.Remove(group)
}


//go:build !windows

package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type pipePTY struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (p *pipePTY) Read(buf []byte) (int, error) {
	return p.stdout.Read(buf)
}

func (p *pipePTY) Write(buf []byte) (int, error) {
	return p.stdin.Write(buf)
}

func (p *pipePTY) Close() error {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
	return nil
}

func (p *pipePTY) Resize(cols, rows int) error {
	return nil
}

func (p *pipePTY) Pid() int {
	if p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// startPTY starts a shell process on non-Windows systems.
func startPTY(shellType, workDir string, cols, rows int) (TerminalPTY, error) {
	shellPath := "/bin/sh"
	if strings.ToLower(strings.TrimSpace(shellType)) == "bash" {
		if path, err := exec.LookPath("bash"); err == nil {
			shellPath = path
		}
	} else {
		if path, err := exec.LookPath("bash"); err == nil {
			shellPath = path
		}
	}

	cmd := exec.Command(shellPath, "-l")
	if workDir == "" {
		workDir = "."
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("starting shell (%s): %w", shellPath, err)
	}

	return &pipePTY{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

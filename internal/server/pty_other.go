//go:build !windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
)

type posixPTY struct {
	cmd  *exec.Cmd
	ptmx *os.File
}

func (p *posixPTY) Read(buf []byte) (int, error) {
	return p.ptmx.Read(buf)
}

func (p *posixPTY) Write(buf []byte) (int, error) {
	return p.ptmx.Write(buf)
}

func (p *posixPTY) Close() error {
	_ = p.ptmx.Close()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
	return nil
}

func (p *posixPTY) Resize(cols, rows int) error {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}
	return pty.Setsize(p.ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func (p *posixPTY) Pid() int {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// startPTY starts a true interactive pseudo-terminal on POSIX systems (Linux, macOS).
func startPTY(shellType, workDir string, cols, rows int) (TerminalPTY, error) {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	shellPath := "/bin/bash"
	if path, err := exec.LookPath("bash"); err == nil {
		shellPath = path
	} else if path, err := exec.LookPath("sh"); err == nil {
		shellPath = path
	}

	// Detect if a specific valid shell was requested
	switch strings.ToLower(strings.TrimSpace(shellType)) {
	case "sh":
		if path, err := exec.LookPath("sh"); err == nil {
			shellPath = path
		}
	case "zsh":
		if path, err := exec.LookPath("zsh"); err == nil {
			shellPath = path
		}
	case "bash":
		if path, err := exec.LookPath("bash"); err == nil {
			shellPath = path
		}
	}

	cmd := exec.Command(shellPath, "-l")
	if workDir == "" {
		workDir = "/data"
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			workDir = "."
		}
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	// Start true POSIX pseudo-terminal with specified dimensions
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		return nil, fmt.Errorf("starting posix pty (%s): %w", shellPath, err)
	}

	return &posixPTY{
		cmd:  cmd,
		ptmx: ptmx,
	}, nil
}

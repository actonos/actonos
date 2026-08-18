//go:build windows

package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/UserExistsError/conpty"
)

type windowsConPTY struct {
	cpty *conpty.ConPty
}

func (w *windowsConPTY) Read(p []byte) (int, error) {
	return w.cpty.Read(p)
}

func (w *windowsConPTY) Write(p []byte) (int, error) {
	return w.cpty.Write(p)
}

func (w *windowsConPTY) Close() error {
	return w.cpty.Close()
}

func (w *windowsConPTY) Resize(cols, rows int) error {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}
	return w.cpty.Resize(cols, rows)
}

func (w *windowsConPTY) Pid() int {
	return w.cpty.Pid()
}

// startPTY starts an interactive pseudo-terminal on Windows using ConPTY.
func startPTY(shellType, workDir string, cols, rows int) (TerminalPTY, error) {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	commandLine := "powershell.exe -NoLogo"
	switch strings.ToLower(strings.TrimSpace(shellType)) {
	case "cmd":
		commandLine = "cmd.exe"
	case "bash", "wsl":
		commandLine = "wsl.exe"
	case "powershell", "ps":
		commandLine = "powershell.exe -NoLogo"
	default:
		commandLine = "powershell.exe -NoLogo"
	}

	if workDir == "" {
		workDir = "."
	}

	env := append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	cpty, err := conpty.Start(
		commandLine,
		conpty.ConPtyDimensions(cols, rows),
		conpty.ConPtyWorkDir(workDir),
		conpty.ConPtyEnv(env),
	)
	if err != nil {
		return nil, fmt.Errorf("starting windows conpty (%s): %w", commandLine, err)
	}

	return &windowsConPTY{cpty: cpty}, nil
}

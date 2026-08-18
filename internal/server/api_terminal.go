package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/coder/websocket"
)

// handleTerminalWebSocket handles interactive web terminal sessions via WebSocket.
func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Warn("terminal: websocket upgrade failed", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 1. Select appropriate shell for current operating system
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile")
	} else {
		// Prefer bash if available, fallback to sh
		if _, err := exec.LookPath("bash"); err == nil {
			cmd = exec.CommandContext(ctx, "bash", "-l")
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-l")
		}
	}

	// 2. Set working directory and environment
	cmd.Dir = s.workspaceDir
	if cmd.Dir == "" {
		cmd.Dir = "."
	}
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
	)

	// 3. Connect standard I/O pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		slog.Warn("terminal: failed to create stdin pipe", "error", err)
		_ = conn.Close(websocket.StatusInternalError, "failed to create stdin pipe")
		return
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Warn("terminal: failed to create stdout pipe", "error", err)
		_ = conn.Close(websocket.StatusInternalError, "failed to create stdout pipe")
		return
	}

	// Merge stderr into stdout pipe
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		slog.Warn("terminal: failed to start shell process", "error", err)
		_ = conn.Close(websocket.StatusInternalError, "failed to start shell")
		return
	}

	slog.Info("terminal: interactive shell session started", "pid", cmd.Process.Pid, "dir", cmd.Dir)

	// 4. Goroutine: Forward shell stdout -> WebSocket client
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
				writeErr := conn.Write(writeCtx, websocket.MessageText, buf[:n])
				writeCancel()
				if writeErr != nil {
					break
				}
			}
			if err != nil {
				if err != io.EOF {
					slog.Debug("terminal: stdout read finished", "error", err)
				}
				break
			}
		}
		cancel()
	}()

	// 5. Goroutine: Forward WebSocket client input -> shell stdin
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		if msgType == websocket.MessageText || msgType == websocket.MessageBinary {
			if len(data) > 0 {
				_, writeErr := stdin.Write(data)
				if writeErr != nil {
					break
				}
			}
		}
	}

	// 6. Graceful cleanup
	_ = stdin.Close()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	slog.Info("terminal: interactive shell session terminated")
}

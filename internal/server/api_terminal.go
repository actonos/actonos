package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
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

	// 1. Parse query options (shell, cols, rows)
	requestedShell := r.URL.Query().Get("shell")
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	// 2. Start True Pseudo-Terminal
	ptySession, err := startPTY(requestedShell, s.workspaceDir, cols, rows)
	if err != nil {
		slog.Warn("terminal: failed to start pseudo-terminal", "error", err)
		_ = conn.Close(websocket.StatusInternalError, "failed to start terminal PTY: "+err.Error())
		return
	}
	defer ptySession.Close()

	slog.Info("terminal: PTY session active", "pid", ptySession.Pid(), "shell", requestedShell, "cols", cols, "rows", rows)

	// 3. Goroutine: Forward PTY output -> WebSocket client
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := ptySession.Read(buf)
			if n > 0 {
				writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
				writeErr := conn.Write(writeCtx, websocket.MessageText, buf[:n])
				writeCancel()
				if writeErr != nil {
					break
				}
			}
			if err != nil {
				if err != io.EOF {
					slog.Debug("terminal: PTY read closed", "error", err)
				}
				break
			}
		}
		cancel()
	}()

	// 4. Goroutine: Keepalive Ping
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// 5. Read incoming WebSocket messages -> PTY stdin / Resize
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		if msgType == websocket.MessageText || msgType == websocket.MessageBinary {
			if len(data) == 0 {
				continue
			}

			// Check for resize command
			if len(data) > 15 && data[0] == '{' {
				var resizeMsg ptyResizeMessage
				if err := json.Unmarshal(data, &resizeMsg); err == nil && resizeMsg.Type == "resize" {
					if resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
						_ = ptySession.Resize(resizeMsg.Cols, resizeMsg.Rows)
					}
					continue
				}
			}

			// Forward raw keystrokes directly into PTY
			_, writeErr := ptySession.Write(data)
			if writeErr != nil {
				break
			}
		}
	}

	slog.Info("terminal: PTY session ended", "pid", ptySession.Pid())
}

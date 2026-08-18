package server

import (
	"io"
)

// TerminalPTY represents an interactive pseudo-terminal session.
type TerminalPTY interface {
	io.Reader
	io.Writer
	io.Closer
	Resize(cols, rows int) error
	Pid() int
}

// ptyResizeMessage represents a terminal dimension update from the frontend.
type ptyResizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

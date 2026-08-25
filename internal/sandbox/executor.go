package sandbox

import (
	"context"
	"errors"
	"time"
)

// ErrSandboxUnavailable is returned when strong isolation is required but unavailable.
var ErrSandboxUnavailable = errors.New("strong command sandbox is unavailable")

// BindMount maps a host directory into the sandbox filesystem.
type BindMount struct {
	Source   string
	Dest     string
	ReadOnly bool
}

// CommandRequest represents a shell command to be executed in the sandbox.
type CommandRequest struct {
	Command      string            `json:"command"`
	WorkspaceDir string            `json:"workspace_dir"`
	Env          map[string]string `json:"env,omitempty"`
	BindMounts   []BindMount       `json:"bind_mounts,omitempty"`
	Timeout      time.Duration     `json:"timeout,omitempty"`
	MaxMemoryMB  int               `json:"max_memory_mb,omitempty"` // Default 512MB
	MaxProcesses int               `json:"max_processes,omitempty"` // Default 30
}

// CommandResult represents the execution result.
type CommandResult struct {
	ExitCode      int           `json:"exit_code"`
	Stdout        string        `json:"stdout"`
	Stderr        string        `json:"stderr"`
	ExecutionTime time.Duration `json:"execution_time"`
	Killed        bool          `json:"killed"`
}

// Sandbox is the abstraction for executing untrusted agent commands in an isolated environment.
type Sandbox interface {
	// Name returns the sandbox implementation name (e.g. "bubblewrap", "subshell").
	Name() string

	// Execute runs a command inside the isolated sandbox.
	Execute(ctx context.Context, req CommandRequest) (*CommandResult, error)
}

type unavailableSandbox struct{}

func (s *unavailableSandbox) Name() string { return "unavailable" }

func (s *unavailableSandbox) Execute(context.Context, CommandRequest) (*CommandResult, error) {
	return nil, ErrSandboxUnavailable
}

package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestSubshellSandbox_Execute(t *testing.T) {
	sb := NewSubshellSandbox()
	tempDir := t.TempDir()

	req := CommandRequest{
		Command:      "echo Hello ActonOS Sandbox",
		WorkspaceDir: tempDir,
		Timeout:      5 * time.Second,
	}

	res, err := sb.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("sandbox execute failed: %v", err)
	}

	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", res.ExitCode, res.Stderr)
	}

	if res.Stdout == "" {
		t.Fatal("expected stdout output")
	}
}

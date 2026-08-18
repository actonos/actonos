package sandbox

import (
	"context"
	"errors"
	"runtime"
	"strings"
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

func TestSubshellSandboxEnvironmentFailureAndTimeout(t *testing.T) {
	sb := NewSubshellSandbox()
	if sb.Name() != "subshell" {
		t.Fatalf("unexpected sandbox name: %s", sb.Name())
	}
	envCommand := "echo $ACTONOS_SANDBOX_TEST"
	failCommand := "exit 7"
	sleepCommand := "sleep 2"
	if isWindows() {
		envCommand = "Write-Output $env:ACTONOS_SANDBOX_TEST"
		failCommand = "exit 7"
		sleepCommand = "Start-Sleep -Seconds 2"
	}
	result, err := sb.Execute(context.Background(), CommandRequest{
		Command: envCommand, WorkspaceDir: t.TempDir(),
		Env: map[string]string{"ACTONOS_SANDBOX_TEST": "isolated"},
	})
	if err != nil || !strings.Contains(result.Stdout, "isolated") {
		t.Fatalf("environment execution failed: result=%+v err=%v", result, err)
	}
	result, err = sb.Execute(context.Background(), CommandRequest{
		Command: failCommand, WorkspaceDir: t.TempDir(), Timeout: time.Second,
	})
	if err != nil || result.ExitCode != 7 {
		t.Fatalf("non-zero exit was not captured: result=%+v err=%v", result, err)
	}
	result, err = sb.Execute(context.Background(), CommandRequest{
		Command: sleepCommand, WorkspaceDir: t.TempDir(), Timeout: 50 * time.Millisecond,
	})
	if err != nil || !result.Killed {
		t.Fatalf("timeout was not classified: result=%+v err=%v", result, err)
	}
}

func TestAutoDetectSandboxFailsClosedWithoutOverride(t *testing.T) {
	t.Setenv("RUNTIME_MODE", "")
	t.Setenv("ACTONOS_ALLOW_INSECURE_EXEC", "")
	sb := AutoDetectSandbox()
	if sb.Name() == "subshell" {
		t.Fatal("auto-detected sandbox must not silently use host subshell")
	}
	if sb.Name() == "unavailable" {
		if _, err := sb.Execute(context.Background(), CommandRequest{}); !errors.Is(err, ErrSandboxUnavailable) {
			t.Fatalf("expected unavailable error, got %v", err)
		}
	}
}

func TestAutoDetectSandboxAllowsExplicitDevelopmentOverride(t *testing.T) {
	t.Setenv("RUNTIME_MODE", "")
	t.Setenv("ACTONOS_ALLOW_INSECURE_EXEC", "1")
	sb := AutoDetectSandbox()
	if sb.Name() != "subshell" {
		t.Fatalf("expected explicit development override to select subshell, got %s", sb.Name())
	}
}

func isWindows() bool {
	return strings.Contains(strings.ToLower(runtimeGOOS()), "windows")
}

var runtimeGOOS = func() string { return runtime.GOOS }

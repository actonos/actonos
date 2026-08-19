package agent

import (
	"runtime"
	"strings"
	"testing"
)

func TestProbeSystemCapabilities(t *testing.T) {
	caps := ProbeSystemCapabilities()
	if len(caps) == 0 {
		t.Fatal("expected system capabilities list to not be empty")
	}

	foundKnown := false
	for _, c := range caps {
		if c.Binary == "rg" || c.Binary == "python3" || c.Binary == "python" || c.Binary == "git" || c.Binary == "curl" {
			foundKnown = true
			break
		}
	}

	if !foundKnown {
		t.Log("Note: none of the common binaries were found, which is possible in isolated CI environments")
	}
}

func TestBuildHostEnvironmentPrompt(t *testing.T) {
	prompt := BuildHostEnvironmentPrompt("/custom/workspace")
	if prompt == "" {
		t.Fatal("expected host environment prompt to not be empty")
	}

	if !strings.Contains(prompt, "Host Operating System & Execution Environment") {
		t.Errorf("prompt missing Host Operating System header, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "/custom/workspace") {
		t.Errorf("prompt missing workspace directory /custom/workspace, got:\n%s", prompt)
	}

	if runtime.GOOS == "windows" {
		if !strings.Contains(prompt, "PowerShell") {
			t.Errorf("expected Windows prompt to reference PowerShell")
		}
	} else {
		if !strings.Contains(prompt, "Bash") && !strings.Contains(prompt, "POSIX") {
			t.Errorf("expected Linux/POSIX prompt to reference Bash/POSIX")
		}
	}
}

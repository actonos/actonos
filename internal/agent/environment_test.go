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

	if !strings.Contains(prompt, "<environment>") {
		t.Errorf("prompt missing <environment> XML tag, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "<workspace_storage_policy>") {
		t.Errorf("prompt missing <workspace_storage_policy> XML tag, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "user_workspace_mandate") || !strings.Contains(prompt, "native_workspace_write") {
		t.Errorf("prompt missing user_workspace_mandate rule, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "agent_internal_scratchpad") {
		t.Errorf("prompt missing agent_internal_scratchpad rule, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "<filesystem_layout>") {
		t.Errorf("prompt missing <filesystem_layout> tag, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<system_data_root") {
		t.Errorf("prompt missing <system_data_root> tag, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<user_workspace") {
		t.Errorf("prompt missing <user_workspace> tag, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "ACTONOS_USER_WORKSPACE") || !strings.Contains(prompt, "user-workspace") {
		t.Errorf("prompt missing named workspace exec paths, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "user_workspace_exec_paths") {
		t.Errorf("prompt missing user_workspace_exec_paths rule, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<agent_private_scratchpad") {
		t.Errorf("prompt missing <agent_private_scratchpad> tag, got:\n%s", prompt)
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

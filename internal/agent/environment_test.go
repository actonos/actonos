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

	if !strings.Contains(prompt, "dedicated_workspace_default") {
		t.Errorf("prompt missing dedicated_workspace_default rule, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "user_workspace_database") || !strings.Contains(prompt, "native_workspace_search") {
		t.Errorf("prompt missing database workspace tool policy, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "<execution_best_practices>") {
		t.Errorf("prompt missing <execution_best_practices> XML tag, got:\n%s", prompt)
	}

	customAgentPrompt := BuildAgentEnvironmentPrompt("/custom", "/custom/workspace", "agent_custom_coder")
	if !strings.Contains(customAgentPrompt, "agent_custom_coder") || !strings.Contains(customAgentPrompt, "agents") {
		t.Errorf("expected custom agent slug in agent_dir prompt, got:\n%s", customAgentPrompt)
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

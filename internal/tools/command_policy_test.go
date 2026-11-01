package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func commandInput(t *testing.T, command string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCommandPolicyRejectsDestructiveVariants(t *testing.T) {
	blocked := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -rf / --no-preserve-root",
		"rm -rf --no-preserve-root /",
		"rm -fr /",
		"rm -r -f /",
		"rm -Rf /",
		"rm --recursive --force /",
		"rm   -rf    /",
		"RM -RF /",
		"sudo rm -rf /",
		"echo start; rm -rf /",
		"true && rm -rf /*",
		"(rm -rf /)",
		"rm -rf ~",
		"rm -rf ~/",
		"rm -rf $HOME",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
		"echo x > /dev/sda",
		"echo x > /dev/nvme0n1",
		":(){ :|:& };:",
		"chmod -R 777 /",
		"chown -R root /",
		"Remove-Item -Recurse C:\\",
	}
	for _, command := range blocked {
		if err := validateCommandToolInput(commandInput(t, command)); err == nil {
			t.Errorf("expected %q to be rejected", command)
		}
	}
}

func TestCommandPolicyAllowsScopedDeletion(t *testing.T) {
	allowed := []string{
		"rm -rf /workspace/build",
		"rm -rf /tmp/scratch",
		"rm -rf ./node_modules",
		"rm -rf build dist",
		"rm -r old_dir",
		"rm file.txt",
		"rm -f /workspace/out.log",
		"ls /",
		"cat /etc/hostname",
		"echo 'rm is dangerous'",
		"grep -r format ./docs",
		"go test ./...",
	}
	for _, command := range allowed {
		if err := validateCommandToolInput(commandInput(t, command)); err != nil {
			t.Errorf("expected %q to be allowed, got %v", command, err)
		}
	}
}

func TestCommandPolicyRejectsMalformedInput(t *testing.T) {
	for name, input := range map[string]json.RawMessage{
		"not json":      json.RawMessage(`{`),
		"missing field": json.RawMessage(`{}`),
		"empty":         commandInput(t, ""),
		"whitespace":    commandInput(t, "  \t\n "),
		"wrong type":    json.RawMessage(`{"command": 42}`),
	} {
		if err := validateCommandToolInput(input); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCommandPolicyErrorNamesTheRule(t *testing.T) {
	err := validateCommandToolInput(commandInput(t, "mkfs.ext4 /dev/sda1"))
	if err == nil || !strings.Contains(err.Error(), "security policy") {
		t.Fatalf("expected a policy violation error, got %v", err)
	}
}

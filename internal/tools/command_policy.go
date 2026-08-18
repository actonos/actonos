package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

var forbiddenCommandPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	":(){ :|:& };:",
	"mkfs",
	"dd if=/dev/zero",
	"> /dev/sda",
	"> /dev/nvme",
	"chmod -r 777 /",
	"chown -r root /",
	"format c:",
	"clear-disk",
	"remove-item -recurse c:\\",
	"remove-item -recurse d:\\",
}

func validateCommandToolInput(input json.RawMessage) error {
	var request struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &request); err != nil {
		return fmt.Errorf("decoding command arguments: %w", err)
	}
	command := strings.ToLower(strings.TrimSpace(request.Command))
	if command == "" {
		return fmt.Errorf("command is required")
	}
	for _, pattern := range forbiddenCommandPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("command violates security policy: matched %q", pattern)
		}
	}
	return nil
}

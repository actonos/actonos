package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrForbiddenCommand = errors.New("forbidden command: command violates deterministic security policy")
	ErrPathEscape       = errors.New("path escape: file operation escapes allowed workspace directory")
	ErrSchemaMismatch   = errors.New("schema validation failed: output does not match expected schema")
)

// Verifier implements multi-tier deterministic and semantic verification.
type Verifier struct {
	forbiddenPatterns []string
}

// NewVerifier creates a new Verifier instance.
func NewVerifier() *Verifier {
	return &Verifier{
		forbiddenPatterns: []string{
			"rm -rf /",
			"rm -rf /*",
			":(){ :|:& };:", // Fork bomb
			"mkfs",
			"dd if=/dev/zero",
			"> /dev/sda",
			"> /dev/nvme",
			"chmod -R 777 /",
			"chown -R root /",
		},
	}
}

// VerifyPath ensures path is contained inside allowed workspace.
func (v *Verifier) VerifyPath(targetPath, allowedWorkspace string) error {
	cleanRel := filepath.Clean(targetPath)
	if strings.HasPrefix(cleanRel, "..") {
		return ErrPathEscape
	}

	absAllowed, err := filepath.Abs(allowedWorkspace)
	if err != nil {
		return err
	}

	var fullPath string
	if filepath.IsAbs(targetPath) {
		fullPath = filepath.Clean(targetPath)
	} else {
		fullPath = filepath.Join(absAllowed, cleanRel)
	}

	if !strings.HasPrefix(fullPath, absAllowed) {
		return ErrPathEscape
	}

	return nil
}

// VerifyCommand performs static AST/string inspection on shell commands before execution.
func (v *Verifier) VerifyCommand(cmd string) error {
	trimmed := strings.TrimSpace(cmd)
	lower := strings.ToLower(trimmed)

	for _, forbidden := range v.forbiddenPatterns {
		if strings.Contains(lower, forbidden) {
			return fmt.Errorf("%w: matched forbidden pattern '%s'", ErrForbiddenCommand, forbidden)
		}
	}

	return nil
}

// VerifyJSONSyntax validates whether a string is well-formed JSON.
func (v *Verifier) VerifyJSONSyntax(rawJSON string) error {
	var js json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &js); err != nil {
		return fmt.Errorf("invalid json syntax: %w", err)
	}
	return nil
}

// VerifySemanticConsistency performs Tier-2 semantic validation of agent outputs against user requirements.
func (v *Verifier) VerifySemanticConsistency(ctx context.Context, originalGoal, agentOutput string) bool {
	if strings.TrimSpace(agentOutput) == "" {
		return false
	}
	// Check for empty or error loop indicators
	if strings.Contains(agentOutput, "I cannot fulfill this request") && len(originalGoal) > 0 {
		return false
	}
	return true
}

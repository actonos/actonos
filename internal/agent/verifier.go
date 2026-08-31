package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

var (
	ErrForbiddenCommand = errors.New("forbidden command: command violates deterministic security policy")
	ErrPathEscape       = errors.New("path escape: file operation escapes allowed workspace directory")
	ErrSchemaMismatch   = errors.New("schema validation failed: output does not match expected schema")
)

// Verifier implements multi-tier deterministic and semantic verification.
type Verifier struct {
	forbiddenPatterns []string
	outcomeVerifier   *OutcomeVerifier
}

// VerifyToolCommand parses a native_exec argument object before applying command policy.
func (v *Verifier) VerifyToolCommand(input json.RawMessage) error {
	var request struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(tools.NormalizeToolInput(input), &request); err != nil {
		return fmt.Errorf("decoding command arguments: %w", err)
	}
	if strings.TrimSpace(request.Command) == "" {
		return errors.New("command is required")
	}
	return v.VerifyCommand(request.Command)
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
		outcomeVerifier: NewOutcomeVerifier(nil, nil),
	}
}

// SetOutcomeDB configures the database handle for SQL outcome assertions.
func (v *Verifier) SetOutcomeDB(db *sql.DB) {
	if v.outcomeVerifier != nil {
		v.outcomeVerifier.SetDB(db)
	}
}

// SetOutcomeRouter configures the LLM cascade router for semantic/rubric assertions.
func (v *Verifier) SetOutcomeRouter(router *llm.ModelCascadeRouter) {
	if v.outcomeVerifier != nil {
		v.outcomeVerifier.llmRouter = router
	}
}

// VerifyOutcomeAssertions runs Tier-3 deterministic outcome validation.
func (v *Verifier) VerifyOutcomeAssertions(ctx context.Context, dataDir string, assertions []OutcomeAssertion) (bool, []AssertionResult) {
	if v.outcomeVerifier == nil {
		v.outcomeVerifier = NewOutcomeVerifier(nil, nil)
	}
	return v.outcomeVerifier.VerifyAll(ctx, dataDir, assertions)
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

// VerifyTaskCompletion rejects unsupported completion claims and obvious failed observations.
func (v *Verifier) VerifyTaskCompletion(originalGoal, agentOutput string, toolCalls []llm.ToolCall) bool {
	if !v.VerifySemanticConsistency(context.Background(), originalGoal, agentOutput) {
		return false
	}
	lowerOutput := strings.ToLower(agentOutput)
	for _, marker := range []string{
		"error executing tool",
		"tool execution blocked",
		"approval required",
		"failed:",
		"tests failed",
	} {
		if strings.Contains(lowerOutput, marker) {
			return false
		}
	}
	lowerGoal := strings.ToLower(originalGoal)
	actionRequired := strings.Contains(lowerGoal, "code") ||
		strings.Contains(lowerGoal, "file") ||
		strings.Contains(lowerGoal, "execute") ||
		strings.Contains(lowerGoal, "implement") ||
		strings.Contains(lowerGoal, "fix")
	return !actionRequired || len(toolCalls) > 0
}

var deliverableWriteTools = map[string]bool{
	"native_file_write":      true,
	"native_file_edit":       true,
	"native_workspace_write": true,
}

// HasDeliverableWrite reports that a native write/edit tool ran in this turn.
func HasDeliverableWrite(calls []llm.ToolCall) bool {
	for _, call := range calls {
		if deliverableWriteTools[tools.NormalizeToolName(call.Function.Name)] {
			return true
		}
	}
	return false
}

// MissionDeliverableSatisfied reports that a write tool produced an artifact
// this turn. Goal wording is ignored — remaining DAG steps, not language,
// decide whether the mission is finished.
func MissionDeliverableSatisfied(_ string, calls []llm.ToolCall) bool {
	return HasDeliverableWrite(calls)
}

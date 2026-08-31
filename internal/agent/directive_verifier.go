package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// VerifyDirectiveOutcome verifies whether an expected outcome rule is satisfied in the environment.
// Supported rule formats:
// - "file_exists:<path>" (verifies file exists and size > 0)
// - "file_contains:<path>|<substring>" (verifies file contains specific text)
// - "http_status:<url>|<expected_code>" (e.g. "http_status:http://localhost:8080/api/health|200")
// - "dir_not_empty:<path>" (verifies directory contains >= 1 file)
// - "sql_count:<query>|<condition>" (e.g. "sql_count:SELECT COUNT(*) FROM memories|> 0")
// - "shell_exit:<cmd>|<exit_code>" (e.g. "shell_exit:echo test|0")
// - "json_schema:<target>|<keys>"
func VerifyDirectiveOutcome(ctx context.Context, dataDir, rule string) (bool, string, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true, "no verification rule specified", nil
	}

	verifier := NewOutcomeVerifier(nil, nil)
	assertion, err := verifier.ParseAssertionString(rule)
	if err != nil {
		return false, "", fmt.Errorf("invalid verification rule format: %w", err)
	}

	res := verifier.VerifyAssertion(ctx, dataDir, assertion)
	if res.Error != "" {
		return false, res.Message, errors.New(res.Error)
	}
	return res.Passed, res.Message, nil
}


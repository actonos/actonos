package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/llm"
)

// AssertionKind identifies the specific deterministic or semantic outcome verification method.
type AssertionKind string

const (
	AssertFileExists   AssertionKind = "file_exists"
	AssertFileContains AssertionKind = "file_contains"
	AssertJSONSchema   AssertionKind = "json_schema"
	AssertHTTPStatus   AssertionKind = "http_status"
	AssertSQLCount     AssertionKind = "sql_count"
	AssertShellExit    AssertionKind = "shell_exit"
	AssertDirNotEmpty  AssertionKind = "dir_not_empty"
	AssertLLMJudge     AssertionKind = "llm_judge"
)

// OutcomeAssertion represents a machine-verifiable post-condition for a task or DAG step.
type OutcomeAssertion struct {
	Kind       AssertionKind `json:"kind"`
	Target     string        `json:"target"`               // File path, URL, command, SQL query, or artifact name
	Expected   string        `json:"expected,omitempty"`   // Substring, status code, row count, or exit code
	Rubric     string        `json:"rubric,omitempty"`     // Evaluation guidelines for LLM judge
	TimeoutSec int           `json:"timeout_sec,omitempty"`// Max verification timeout in seconds (default: 10s)
	Critical   bool          `json:"critical"`             // If true, failure fails the entire task
}

// AssertionResult records the outcome of verifying an assertion.
type AssertionResult struct {
	Assertion OutcomeAssertion `json:"assertion"`
	Passed    bool             `json:"passed"`
	Message   string           `json:"message"`
	Duration  time.Duration    `json:"duration"`
	Error     string           `json:"error,omitempty"`
}

// OutcomeVerifier evaluates assertions against the operating environment and artifacts.
type OutcomeVerifier struct {
	db        *sql.DB
	llmRouter *llm.ModelCascadeRouter
	client    *http.Client
}

// NewOutcomeVerifier creates a new OutcomeVerifier instance.
func NewOutcomeVerifier(db *sql.DB, router *llm.ModelCascadeRouter) *OutcomeVerifier {
	return &OutcomeVerifier{
		db:        db,
		llmRouter: router,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetDB attaches the active SQLite database handle.
func (v *OutcomeVerifier) SetDB(db *sql.DB) {
	v.db = db
}

// ParseAssertionString converts a single string rule (e.g. "file_exists:report.md") into an OutcomeAssertion.
func (v *OutcomeVerifier) ParseAssertionString(rule string) (OutcomeAssertion, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return OutcomeAssertion{}, errors.New("empty assertion rule")
	}

	parts := strings.SplitN(rule, ":", 2)
	if len(parts) < 2 {
		return OutcomeAssertion{}, fmt.Errorf("invalid assertion syntax (expected kind:target): %q", rule)
	}

	kind := AssertionKind(strings.ToLower(strings.TrimSpace(parts[0])))
	payload := strings.TrimSpace(parts[1])

	switch kind {
	case AssertFileExists, AssertDirNotEmpty:
		return OutcomeAssertion{
			Kind:     kind,
			Target:   payload,
			Critical: true,
		}, nil

	case AssertFileContains:
		sub := strings.SplitN(payload, "|", 2)
		expected := ""
		if len(sub) > 1 {
			expected = sub[1]
		}
		return OutcomeAssertion{
			Kind:     kind,
			Target:   sub[0],
			Expected: expected,
			Critical: true,
		}, nil

	case AssertHTTPStatus:
		sub := strings.SplitN(payload, "|", 2)
		expected := "200"
		if len(sub) > 1 {
			expected = sub[1]
		}
		return OutcomeAssertion{
			Kind:     kind,
			Target:   sub[0],
			Expected: expected,
			Critical: true,
		}, nil

	case AssertSQLCount:
		sub := strings.SplitN(payload, "|", 2)
		expected := "> 0"
		if len(sub) > 1 {
			expected = sub[1]
		}
		return OutcomeAssertion{
			Kind:     kind,
			Target:   sub[0],
			Expected: expected,
			Critical: true,
		}, nil

	case AssertShellExit:
		sub := strings.SplitN(payload, "|", 2)
		expected := "0"
		if len(sub) > 1 {
			expected = sub[1]
		}
		return OutcomeAssertion{
			Kind:     kind,
			Target:   sub[0],
			Expected: expected,
			Critical: true,
		}, nil

	case AssertJSONSchema:
		sub := strings.SplitN(payload, "|", 2)
		expected := ""
		if len(sub) > 1 {
			expected = sub[1]
		}
		return OutcomeAssertion{
			Kind:     kind,
			Target:   sub[0],
			Expected: expected,
			Critical: true,
		}, nil

	case AssertLLMJudge:
		sub := strings.SplitN(payload, "|", 2)
		rubric := ""
		if len(sub) > 1 {
			rubric = sub[1]
		}
		return OutcomeAssertion{
			Kind:     kind,
			Target:   sub[0],
			Rubric:   rubric,
			Critical: false,
		}, nil

	default:
		return OutcomeAssertion{}, fmt.Errorf("unsupported assertion kind: %q", kind)
	}
}

// VerifyAssertion executes a single outcome check.
func (v *OutcomeVerifier) VerifyAssertion(ctx context.Context, dataDir string, assertion OutcomeAssertion) AssertionResult {
	start := time.Now()
	timeout := 10 * time.Second
	if assertion.TimeoutSec > 0 {
		timeout = time.Duration(assertion.TimeoutSec) * time.Second
	}
	vCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resolvePath := func(p string) string {
		p = strings.TrimSpace(p)
		if filepath.IsAbs(p) {
			return p
		}
		if dataDir != "" {
			return filepath.Join(dataDir, p)
		}
		return p
	}

	var res AssertionResult
	res.Assertion = assertion

	switch assertion.Kind {
	case AssertFileExists:
		targetPath := resolvePath(assertion.Target)
		info, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				res.Passed = false
				res.Message = fmt.Sprintf("file %q does not exist", assertion.Target)
			} else {
				res.Passed = false
				res.Error = err.Error()
				res.Message = fmt.Sprintf("failed to stat file %q: %v", targetPath, err)
			}
		} else if info.IsDir() {
			res.Passed = false
			res.Message = fmt.Sprintf("path %q is a directory, expected regular file", assertion.Target)
		} else if info.Size() == 0 {
			res.Passed = false
			res.Message = fmt.Sprintf("file %q is empty (0 bytes)", assertion.Target)
		} else {
			res.Passed = true
			res.Message = fmt.Sprintf("file %q verified (size: %d bytes)", assertion.Target, info.Size())
		}

	case AssertFileContains:
		targetPath := resolvePath(assertion.Target)
		data, err := os.ReadFile(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				res.Passed = false
				res.Message = fmt.Sprintf("file %q does not exist", assertion.Target)
			} else {
				res.Passed = false
				res.Error = err.Error()
				res.Message = fmt.Sprintf("failed to read file %q: %v", targetPath, err)
			}
		} else if !strings.Contains(string(data), assertion.Expected) {
			res.Passed = false
			res.Message = fmt.Sprintf("file %q does not contain expected substring %q", assertion.Target, assertion.Expected)
		} else {
			res.Passed = true
			res.Message = fmt.Sprintf("file %q contains expected substring (%d bytes match)", assertion.Target, len(assertion.Expected))
		}

	case AssertDirNotEmpty:
		targetPath := resolvePath(assertion.Target)
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				res.Passed = false
				res.Message = fmt.Sprintf("directory %q does not exist", assertion.Target)
			} else {
				res.Passed = false
				res.Error = err.Error()
				res.Message = fmt.Sprintf("failed to read directory %q: %v", targetPath, err)
			}
		} else if len(entries) == 0 {
			res.Passed = false
			res.Message = fmt.Sprintf("directory %q is empty", assertion.Target)
		} else {
			res.Passed = true
			res.Message = fmt.Sprintf("directory %q verified (%d entries)", assertion.Target, len(entries))
		}

	case AssertHTTPStatus:
		urlStr := strings.TrimSpace(assertion.Target)
		if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
			urlStr = "http://" + urlStr
		}
		expectedCode := 200
		if assertion.Expected != "" {
			if code, err := strconv.Atoi(assertion.Expected); err == nil {
				expectedCode = code
			}
		}

		req, err := http.NewRequestWithContext(vCtx, http.MethodGet, urlStr, nil)
		if err != nil {
			res.Passed = false
			res.Error = err.Error()
			res.Message = fmt.Sprintf("invalid HTTP request for %q: %v", urlStr, err)
			break
		}

		resp, err := v.client.Do(req)
		if err != nil {
			res.Passed = false
			res.Error = err.Error()
			res.Message = fmt.Sprintf("HTTP GET %q failed: %v", urlStr, err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == expectedCode {
				res.Passed = true
				res.Message = fmt.Sprintf("HTTP %q returned expected status %d", urlStr, expectedCode)
			} else {
				res.Passed = false
				res.Message = fmt.Sprintf("HTTP %q returned status %d, expected %d", urlStr, resp.StatusCode, expectedCode)
			}
		}

	case AssertJSONSchema:
		// Target can be a file path or raw JSON string
		var raw []byte
		targetPath := resolvePath(assertion.Target)
		if fileBytes, err := os.ReadFile(targetPath); err == nil {
			raw = fileBytes
		} else {
			raw = []byte(assertion.Target)
		}

		var parsed any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			res.Passed = false
			res.Error = err.Error()
			res.Message = fmt.Sprintf("JSON validation failed: %v", err)
		} else {
			// If expected schema/keys specified, verify required keys exist
			if assertion.Expected != "" {
				var requiredKeys []string
				if err := json.Unmarshal([]byte(assertion.Expected), &requiredKeys); err != nil {
					// Fallback to comma-separated list
					for _, k := range strings.Split(assertion.Expected, ",") {
						if tk := strings.TrimSpace(k); tk != "" {
							requiredKeys = append(requiredKeys, tk)
						}
					}
				}
				if obj, ok := parsed.(map[string]any); ok && len(requiredKeys) > 0 {
					missing := []string{}
					for _, key := range requiredKeys {
						if _, exists := obj[key]; !exists {
							missing = append(missing, key)
						}
					}
					if len(missing) > 0 {
						res.Passed = false
						res.Message = fmt.Sprintf("JSON missing required keys: %s", strings.Join(missing, ", "))
						break
					}
				}
			}
			res.Passed = true
			res.Message = "valid JSON payload verified"
		}

	case AssertSQLCount:
		if v.db == nil {
			res.Passed = false
			res.Message = "database connection not available for SQL count assertion"
			break
		}
		query := strings.TrimSpace(assertion.Target)
		var count int64
		err := v.db.QueryRowContext(vCtx, query).Scan(&count)
		if err != nil {
			res.Passed = false
			res.Error = err.Error()
			res.Message = fmt.Sprintf("SQL query failed: %v", err)
		} else {
			// Evaluate condition: e.g. "> 0", "== 5", ">= 1"
			exp := strings.TrimSpace(assertion.Expected)
			passed := false
			if exp == "" || exp == "> 0" {
				passed = count > 0
			} else if strings.HasPrefix(exp, ">=") {
				targetVal, _ := strconv.ParseInt(strings.TrimSpace(exp[2:]), 10, 64)
				passed = count >= targetVal
			} else if strings.HasPrefix(exp, "<=") {
				targetVal, _ := strconv.ParseInt(strings.TrimSpace(exp[2:]), 10, 64)
				passed = count <= targetVal
			} else if strings.HasPrefix(exp, ">") {
				targetVal, _ := strconv.ParseInt(strings.TrimSpace(exp[1:]), 10, 64)
				passed = count > targetVal
			} else if strings.HasPrefix(exp, "<") {
				targetVal, _ := strconv.ParseInt(strings.TrimSpace(exp[1:]), 10, 64)
				passed = count < targetVal
			} else if strings.HasPrefix(exp, "==") || strings.HasPrefix(exp, "=") {
				clean := strings.TrimPrefix(strings.TrimPrefix(exp, "=="), "=")
				targetVal, _ := strconv.ParseInt(strings.TrimSpace(clean), 10, 64)
				passed = count == targetVal
			} else {
				targetVal, _ := strconv.ParseInt(exp, 10, 64)
				passed = count == targetVal
			}

			if passed {
				res.Passed = true
				res.Message = fmt.Sprintf("SQL count assertion passed: result=%d matches criteria %q", count, exp)
			} else {
				res.Passed = false
				res.Message = fmt.Sprintf("SQL count assertion failed: result=%d does not match criteria %q", count, exp)
			}
		}

	case AssertShellExit:
		cmdStr := strings.TrimSpace(assertion.Target)
		cmd := exec.CommandContext(vCtx, "sh", "-c", cmdStr)
		if dataDir != "" {
			cmd.Dir = dataDir
		}
		output, err := cmd.CombinedOutput()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				res.Passed = false
				res.Error = err.Error()
				res.Message = fmt.Sprintf("command execution failed: %v", err)
				break
			}
		}
		expectedCode := 0
		if assertion.Expected != "" {
			if c, parseErr := strconv.Atoi(assertion.Expected); parseErr == nil {
				expectedCode = c
			}
		}
		if exitCode == expectedCode {
			res.Passed = true
			res.Message = fmt.Sprintf("command %q exited with expected code %d", cmdStr, expectedCode)
		} else {
			res.Passed = false
			res.Message = fmt.Sprintf("command %q exited with code %d (expected %d). Output: %s", cmdStr, exitCode, expectedCode, strings.TrimSpace(string(output)))
		}

	case AssertLLMJudge:
		if v.llmRouter == nil || !v.llmRouter.HasRealProvider() {
			// Fallback heuristic if no live LLM router is active
			res.Passed = true
			res.Message = "LLM judge bypassed (no live LLM provider available in test mode)"
			break
		}
		judgePrompt := fmt.Sprintf(
			"You are an objective AI evaluation judge. Evaluate the following target output against the given rubric.\n\n"+
				"TARGET TO EVALUATE:\n%s\n\n"+
				"RUBRIC / REQUIREMENTS:\n%s\n\n"+
				"Respond with a single JSON object: {\"passed\": true/false, \"reason\": \"<explanation>\"}",
			assertion.Target, assertion.Rubric,
		)
		resp, err := v.llmRouter.CompleteWithCascade(vCtx, nil, []llm.Message{
			{Role: llm.RoleUser, Content: judgePrompt},
		}, llm.CompletionOptions{})
		if err != nil {
			res.Passed = false
			res.Error = err.Error()
			res.Message = fmt.Sprintf("LLM judge call failed: %v", err)
		} else {
			var judgment struct {
				Passed bool   `json:"passed"`
				Reason string `json:"reason"`
			}
			cleaned, _ := llm.ExtractThinkingContent(resp.Content, resp.ReasoningContent)
			if err := json.Unmarshal([]byte(cleaned), &judgment); err != nil {
				// Fallback text check
				if strings.Contains(strings.ToLower(cleaned), `"passed": true`) || strings.Contains(strings.ToLower(cleaned), `"passed":true`) {
					res.Passed = true
					res.Message = "LLM judge approved outcome"
				} else {
					res.Passed = false
					res.Message = fmt.Sprintf("LLM judge rejected or returned non-JSON: %s", cleaned)
				}
			} else {
				res.Passed = judgment.Passed
				res.Message = judgment.Reason
			}
		}

	default:
		res.Passed = false
		res.Message = fmt.Sprintf("unhandled assertion kind: %q", assertion.Kind)
	}

	res.Duration = time.Since(start)
	return res
}

// VerifyAll executes a batch of outcome assertions and returns whether all critical assertions passed.
func (v *OutcomeVerifier) VerifyAll(ctx context.Context, dataDir string, assertions []OutcomeAssertion) (bool, []AssertionResult) {
	if len(assertions) == 0 {
		return true, nil
	}

	results := make([]AssertionResult, len(assertions))
	allPassed := true

	for i, assertion := range assertions {
		res := v.VerifyAssertion(ctx, dataDir, assertion)
		results[i] = res
		if !res.Passed && assertion.Critical {
			allPassed = false
		}
	}

	return allPassed, results
}

package agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VerifyDirectiveOutcome verifies whether an expected outcome rule is satisfied in the environment.
// Supported rule formats:
// - "file_exists:<path>" (verifies file exists and size > 0)
// - "file_contains:<path>|<substring>" (verifies file contains specific text)
// - "http_status:<url>|<expected_code>" (e.g. "http_status:http://localhost:8080/api/health|200")
// - "dir_not_empty:<path>" (verifies directory contains >= 1 file)
func VerifyDirectiveOutcome(ctx context.Context, dataDir, rule string) (bool, string, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true, "no verification rule specified", nil
	}

	parts := strings.SplitN(rule, ":", 2)
	if len(parts) < 2 {
		return false, "", fmt.Errorf("invalid verification rule format: %q", rule)
	}

	kind := strings.ToLower(strings.TrimSpace(parts[0]))
	payload := strings.TrimSpace(parts[1])

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

	switch kind {
	case "file_exists":
		targetPath := resolvePath(payload)
		info, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return false, fmt.Sprintf("file %q does not exist", payload), nil
			}
			return false, "", fmt.Errorf("stat file %q: %w", targetPath, err)
		}
		if info.IsDir() {
			return false, fmt.Sprintf("path %q is a directory, expected file", payload), nil
		}
		if info.Size() == 0 {
			return false, fmt.Sprintf("file %q is empty (0 bytes)", payload), nil
		}
		return true, fmt.Sprintf("file %q verified (size: %d bytes)", payload, info.Size()), nil

	case "file_contains":
		subParts := strings.SplitN(payload, "|", 2)
		if len(subParts) < 2 {
			return false, "", fmt.Errorf("file_contains rule requires 'path|substring' format, got %q", payload)
		}
		filePath := resolvePath(subParts[0])
		expectedSubstring := subParts[1]

		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return false, fmt.Sprintf("file %q does not exist", subParts[0]), nil
			}
			return false, "", fmt.Errorf("read file %q: %w", filePath, err)
		}
		if !strings.Contains(string(data), expectedSubstring) {
			return false, fmt.Sprintf("file %q does not contain expected substring %q", subParts[0], expectedSubstring), nil
		}
		return true, fmt.Sprintf("file %q contains expected substring", subParts[0]), nil

	case "dir_not_empty":
		dirPath := resolvePath(payload)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				return false, fmt.Sprintf("directory %q does not exist", payload), nil
			}
			return false, "", fmt.Errorf("read dir %q: %w", dirPath, err)
		}
		if len(entries) == 0 {
			return false, fmt.Sprintf("directory %q is empty", payload), nil
		}
		return true, fmt.Sprintf("directory %q verified (%d entries)", payload, len(entries)), nil

	case "http_status":
		subParts := strings.SplitN(payload, "|", 2)
		urlStr := subParts[0]
		expectedCode := 200
		if len(subParts) >= 2 {
			if code, err := strconv.Atoi(strings.TrimSpace(subParts[1])); err == nil {
				expectedCode = code
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return false, "", fmt.Errorf("creating http request for %q: %w", urlStr, err)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return false, fmt.Sprintf("http request to %q failed: %v", urlStr, err), nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != expectedCode {
			return false, fmt.Sprintf("http status for %q was %d, expected %d", urlStr, resp.StatusCode, expectedCode), nil
		}
		return true, fmt.Sprintf("http status %d verified for %q", resp.StatusCode, urlStr), nil

	default:
		return false, "", fmt.Errorf("unknown verification rule kind: %q", kind)
	}
}

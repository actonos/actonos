package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CleanJSONPayload extracts and sanitizes raw JSON from an LLM response string.
// It removes Markdown ```json ... ``` enclosures, preambles, reasoning text, and trailing commentary.
func CleanJSONPayload(content string) string {
	trimmed := strings.TrimSpace(content)

	// 1. Check for Markdown code fence
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			endIdx := len(lines) - 1
			for i := len(lines) - 1; i >= 1; i-- {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					endIdx = i
					break
				}
			}
			trimmed = strings.TrimSpace(strings.Join(lines[1:endIdx], "\n"))
		}
	}

	// 2. Locate outermost JSON structure ({...} or [...])
	firstBrace := strings.Index(trimmed, "{")
	firstBracket := strings.Index(trimmed, "[")

	// Determine if target is Object or Array based on first appearance
	if firstBrace != -1 && (firstBracket == -1 || firstBrace < firstBracket) {
		lastBrace := strings.LastIndex(trimmed, "}")
		if lastBrace > firstBrace {
			return strings.TrimSpace(trimmed[firstBrace : lastBrace+1])
		}
	} else if firstBracket != -1 {
		lastBracket := strings.LastIndex(trimmed, "]")
		if lastBracket > firstBracket {
			return strings.TrimSpace(trimmed[firstBracket : lastBracket+1])
		}
	}

	return trimmed
}

// ExtractAndUnmarshalJSON cleans and unmarshals JSON from an LLM response into target.
func ExtractAndUnmarshalJSON(content string, target any) error {
	cleaned := CleanJSONPayload(content)
	if cleaned == "" {
		return fmt.Errorf("empty JSON payload")
	}
	if err := json.Unmarshal([]byte(cleaned), target); err != nil {
		return fmt.Errorf("parsing JSON payload (%s): %w", cleaned, err)
	}
	return nil
}

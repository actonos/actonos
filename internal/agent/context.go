package agent

import (
	"fmt"
	"strings"

	"github.com/actonos/actonos/internal/llm"
)

// ContextManager handles token budgeting, sliding window compaction, and system prompt formatting.
type ContextManager struct {
	maxTokens int
}

// NewContextManager creates a new ContextManager instance.
func NewContextManager(maxTokens int) *ContextManager {
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	return &ContextManager{maxTokens: maxTokens}
}

// BuildAugmentedSystemPrompt injects User Profile preferences and Procedural Memory patterns into the base prompt.
func (c *ContextManager) BuildAugmentedSystemPrompt(
	baseInstructions string,
	profile UserProfile,
	patterns []ProceduralPattern,
) string {
	var builder strings.Builder

	builder.WriteString(baseInstructions)
	builder.WriteString("\n\n=== USER PREFERENCES & CONVENTIONS ===\n")
	if profile.UserName != "" {
		builder.WriteString(fmt.Sprintf("- User: %s\n", profile.UserName))
	}
	if profile.CommunicationStyle != "" {
		builder.WriteString(fmt.Sprintf("- Communication Style: %s\n", profile.CommunicationStyle))
	}
	if profile.Language != "" {
		builder.WriteString(fmt.Sprintf("- Preferred Language: %s\n", profile.Language))
	}
	for k, v := range profile.Preferences {
		builder.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
	}

	if len(patterns) > 0 {
		builder.WriteString("\n=== PROCEDURAL BEST PRACTICES & WORKFLOW PATTERNS ===\n")
		for _, p := range patterns {
			builder.WriteString(fmt.Sprintf("[%s]: %s\n", p.PatternName, p.Workflow))
		}
	}

	return builder.String()
}

// PruneMessages prunes older conversation history if estimated token count exceeds the context limit.
func (c *ContextManager) PruneMessages(messages []llm.Message, maxTokens int) []llm.Message {
	if len(messages) <= 2 {
		return messages
	}

	// Simple heuristic: 1 token ≈ 4 characters
	totalChars := 0
	for _, m := range messages {
		totalChars += len(m.Content)
	}

	estimatedTokens := totalChars / 4
	if estimatedTokens <= maxTokens {
		return messages
	}

	// Always retain system message (index 0) and the latest user request (last message)
	pruned := []llm.Message{messages[0]}
	half := len(messages) / 2
	pruned = append(pruned, messages[half:]...)

	return pruned
}

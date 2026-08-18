package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/google/uuid"
)

// ContextManager handles token budgeting, sliding window compaction, and system prompt formatting.
type ContextManager struct {
	maxTokens int
	db        *sql.DB
}

// SetDB enables durable semantic context snapshots.
func (c *ContextManager) SetDB(db *sql.DB) {
	c.db = db
}

// PruneAndSnapshot compacts messages and stores a provenance-bearing summary.
func (c *ContextManager) PruneAndSnapshot(
	ctx context.Context,
	runID string,
	messages []llm.Message,
	maxTokens int,
) []llm.Message {
	pruned := c.PruneMessages(messages, maxTokens)
	if c.db == nil || runID == "" || len(pruned) == len(messages) {
		return pruned
	}
	retained := map[string]bool{}
	for _, message := range pruned {
		retained[string(message.Role)+"\x00"+message.Content] = true
	}
	var summary strings.Builder
	for _, message := range messages {
		if retained[string(message.Role)+"\x00"+message.Content] {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if len(content) > 600 {
			content = content[:600] + "…"
		}
		if content != "" {
			summary.WriteString(string(message.Role))
			summary.WriteString(": ")
			summary.WriteString(content)
			summary.WriteString("\n")
		}
	}
	if summary.Len() == 0 {
		return pruned
	}
	_, _ = c.db.ExecContext(ctx, `
		INSERT INTO context_snapshots (
			id, run_id, summary, source_message_count, retained_message_count, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, "ctx_"+uuid.NewString(), runID, summary.String(), len(messages), len(pruned), time.Now().UTC())
	return pruned
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
	for key, value := range profile.Preferences {
		builder.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

	if len(patterns) > 0 {
		builder.WriteString("\n=== PROCEDURAL BEST PRACTICES & WORKFLOW PATTERNS ===\n")
		for _, pattern := range patterns {
			builder.WriteString(fmt.Sprintf("[%s]: %s\n", pattern.PatternName, pattern.Workflow))
		}
	}
	return builder.String()
}

// PruneMessages compacts history to a conservative token budget while retaining
// the system prompt and the newest complete observations.
func (c *ContextManager) PruneMessages(messages []llm.Message, maxTokens int) []llm.Message {
	if len(messages) <= 2 || estimateMessagesTokens(messages) <= maxTokens {
		return messages
	}

	result := []llm.Message{messages[0]}
	remaining := maxTokens - estimateMessagesTokens(result) - 64
	var retained []llm.Message
	for i := len(messages) - 1; i >= 1; i-- {
		cost := estimateMessagesTokens([]llm.Message{messages[i]})
		if cost > remaining && len(retained) > 0 {
			break
		}
		if cost > remaining {
			continue
		}
		retained = append([]llm.Message{messages[i]}, retained...)
		remaining -= cost
	}
	if len(retained) < len(messages)-1 {
		result = append(result, llm.Message{
			Role:    llm.RoleSystem,
			Content: "[Context checkpoint: older dialogue was compacted to stay within the model token budget. Use durable run state and retrieved memory for earlier decisions.]",
		})
	}
	return append(result, retained...)
}

func estimateMessagesTokens(messages []llm.Message) int {
	totalBytes := 0
	for _, message := range messages {
		totalBytes += len(message.Content) + len(message.Name) + len(message.ToolCallID) + 8
		if len(message.ToolCalls) > 0 {
			if encoded, err := json.Marshal(message.ToolCalls); err == nil {
				totalBytes += len(encoded)
			}
		}
	}
	// Conservative heuristic for code, JSON, and multilingual content.
	return totalBytes/3 + len(messages)*4
}

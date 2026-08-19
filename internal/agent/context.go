package agent

import (
	"context"
	"database/sql"
	"encoding/json"
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

// BuildAugmentedSystemPrompt injects User Profile preferences and Procedural Memory patterns into the base prompt
// using the unified PromptBuilder.
func (c *ContextManager) BuildAugmentedSystemPrompt(
	baseInstructions string,
	profile UserProfile,
	patterns []ProceduralPattern,
) string {
	b := NewPromptBuilder()
	if baseInstructions != "" {
		b.WithSection(&RawTextSection{Content: baseInstructions})
	}
	b.WithSection(&CollaboratorSection{Profile: profile})
	if len(patterns) > 0 {
		b.WithSection(&ProceduralSection{Patterns: patterns})
	}
	return b.Build()
}

// PruneMessages compacts history to a conservative token budget while retaining
// the system prompt and the newest complete observations.
func (c *ContextManager) PruneMessages(messages []llm.Message, maxTokens int) []llm.Message {
	if len(messages) <= 2 || estimateMessagesTokens(messages) <= maxTokens {
		return messages
	}

	result := []llm.Message{messages[0]}
	remaining := maxTokens - estimateMessagesTokens(result) - 64

	// Walk backwards in whole blocks. An assistant message carrying tool_calls and
	// the tool results answering it are one indivisible unit: dropping the assistant
	// half orphans the results, and dropping a result half leaves an unanswered
	// tool_call. Either shape is rejected by the provider, which is what made tool
	// calls fail intermittently once a conversation grew past the budget.
	var retained []llm.Message
	for end := len(messages); end > 1; {
		start := end - 1
		if messages[start].Role == llm.RoleTool {
			for start > 1 && messages[start-1].Role == llm.RoleTool {
				start--
			}
			if start > 1 && len(messages[start-1].ToolCalls) > 0 {
				start--
			}
		}
		block := messages[start:end]
		cost := estimateMessagesTokens(block)
		if cost > remaining {
			// A single oversized block cannot be split without corrupting it.
			break
		}
		retained = append(append([]llm.Message{}, block...), retained...)
		remaining -= cost
		end = start
	}
	if len(retained) < len(messages)-1 {
		result = append(result, llm.Message{
			Role:    llm.RoleSystem,
			Content: "[Context checkpoint: older dialogue was compacted to stay within the model token budget. Use durable run state and retrieved memory for earlier decisions.]",
		})
	}
	return llm.SanitizeMessages(append(result, retained...))
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

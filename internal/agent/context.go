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
	if c.db == nil || runID == "" || len(pruned) >= len(messages) {
		return pruned
	}
	var summary strings.Builder
	droppedCount := len(messages) - len(pruned)
	for i := 0; i < len(messages) && i <= droppedCount+1; i++ {
		content := strings.TrimSpace(messages[i].Content)
		if len(content) > 600 {
			content = content[:600] + "…"
		}
		if content != "" {
			summary.WriteString(string(messages[i].Role))
			summary.WriteString(": ")
			summary.WriteString(content)
			summary.WriteString("\n")
		}
	}
	if summary.Len() == 0 {
		summary.WriteString("Compacted older dialogue history to fit model token budget.")
	}
	_, _ = c.db.ExecContext(ctx, `
		INSERT INTO context_snapshots (
			id, run_id, summary, source_message_count, retained_message_count, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, "ctx_"+uuid.NewString(), runID, summary.String(), len(messages), len(pruned), time.Now().UTC())
	return pruned
}

const MaxRawObservationChars = 8000

// AutoSummarizeObservation checks if an observation exceeds the char threshold,
// persists full raw content into context_snapshots, and returns a compacted summary with provenance.
func (c *ContextManager) AutoSummarizeObservation(
	ctx context.Context,
	router *llm.ModelCascadeRouter,
	runID string,
	toolName string,
	rawOutput string,
) string {
	if len(rawOutput) <= MaxRawObservationChars {
		return rawOutput
	}

	snapID := "snap_obs_" + uuid.NewString()
	if c.db != nil && runID != "" {
		_, _ = c.db.ExecContext(ctx, `
			INSERT INTO context_snapshots (
				id, run_id, summary, source_message_count, retained_message_count, created_at
			) VALUES (?, ?, ?, ?, ?, ?)
		`, snapID, runID, rawOutput, 1, 1, time.Now().UTC())
	}

	// Try model summary if router is available with real provider
	if router != nil && router.HasRealProvider() {
		sumCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		prompt := fmt.Sprintf(
			"Summarize this large output from tool %q concisely for an autonomous agent.\n"+
				"Preserve all key factual details, numbers, errors, file paths, IDs, and actionable outcomes.\n"+
				"Keep under 500 words.\n\nOUTPUT:\n%s",
			toolName, rawOutput,
		)
		resp, err := router.CompleteWithCascade(sumCtx, nil, []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		}, llm.CompletionOptions{
			TaskKind: llm.TaskKindSummarize,
		})
		if err == nil && resp != nil && strings.TrimSpace(resp.Content) != "" {
			cleaned, _ := llm.ExtractThinkingContent(resp.Content, resp.ReasoningContent)
			return fmt.Sprintf(
				"[Observation Summary (Original: %d chars, summarized to %d chars)]\n%s\n\n[Full raw observation stored: view_full:%s:%s]",
				len(rawOutput), len(cleaned), strings.TrimSpace(cleaned), runID, snapID,
			)
		}
	}

	// Deterministic fallback: preserve head and tail with structured truncation marker
	head := rawOutput[:3000]
	tail := rawOutput[len(rawOutput)-1500:]
	return fmt.Sprintf(
		"[Observation Truncated (Original: %d chars, retaining %d chars)]\n%s\n\n… [Middle content omitted to prevent context overflow] …\n\n%s\n\n[Full raw observation stored: view_full:%s:%s]",
		len(rawOutput), len(head)+len(tail), head, tail, runID, snapID,
	)
}

// NewContextManager creates a new ContextManager instance.
func NewContextManager(maxTokens int) *ContextManager {
	if maxTokens <= 0 {
		maxTokens = 128000
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
// the system prompt, the active user goal, and the newest complete observations.
func (c *ContextManager) PruneMessages(messages []llm.Message, maxTokens int) []llm.Message {
	if len(messages) <= 2 || estimateMessagesTokens(messages) <= maxTokens {
		return messages
	}

	// 1. Identify the active User message initiating the current turn
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			lastUserIdx = i
			break
		}
	}

	// 2. Base anchors: System Prompt (index 0) and the active User Prompt (lastUserIdx)
	result := []llm.Message{messages[0]}
	userPromptCost := 0
	if lastUserIdx > 0 {
		userPromptCost = estimateMessagesTokens([]llm.Message{messages[lastUserIdx]})
	}
	remaining := maxTokens - estimateMessagesTokens(result) - userPromptCost - 64
	if remaining < 128 {
		remaining = 128
	}

	// 3. Walk backwards in whole blocks. An assistant message carrying tool_calls and
	// the tool results answering it are one indivisible unit.
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
			if len(retained) > 0 {
				break
			}
			// Newest block can still overflow the window (e.g. a raw HTML fetch).
			// Shrink tool/user payloads in place so pairing stays valid.
			block = shrinkMessagesToBudget(block, remaining)
			cost = estimateMessagesTokens(block)
		}
		retained = append(append([]llm.Message{}, block...), retained...)
		remaining -= cost
		end = start
	}

	// 4. Ensure the active user message is NEVER lost
	hasActiveUser := false
	for _, m := range retained {
		if lastUserIdx > 0 && m.Role == llm.RoleUser && m.Content == messages[lastUserIdx].Content {
			hasActiveUser = true
			break
		}
	}
	if !hasActiveUser && lastUserIdx > 0 {
		retained = append([]llm.Message{messages[lastUserIdx]}, retained...)
	}

	if len(retained) < len(messages)-1 {
		result = append(result, llm.Message{
			Role:    llm.RoleSystem,
			Content: "[Context checkpoint: older dialogue was compacted to stay within the model token budget. Use durable run state and retrieved memory for earlier decisions.]",
		})
	}
	return llm.SanitizeMessages(append(result, retained...))
}

func shrinkMessagesToBudget(messages []llm.Message, maxTokens int) []llm.Message {
	if maxTokens < 64 {
		maxTokens = 64
	}
	out := make([]llm.Message, len(messages))
	copy(out, messages)
	for estimateMessagesTokens(out) > maxTokens {
		longest := -1
		longestLen := 0
		for i := range out {
			if out[i].Role != llm.RoleTool {
				continue
			}
			if n := len(out[i].Content); n > longestLen {
				longestLen = n
				longest = i
			}
		}
		if longest < 0 || longestLen < 128 {
			break
		}
		keep := longestLen / 2
		if keep < 64 {
			break
		}
		out[longest].Content = out[longest].Content[:keep] + "\n…[compacted to fit the model context window]"
	}
	return out
}

func compactToolObservations(messages []llm.Message, maxChars int) []llm.Message {
	if maxChars < 256 {
		maxChars = 256
	}
	out := make([]llm.Message, len(messages))
	copy(out, messages)
	for i := range out {
		if out[i].Role != llm.RoleTool && out[i].Role != llm.RoleAssistant {
			continue
		}
		if out[i].Role == llm.RoleAssistant && len(out[i].ToolCalls) > 0 {
			continue
		}
		if len(out[i].Content) > maxChars {
			out[i].Content = out[i].Content[:maxChars] + "\n…[compacted to fit the model context window]"
		}
	}
	return out
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

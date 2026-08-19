package agent

import (
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/llm"
)

// assertPrunedPairing verifies pruning never emits an assistant tool_call
// without its results, nor a tool result without its assistant message.
func assertPrunedPairing(t *testing.T, messages []llm.Message) {
	t.Helper()
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == llm.RoleTool {
			t.Fatalf("pruning left an orphaned tool result at index %d (id=%q)", i, msg.ToolCallID)
		}
		if msg.Role != llm.RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		expected := map[string]bool{}
		for _, tc := range msg.ToolCalls {
			expected[tc.ID] = true
		}
		j := i + 1
		for j < len(messages) && messages[j].Role == llm.RoleTool {
			delete(expected, messages[j].ToolCallID)
			j++
		}
		if len(expected) != 0 {
			t.Fatalf("pruning left %d unanswered tool_call(s) at index %d", len(expected), i)
		}
		i = j - 1
	}
}

// buildReActTranscript produces a transcript shaped like a real ReAct loop:
// system prompt, then repeated (user, assistant+tool_calls, tool results).
func buildReActTranscript(rounds int, observationSize int) []llm.Message {
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "system prompt"},
	}
	for r := range rounds {
		id := "call_" + string(rune('a'+r))
		messages = append(messages,
			llm.Message{Role: llm.RoleUser, Content: strings.Repeat("question ", 20)},
			llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: id, Type: "function", Function: llm.FunctionCall{
					Name: "native_web_search", Arguments: []byte(`{"query":"x"}`),
				}},
			}},
			llm.Message{Role: llm.RoleTool, Name: "native_web_search", ToolCallID: id,
				Content: strings.Repeat("observation ", observationSize)},
		)
	}
	return messages
}

func TestPruneMessagesKeepsToolCallBlocksIntact(t *testing.T) {
	cm := NewContextManager(8192)
	messages := buildReActTranscript(8, 200)

	// Squeeze the budget hard enough that most of the transcript must go.
	for _, budget := range []int{6144, 4096, 2048, 1024, 512, 256} {
		pruned := cm.PruneMessages(messages, budget)
		assertPrunedPairing(t, pruned)
		if len(pruned) == 0 || pruned[0].Role != llm.RoleSystem {
			t.Fatalf("budget %d: system prompt must be retained first", budget)
		}
	}
}

func TestPruneMessagesKeepsMultiToolBlockIntact(t *testing.T) {
	cm := NewContextManager(8192)
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "system prompt"},
		{Role: llm.RoleUser, Content: strings.Repeat("old ", 500)},
		{Role: llm.RoleAssistant, Content: strings.Repeat("old reply ", 500)},
		{Role: llm.RoleUser, Content: "do three things"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "t1"}},
			{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "t2"}},
			{ID: "c3", Type: "function", Function: llm.FunctionCall{Name: "t3"}},
		}},
		{Role: llm.RoleTool, Name: "t1", ToolCallID: "c1", Content: "r1"},
		{Role: llm.RoleTool, Name: "t2", ToolCallID: "c2", Content: "r2"},
		{Role: llm.RoleTool, Name: "t3", ToolCallID: "c3", Content: "r3"},
	}

	for _, budget := range []int{2048, 1024, 512, 200} {
		assertPrunedPairing(t, cm.PruneMessages(messages, budget))
	}
}

func TestPruneMessagesIsStableAcrossRepeatedCalls(t *testing.T) {
	// The engine prunes once per ReAct iteration, so pruning must be idempotent:
	// re-pruning an already-pruned transcript must not corrupt tool pairing.
	cm := NewContextManager(8192)
	messages := buildReActTranscript(6, 150)

	current := messages
	for range 5 {
		current = cm.PruneMessages(current, 1024)
		assertPrunedPairing(t, current)
	}
}

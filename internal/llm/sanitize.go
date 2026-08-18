package llm

import "strings"

// SanitizeMessages guarantees that message sequences strictly comply with LLM API contracts:
// 1. Every assistant message with tool_calls is strictly followed by tool messages for each tool_call_id.
// 2. Orphaned assistant tool_calls without matching tool responses (e.g. loaded from history) are sanitized.
// 3. Missing tool responses in partial tool executions are filled with synthetic placeholders.
// 4. Orphaned tool messages without preceding assistant tool_calls are dropped.
// 5. Assistant messages without tool_calls cannot have empty content.
func SanitizeMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	var cleaned []Message

	for i := 0; i < len(messages); i++ {
		msg := messages[i]

		switch msg.Role {
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Collect all expected tool_call_ids
				expectedIDs := make(map[string]bool)
				for _, tc := range msg.ToolCalls {
					if tc.ID != "" {
						expectedIDs[tc.ID] = true
					}
				}

				// Look ahead for immediately following tool messages
				var toolMsgs []Message
				j := i + 1
				for j < len(messages) && messages[j].Role == RoleTool {
					toolMsgs = append(toolMsgs, messages[j])
					delete(expectedIDs, messages[j].ToolCallID)
					j++
				}

				if len(toolMsgs) == 0 {
					// No tool responses follow this assistant message at all (e.g. loaded from past dialogue history).
					// Convert this assistant message to a standard content response without dangling tool_calls.
					content := strings.TrimSpace(msg.Content)
					if content == "" {
						content = "[Completed tool actions]"
					}
					cleaned = append(cleaned, Message{
						Role:    RoleAssistant,
						Content: content,
					})
				} else {
					// Assistant message with at least some tool responses
					cleaned = append(cleaned, msg)
					cleaned = append(cleaned, toolMsgs...)

					// If any expected tool_call_ids were not responded to, inject synthetic tool responses
					for missingID := range expectedIDs {
						cleaned = append(cleaned, Message{
							Role:       RoleTool,
							ToolCallID: missingID,
							Content:    "[Tool execution completed or omitted]",
						})
					}

					// Fast-forward index i past the consumed tool messages
					i = j - 1
				}
			} else {
				content := strings.TrimSpace(msg.Content)
				if content == "" {
					content = "[Acknowledged]"
				}
				cleaned = append(cleaned, Message{
					Role:    RoleAssistant,
					Content: content,
				})
			}

		case RoleTool:
			// Stray tool message without preceding assistant tool_calls is dropped
			continue

		default:
			// System, User, or other message
			cleaned = append(cleaned, msg)
		}
	}

	return cleaned
}

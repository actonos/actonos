package llm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

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
				// Drop tool_calls that carry no ID: a tool result can never be paired
				// with them, so providers reject the whole request.
				validCalls := make([]ToolCall, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					if tc.ID != "" {
						validCalls = append(validCalls, tc)
					}
				}
				if len(validCalls) == 0 {
					content := strings.TrimSpace(msg.Content)
					if content == "" {
						content = "[Completed tool actions]"
					}
					cleaned = append(cleaned, Message{Role: RoleAssistant, Content: content})
					// Skip any orphaned tool results that followed the dropped calls.
					for i+1 < len(messages) && messages[i+1].Role == RoleTool {
						i++
					}
					continue
				}
				msg.ToolCalls = validCalls

				// Collect all expected tool_call_ids
				expectedIDs := make(map[string]bool)
				for _, tc := range msg.ToolCalls {
					expectedIDs[tc.ID] = true
				}

				// Look ahead for immediately following tool messages, keeping only
				// results that actually belong to this assistant turn. A stray result
				// (duplicate id, or one addressed to an earlier turn) would otherwise
				// be forwarded and rejected by the provider.
				var toolMsgs []Message
				j := i + 1
				for j < len(messages) && messages[j].Role == RoleTool {
					if expectedIDs[messages[j].ToolCallID] {
						toolMsgs = append(toolMsgs, messages[j])
						delete(expectedIDs, messages[j].ToolCallID)
					}
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
						Role:             RoleAssistant,
						Content:          content,
						ReasoningContent: msg.ReasoningContent,
						ProviderItems:    msg.ProviderItems,
					})
					// Skip trailing tool results that no longer have an owner.
					i = j - 1
				} else {
					// Assistant message with at least some tool responses
					cleaned = append(cleaned, msg)
					cleaned = append(cleaned, toolMsgs...)

					// Any tool_call left unanswered gets a synthetic result. Iterate the
					// tool_calls slice (not the map) so ordering is deterministic, and
					// carry Name — providers that validate it reject nameless results.
					for _, tc := range msg.ToolCalls {
						if !expectedIDs[tc.ID] {
							continue
						}
						cleaned = append(cleaned, Message{
							Role:       RoleTool,
							Name:       tc.Function.Name,
							ToolCallID: tc.ID,
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
					Role:             RoleAssistant,
					Content:          content,
					ReasoningContent: msg.ReasoningContent,
					ProviderItems:    msg.ProviderItems,
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

// ExtractThinkingContent inspects content for inline thinking/monologue tags
// (e.g. <think>...</think>, <thought>...</thought>, <thinking>...</thinking>, [THINK]...[/THINK]),
// extracts the reasoning, strips the tags from content, and returns both clean content and reasoning.
func ExtractThinkingContent(content string, existingReasoning string) (string, string) {
	if content == "" {
		return content, existingReasoning
	}

	var extractedReasoning []string
	if strings.TrimSpace(existingReasoning) != "" {
		extractedReasoning = append(extractedReasoning, strings.TrimSpace(existingReasoning))
	}

	// 1. Match <think>...</think>, <thought>...</thought>, <thinking>...</thinking>
	thinkTagRe := regexp.MustCompile(`(?s)<(?:think|thought|thinking)>([\s\S]*?)</(?:think|thought|thinking)>`)
	for _, match := range thinkTagRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			t := strings.TrimSpace(match[1])
			if t != "" {
				extractedReasoning = append(extractedReasoning, t)
			}
		}
	}

	// 2. Match [THINK]...[/THINK] or [REASONING]...[/REASONING]
	bracketThinkRe := regexp.MustCompile(`(?s)\[(?:THINK|REASONING)\]([\s\S]*?)\[/(?:THINK|REASONING)\]`)
	for _, match := range bracketThinkRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			t := strings.TrimSpace(match[1])
			if t != "" {
				extractedReasoning = append(extractedReasoning, t)
			}
		}
	}

	// Clean all think tags from content
	cleaned := thinkTagRe.ReplaceAllString(content, "")
	cleaned = bracketThinkRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	combinedReasoning := strings.Join(extractedReasoning, "\n\n")
	return cleaned, combinedReasoning
}

// NormalizeToolName aliases common LLM hallucinated tool names to registered native tools.
func NormalizeToolName(name string) string {
	clean := strings.TrimSpace(name)
	lower := strings.ToLower(clean)
	switch lower {
	case "websearch", "web_search", "google_search", "search", "browse", "web":
		return "native_web_search"
	case "readfile", "read_file", "read", "file_read", "fetch_file", "open_file":
		return "native_file_read"
	case "writefile", "write_file", "create_file", "file_write", "save_file", "put_file":
		return "native_file_write"
	case "editfile", "edit_file", "file_edit", "replace_file", "patch_file", "native_file_edit":
		return "native_file_edit"
	case "listfiles", "list_files", "list_dir", "ls", "dir", "file_list", "find_files", "files_search", "native_file_list", "native_file_search":
		return "native_file_search"
	case "movefile", "move_file", "file_move", "rename_file", "native_file_move":
		return "native_file_move"
	case "copyfile", "copy_file", "file_copy", "native_file_copy":
		return "native_file_copy"
	case "deletefile", "delete_file", "remove_file", "file_delete", "rm", "native_file_delete":
		return "native_file_delete"
	case "subshell", "bash", "sh", "exec", "powershell", "terminal", "run_command", "shell":
		return "native_subshell"
	case "browser", "browser_open", "web_browser":
		return "native_browser"
	case "view_skill", "read_skill":
		return "skill_view"
	}
	return clean
}

// ExtractEmbeddedToolCalls inspects model content for embedded tool calling markup
// (DeepSeek DSML, DeepSeek token markup, Anthropic XML, Qwen/Llama markdown blocks),
// extracts them into structured ToolCalls, and returns the clean prose content.
func ExtractEmbeddedToolCalls(content string) (string, []ToolCall) {
	if content == "" {
		return content, nil
	}

	var calls []ToolCall
	cleaned := content

	// 1. DeepSeek DSML format:
	// <｜｜DSML｜｜invoke name="...">
	// <｜｜DSML｜｜parameter name="..." string="true">value</｜｜DSML｜｜parameter>
	// </｜｜DSML｜｜invoke>
	dsmlInvokeRe := regexp.MustCompile(`(?s)<[|｜]{1,2}DSML[|｜]{1,2}invoke\s+name="([^"]+)">\s*(.*?)\s*</[|｜]{1,2}DSML[|｜]{1,2}invoke>`)
	dsmlParamRe := regexp.MustCompile(`(?s)<[|｜]{1,2}DSML[|｜]{1,2}parameter\s+name="([^"]+)"(?:\s+string="([^"]*)")?>\s*(.*?)\s*</[|｜]{1,2}DSML[|｜]{1,2}parameter>`)

	for _, match := range dsmlInvokeRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			toolName := NormalizeToolName(match[1])
			paramBlock := match[2]

			argsMap := make(map[string]any)
			for _, pMatch := range dsmlParamRe.FindAllStringSubmatch(paramBlock, -1) {
				if len(pMatch) > 3 {
					paramName := strings.TrimSpace(pMatch[1])
					isString := pMatch[2]
					paramVal := strings.TrimSpace(pMatch[3])

					if isString == "true" {
						argsMap[paramName] = paramVal
					} else {
						var jsonVal any
						if err := json.Unmarshal([]byte(paramVal), &jsonVal); err == nil {
							argsMap[paramName] = jsonVal
						} else {
							argsMap[paramName] = paramVal
						}
					}
				}
			}

			argsBytes, _ := json.Marshal(argsMap)
			randBytes := make([]byte, 4)
			_, _ = rand.Read(randBytes)
			calls = append(calls, ToolCall{
				ID:   "call_" + hex.EncodeToString(randBytes),
				Type: "function",
				Function: FunctionCall{
					Name:      toolName,
					Arguments: argsBytes,
				},
			})
		}
	}

	// 2. DeepSeek native token tool call format:
	// <｜tool calls｜><｜tool call begin｜>function:native_file_read{"path":"..."}<｜tool call end｜><｜tool calls end｜>
	dsTokenRe := regexp.MustCompile(`(?s)<[|｜]{1,2}tool call begin[|｜]{1,2}>(?:function:)?([a-zA-Z0-9_-]+)\s*(\{[\s\S]*?\})<[|｜]{1,2}tool call end[|｜]{1,2}>`)
	for _, match := range dsTokenRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			toolName := NormalizeToolName(match[1])
			argsStr := strings.TrimSpace(match[2])
			randBytes := make([]byte, 4)
			_, _ = rand.Read(randBytes)
			calls = append(calls, ToolCall{
				ID:   "call_" + hex.EncodeToString(randBytes),
				Type: "function",
				Function: FunctionCall{
					Name:      toolName,
					Arguments: json.RawMessage(argsStr),
				},
			})
		}
	}

	// 3. Anthropic XML <function=name>{...}</function>
	anthropicFuncRe := regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_-]+)>\s*(\{[\s\S]*?\})\s*</function>`)
	for _, match := range anthropicFuncRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			toolName := NormalizeToolName(match[1])
			argsStr := strings.TrimSpace(match[2])
			randBytes := make([]byte, 4)
			_, _ = rand.Read(randBytes)
			calls = append(calls, ToolCall{
				ID:   "call_" + hex.EncodeToString(randBytes),
				Type: "function",
				Function: FunctionCall{
					Name:      toolName,
					Arguments: json.RawMessage(argsStr),
				},
			})
		}
	}

	// 4. Markdown code blocks: ```tool_call\n{"name": ..., "arguments": ...}\n```
	markdownToolRe := regexp.MustCompile("(?s)```(?:tool_call|function_call|tool)\n\\s*(\\{[\\s\\S]*?\\})\\s*\n```")
	for _, match := range markdownToolRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			var jsonObj struct {
				Name       string          `json:"name"`
				Tool       string          `json:"tool"`
				Arguments  json.RawMessage `json:"arguments"`
				Parameters json.RawMessage `json:"parameters"`
			}
			if err := json.Unmarshal([]byte(match[1]), &jsonObj); err == nil {
				toolName := jsonObj.Name
				if toolName == "" {
					toolName = jsonObj.Tool
				}
				toolName = NormalizeToolName(toolName)
				args := jsonObj.Arguments
				if len(args) == 0 {
					args = jsonObj.Parameters
				}
				if toolName != "" {
					if len(args) == 0 {
						args = json.RawMessage("{}")
					}
					randBytes := make([]byte, 4)
					_, _ = rand.Read(randBytes)
					calls = append(calls, ToolCall{
						ID:   "call_" + hex.EncodeToString(randBytes),
						Type: "function",
						Function: FunctionCall{
							Name:      toolName,
							Arguments: args,
						},
					})
				}
			}
		}
	}

	// 5. Generic XML <tool_call> or <invoke name="...">
	genericInvokeRe := regexp.MustCompile(`(?s)<(?:tool_call|function_call|invoke)\s*(?:name="([^"]+)")?>\s*(.*?)\s*</(?:tool_call|function_call|invoke)>`)
	for _, match := range genericInvokeRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			toolName := NormalizeToolName(match[1])
			body := strings.TrimSpace(match[2])
			randBytes := make([]byte, 4)
			_, _ = rand.Read(randBytes)
			callID := "call_" + hex.EncodeToString(randBytes)

			if toolName == "" {
				var jsonObj struct {
					Name       string          `json:"name"`
					Tool       string          `json:"tool"`
					Arguments  json.RawMessage `json:"arguments"`
					Parameters json.RawMessage `json:"parameters"`
				}
				if err := json.Unmarshal([]byte(body), &jsonObj); err == nil {
					tName := jsonObj.Name
					if tName == "" {
						tName = jsonObj.Tool
					}
					tName = NormalizeToolName(tName)
					args := jsonObj.Arguments
					if len(args) == 0 {
						args = jsonObj.Parameters
					}
					if tName != "" {
						if len(args) == 0 {
							args = json.RawMessage("{}")
						}
						calls = append(calls, ToolCall{
							ID:   callID,
							Type: "function",
							Function: FunctionCall{
								Name:      tName,
								Arguments: args,
							},
						})
					}
				}
			} else {
				var argsRaw json.RawMessage = []byte(body)
				if !strings.HasPrefix(body, "{") {
					argsMap := map[string]string{"input": body}
					b, _ := json.Marshal(argsMap)
					argsRaw = b
				}
				calls = append(calls, ToolCall{
					ID:   callID,
					Type: "function",
					Function: FunctionCall{
						Name:      toolName,
						Arguments: argsRaw,
					},
				})
			}
		}
	}

	// Clean all DSML / tool_call / invoke / markdown blocks from content
	cleanBlockRe := regexp.MustCompile(`(?s)<[|｜]{1,2}DSML[|｜]{1,2}tool_calls>.*?</[|｜]{1,2}DSML[|｜]{1,2}tool_calls>|<[|｜]{1,2}tool call begin[|｜]{1,2}>.*?<[|｜]{1,2}tool call end[|｜]{1,2}>|<[|｜]{1,2}tool calls[|｜]{1,2}>.*?</[|｜]{1,2}tool calls[|｜]{1,2}>|<tool_call>.*?</tool_call>|<function_call>.*?</function_call>|<function=[^>]+>.*?</function>|` + "```(?:tool_call|function_call|tool)[\\s\\S]*?```")
	cleaned = cleanBlockRe.ReplaceAllString(cleaned, "")
	cleanSingleTagRe := regexp.MustCompile(`(?s)</?[|｜]{1,2}[^>]*>|</?(?:tool_call|function_call|invoke|parameter)[^>]*>`)
	cleaned = cleanSingleTagRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned, calls
}

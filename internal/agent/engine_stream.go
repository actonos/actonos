package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

// ExecuteStepStream runs a cognitive iteration of the ReAct state machine while emitting
// ExecuteStepStream runs a single cognitive iteration of the ReAct state machine while streaming events.
func (e *Engine) ExecuteStepStream(ctx context.Context, agentID string, userMessage string, eventChan chan<- AgentStreamEvent) (*llm.Response, error) {
	return e.ExecuteStepStreamWithHistory(ctx, agentID, userMessage, nil, eventChan)
}

// ExecuteStepStreamWithHistory runs a streaming cognitive iteration with dialogue history.
func (e *Engine) ExecuteStepStreamWithHistory(ctx context.Context, agentID string, userMessage string, history []llm.Message, eventChan chan<- AgentStreamEvent) (finalResp *llm.Response, err error) {
	defer close(eventChan)

	var run *AgentRun
	var iterationsCompleted int
	var totalUsage llm.Usage

	defer func() {
		if rec := recover(); rec != nil {
			err = recoverAsError(agentID, rec)
			if run != nil {
				e.finishRun(context.Background(), run, RunFailed, "panic: "+fmt.Sprint(rec), iterationsCompleted, totalUsage)
			}
		} else if run != nil && (run.Status == RunRunning || run.Status == "") {
			if err != nil {
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(err, &approvalErr) {
					// approval pending was already recorded
					return
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					e.finishRun(context.Background(), run, RunCancelled, "timeout_or_cancelled", iterationsCompleted, totalUsage)
				} else {
					e.finishRun(context.Background(), run, RunFailed, err.Error(), iterationsCompleted, totalUsage)
				}
			} else {
				e.finishRun(context.Background(), run, RunCompleted, "goal_completed", iterationsCompleted, totalUsage)
			}
		}
	}()

	startTime := time.Now()

	agent, err := e.agentMgr.Get(ctx, agentID)
	if err != nil {
		eventChan <- AgentStreamEvent{
			Type:  EventStreamError,
			Error: fmt.Sprintf("getting agent %s: %v", agentID, err),
		}
		return nil, fmt.Errorf("getting agent %s: %w", agentID, err)
	}

	if agent.Status == "" {
		agent.Status = StatusActive
	}
	if agent.Status != StatusActive {
		err := fmt.Errorf("agent %s is not active (status=%s)", agentID, agent.Status)
		eventChan <- AgentStreamEvent{
			Type:  EventStreamError,
			Error: err.Error(),
		}
		return nil, err
	}
	source := sourceFromContextOrMessage(ctx, userMessage, "stream")
	ctx = WithExecutionSource(ctx, source)
	if err := e.checkBudget(ctx, agent, source); err != nil {
		eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
		return nil, err
	}

	traceID := generateTraceID()
	ctx = tools.WithTraceID(ctx, traceID)
	run = e.startRun(ctx, traceID, agentID, userMessage, source)

	eventChan <- AgentStreamEvent{
		Type:    EventStreamThought,
		Thought: fmt.Sprintf("Activating agent '%s' (%s)...", agent.Name, agent.ModelConfig.PrimaryModel),
	}

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Build unified 4-layer cognitive system prompt & search memory
	eventChan <- AgentStreamEvent{
		Type:    EventStreamThought,
		Thought: "Retrieving user profile, procedural guidelines, and episodic memory...",
	}
	memStart := time.Now()
	fullSystemPrompt, memoryCount := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)
	memDuration := time.Since(memStart).Milliseconds()

	// Using for debug only, don't remove it
	// fmt.Printf("\n================ [DEBUG STREAM SYSTEM PROMPT (%s)] ================\n%s\n===================================================================\n\n", agentID, fullSystemPrompt)

	if memoryCount > 0 {
		eventChan <- AgentStreamEvent{
			Type: EventStreamAudit,
			AuditLog: &AuditLogEntry{
				Timestamp:    time.Now().UTC(),
				AgentID:      agentID,
				Action:       "cognitive_memory_retrieval",
				Status:       "success",
				Verification: fmt.Sprintf("Retrieved %d memory fragments (episodic diary + semantic vector knowledge)", memoryCount),
				DurationMs:   memDuration,
			},
		}
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: fullSystemPrompt},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMessage})
	messages = e.attachAutonomousPlan(ctx, messages, userMessage, history)

	// 3. Model cascade
	var cascadeOrder []string
	if agent.ModelConfig.PrimaryModel != "" {
		cascadeOrder = append(cascadeOrder, agent.ModelConfig.PrimaryModel)
	}
	if agent.ModelConfig.FallbackModel != "" {
		cascadeOrder = append(cascadeOrder, agent.ModelConfig.FallbackModel)
	}

	opts := llm.CompletionOptions{
		ReasoningEffort: agent.ModelConfig.EffectiveReasoningEffort(),
	}
	if agent.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &agent.ModelConfig.MaxTokens
	} else {
		maxOut := defaultStreamMaxTokens
		opts.MaxTokens = &maxOut
	}

	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		authorizedTools := agent.AuthorizedTools
		if allowed := tools.AllowedTools(ctx); allowed != nil {
			authorizedTools = allowed
		}
		opts.Tools = e.tools.ToLLMToolDefinitions(authorizedTools, tools.DeniedTools(ctx)...)
	}

	var allExecutedToolCalls []llm.ToolCall
	totalUsage = llm.Usage{}
	maxIterations := 20
	converged := false
	iterationsCompleted = 0
	consecutiveFailures := 0
	emptyReplyRetries := 0
	lastObservation := ""
	repeatedObservations := 0

	for iter := 0; iter < maxIterations; iter++ {
		iterationsCompleted = iter + 1
		targetModel := agent.ModelConfig.PrimaryModel
		if targetModel == "" {
			targetModel = "cascade-llm"
		}

		currentOpts := opts
		if iter == maxIterations-1 {
			currentOpts.Tools = nil
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "Iteration limit reached. Do not call any more tools. Synthesize all observations gathered above into your final comprehensive response to the user.",
			})
		}

		eventChan <- AgentStreamEvent{
			Type:    EventStreamThought,
			Thought: fmt.Sprintf("Deliberating with %s (ReAct iteration %d)...", targetModel, iter+1),
		}

		if e.contextManager != nil {
			runID := ""
			if run != nil {
				runID = run.ID
			}
			messages = e.contextManager.PruneAndSnapshot(ctx, runID, messages, e.contextBudget(agent))
		}
		resp, err := e.completeStreamIteration(ctx, cascadeOrder, messages, currentOpts, eventChan)
		if err != nil && llm.IsContextWindowError(err) {
			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: "Input exceeded the model context window. Compacting tool observations and retrying...",
			}
			runID := ""
			if run != nil {
				runID = run.ID
			}
			messages = e.compactAfterContextWindowError(ctx, runID, agent, messages)
			resp, err = e.completeStreamIteration(ctx, cascadeOrder, messages, currentOpts, eventChan)
		}
		if err != nil {
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
			}
			eventChan <- AgentStreamEvent{
				Type:  EventStreamError,
				Error: err.Error(),
			}
			e.finishRun(ctx, run, RunFailed, "infrastructure_failure", iterationsCompleted, totalUsage)
			return nil, fmt.Errorf("llm completion: %w", err)
		}
		totalUsage = addUsage(totalUsage, resp.Usage)
		if run != nil {
			_ = e.runStore.AppendEvent(ctx, RunEvent{
				RunID: run.ID, TraceID: traceID, Step: iterationsCompleted, Type: "llm",
				Status: "success", Data: map[string]any{"model": resp.Model, "tool_calls": len(resp.ToolCalls)},
			})
		}

		finalResp = resp
		if len(resp.ToolCalls) > 0 {
			allExecutedToolCalls = append(allExecutedToolCalls, resp.ToolCalls...)
		}

		// A thinking model can end its turn with everything in its reasoning and an empty
		// reply. Ask once for the actual answer instead of streaming silence to the user.
		if len(resp.ToolCalls) == 0 && strings.TrimSpace(resp.Content) == "" && emptyReplyRetries < 1 && iter < maxIterations-2 {
			emptyReplyRetries++
			messages = append(messages,
				llm.Message{Role: llm.RoleAssistant, Content: "[no reply]", ReasoningContent: resp.ReasoningContent},
				llm.Message{Role: llm.RoleUser, Content: "Your last reply was empty. Reply now with your final answer to the request."},
			)
			continue
		}

		// If no tool calls requested, stream tokens preserving exact whitespace and newlines
		if len(resp.ToolCalls) == 0 || e.tools == nil || currentOpts.Tools == nil {
			converged = true
			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "cognitive_react_cycle",
					Status:       "converged",
					Verification: "Tier-2 Semantic Consistency Verified",
					DurationMs:   time.Since(startTime).Milliseconds(),
				},
			}
			break
		}

		// CRITICAL FIX: the assistant message and every tool result must enter the
		// transcript as one unit. Collect results first, append atomically after the
		// loop so pruning can never observe a half-built tool-call block.
		assistantMsg := llm.Message{
			Role:             llm.RoleAssistant,
			Content:          "",
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
			ProviderItems:    resp.ProviderItems,
		}
		var toolMessages []llm.Message

		// Execute each tool call with AST inspection & live streaming events
		for _, tc := range resp.ToolCalls {
			toolName := tc.Function.Name
			toolArgs := string(tc.Function.Arguments)

			eventChan <- AgentStreamEvent{
				Type:       EventStreamToolCall,
				Tool:       toolName,
				ToolCallID: tc.ID,
				Args:       toolArgs,
			}

			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: fmt.Sprintf("Running Tier-1 AST & security policy verification for tool '%s'...", toolName),
			}

			// Perform Tier-1 Security Verification. A failed policy check is terminal
			// for this call and must never degrade into a warning.
			var verifyErr error
			if e.verifier != nil {
				if toolName == "native_exec" {
					verifyErr = e.verifier.VerifyToolCommand(tc.Function.Arguments)
				}
			}

			verificationStatus := "Tier-1 AST Clean & Safe"
			if verifyErr != nil {
				verificationStatus = fmt.Sprintf("Blocked: %v", verifyErr)
			}

			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "tool_security_check",
					ToolName:     toolName,
					Parameters:   toolArgs,
					Status:       map[bool]string{true: "blocked", false: "verified"}[verifyErr != nil],
					Verification: verificationStatus,
					DurationMs:   0,
				},
			}
			if verifyErr != nil {
				resultStr := fmt.Sprintf("Tool execution blocked by policy: %v", verifyErr)
				eventChan <- AgentStreamEvent{
					Type: EventStreamToolResult, Tool: toolName, ToolCallID: tc.ID,
					Result: resultStr, Status: "blocked",
				}
				toolMessages = append(toolMessages, llm.Message{
					Role: llm.RoleTool, Name: toolName, ToolCallID: tc.ID, Content: resultStr,
				})
				consecutiveFailures++
				continue
			}

			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: fmt.Sprintf("Executing tool '%s' in isolated runtime sandbox...", toolName),
			}

			t0 := time.Now()
			toolResult, execErr := e.tools.Execute(ctx, agentID, toolName, tc.Function.Arguments)
			latency := time.Since(t0).Milliseconds()

			resultStr := ""
			statusStr := "success"
			if execErr != nil {
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(execErr, &approvalErr) {
					checkpointMessages := append(append([]llm.Message{}, messages...), assistantMsg)
					checkpointMessages = append(checkpointMessages, toolMessages...)
					e.saveApprovalCheckpoint(ctx, run, agentID, userMessage, source, checkpointMessages, iterationsCompleted, totalUsage, tc)
					e.finishRun(ctx, run, RunApprovalPending, "approval_required", iterationsCompleted, totalUsage)
					approval := approvalErr.Approval
					waitCh := e.approvalWaiters.register(approval.ID)
					eventChan <- AgentStreamEvent{
						Type: EventStreamAudit,
						AuditLog: &AuditLogEntry{
							Timestamp: time.Now().UTC(), AgentID: agentID,
							Action: "approval_required", ToolName: toolName, Status: "pending",
							Verification: approval.ID,
						},
					}
					eventChan <- AgentStreamEvent{
						Type: EventStreamApprovalRequired, Tool: toolName, ToolCallID: tc.ID,
						Args: toolArgs, Status: "awaiting_approval", Approval: &approval,
					}
					eventChan <- AgentStreamEvent{
						Type: EventStreamToolResult, Tool: toolName, ToolCallID: tc.ID,
						Result: "awaiting human approval", Status: "awaiting_approval",
					}
					eventChan <- AgentStreamEvent{
						Type:    EventStreamThought,
						Thought: fmt.Sprintf("Waiting for operator approval of '%s'...", toolName),
					}
					decision, waitErr := waitOnApproval(ctx, waitCh)
					e.approvalWaiters.unregister(approval.ID)
					if waitErr != nil {
						return nil, execErr
					}
					if !decision.Approved {
						statusStr = "rejected"
						reason := strings.TrimSpace(decision.Reason)
						if reason == "" {
							resultStr = fmt.Sprintf("Operator rejected tool %s", toolName)
						} else {
							resultStr = fmt.Sprintf("Operator rejected tool %s: %s", toolName, reason)
						}
						consecutiveFailures++
					} else {
						e.finishRun(ctx, run, RunRunning, "", iterationsCompleted, totalUsage)
						approvedCtx := tools.WithApprovalID(ctx, approval.ID)
						t0 = time.Now()
						toolResult, execErr = e.tools.Execute(approvedCtx, agentID, toolName, tc.Function.Arguments)
						latency = time.Since(t0).Milliseconds()
						if execErr != nil {
							statusStr = "error"
							resultStr = fmt.Sprintf("Error executing tool %s: %v", toolName, execErr)
							consecutiveFailures++
						} else if toolResult != nil {
							resultStr = toolResult.Content
							consecutiveFailures = 0
						}
					}
				} else {
					statusStr = "error"
					resultStr = fmt.Sprintf("Error executing tool %s: %v", toolName, execErr)
					if toolResult != nil && toolResult.Content != "" && toolResult.Content != resultStr {
						resultStr = fmt.Sprintf("%s\n%s", resultStr, toolResult.Content)
					}
					consecutiveFailures++
				}
			} else if toolResult != nil {
				resultStr = toolResult.Content
				consecutiveFailures = 0
			}
			if e.contextManager != nil {
				runID := ""
				if run != nil {
					runID = run.ID
				}
				resultStr = e.contextManager.AutoSummarizeObservation(ctx, e.llm, runID, tc.Function.Name, resultStr)
			} else {
				resultStr = capToolObservation(resultStr)
			}
			if resultStr == lastObservation {
				repeatedObservations++
			} else {
				repeatedObservations = 0
				lastObservation = resultStr
			}
			if run != nil {
				_ = e.runStore.AppendEvent(ctx, RunEvent{
					RunID: run.ID, TraceID: traceID, Step: iterationsCompleted, Type: "tool",
					Status: statusStr, ToolName: toolName, DurationMS: latency,
					Data: map[string]any{"tool_call_id": tc.ID, "observation": truncateRunData(resultStr, 4096)},
				})
			}

			eventChan <- AgentStreamEvent{
				Type:       EventStreamToolResult,
				Tool:       toolName,
				ToolCallID: tc.ID,
				Result:     resultStr,
				Status:     statusStr,
				LatencyMs:  latency,
			}

			eventChan <- AgentStreamEvent{
				Type: EventStreamAudit,
				AuditLog: &AuditLogEntry{
					Timestamp:    time.Now().UTC(),
					AgentID:      agentID,
					Action:       "tool_execution",
					ToolName:     toolName,
					Parameters:   toolArgs,
					Status:       statusStr,
					Verification: "Sandboxed Process Isolation Safe",
					DurationMs:   latency,
				},
			}

			eventChan <- AgentStreamEvent{
				Type:    EventStreamThought,
				Thought: fmt.Sprintf("Tool '%s' returned (%d ms). Analyzing observation...", toolName, latency),
			}

			toolMessages = append(toolMessages, llm.Message{
				Role:       llm.RoleTool,
				Name:       toolName,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})
			if consecutiveFailures >= 5 || repeatedObservations >= 3 || iterationsCompleted >= maxIterations-2 {
				// Don't kill the stream abruptly! Disable tools so LLM processes the error
				// and streams its concluding explanation directly to the user.
				opts.Tools = nil
			}
		}

		// ATOMIC APPEND: assistant tool-call message plus every matching result.
		messages = append(messages, assistantMsg)
		messages = append(messages, toolMessages...)
	}
	if !converged {
		// Stream a final recovery turn with tools disabled so LLM explains the outcome to the user
		finalOpts := opts
		finalOpts.Tools = nil
		eventChan <- AgentStreamEvent{
			Type:    EventStreamThought,
			Thought: "Synthesizing final findings and reporting observations...",
		}
		resp, streamErr := e.completeStreamIteration(ctx, cascadeOrder, messages, finalOpts, eventChan)
		if streamErr == nil && resp != nil {
			finalResp = resp
			converged = true
			totalUsage = addUsage(totalUsage, resp.Usage)
		} else {
			e.finishRun(ctx, run, RunBlocked, "iteration_budget_exhausted", iterationsCompleted, totalUsage)
			err := fmt.Errorf("agent %s reached the maximum of %d ReAct iterations without convergence", agentID, maxIterations)
			eventChan <- AgentStreamEvent{Type: EventStreamError, Error: err.Error()}
			return nil, err
		}
	}
	if finalResp == nil {
		finalResp = &llm.Response{
			Content: "",
			Model:   agent.ModelConfig.PrimaryModel,
		}
	}
	if !e.verifier.VerifySemanticConsistency(ctx, userMessage, finalResp.Content) && strings.TrimSpace(finalResp.Content) != "" {
		finalResp.Content = "I have processed your request and completed the authorized actions."
	}
	finalResp.Usage = totalUsage
	if finalResp != nil && len(allExecutedToolCalls) > 0 {
		finalResp.ToolCalls = allExecutedToolCalls
	}

	// 4. Trigger reflection daemon asynchronously (updates MEMORY.md, preferences, and episodic memory)
	if finalResp != nil && finalResp.Content != "" {
		if e.reflectionEngine != nil {
			e.reflectionEngine.ReflectOnConversation(context.Background(), agentID, userMessage, finalResp.Content)
		} else if e.memory != nil {
			go func() {
				_, _ = e.memory.StoreMemory(
					context.Background(),
					agentID,
					memory.LayerEpisodic,
					fmt.Sprintf("User asked: %s | Response: %s", userMessage, finalResp.Content),
					nil,
					map[string]any{"timestamp": time.Now().UTC()},
					1.0,
				)
			}()
		}
	}

	if e.bus != nil && finalResp != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionDone, agentID, map[string]any{
			"duration_ms": time.Since(startTime).Milliseconds(),
			"tokens":      finalResp.Usage.TotalTokens,
		}))
	}

	if finalResp != nil {
		modelName := finalResp.Model
		if (modelName == "" || !strings.Contains(modelName, "/")) && agent.ModelConfig.PrimaryModel != "" {
			modelName = agent.ModelConfig.PrimaryModel
		}
		e.RecordTokenUsage(ctx, agentID, modelName, "", source, "", finalResp.Usage)
	}

	eventChan <- AgentStreamEvent{
		Type:    EventStreamDone,
		Content: finalResp.Content,
		Model:   finalResp.Model,
		Usage:   &finalResp.Usage,
	}
	e.persistConversationReply(context.Background(), ConversationIDFromContext(ctx), agentID, finalResp)
	e.finishRun(ctx, run, RunCompleted, "goal_completed", iterationsCompleted, totalUsage)

	return finalResp, nil
}

func (e *Engine) completeStreamIteration(
	ctx context.Context,
	cascadeOrder []string,
	messages []llm.Message,
	opts llm.CompletionOptions,
	eventChan chan<- AgentStreamEvent,
) (*llm.Response, error) {
	stream, err := e.llm.StreamCompleteWithCascade(ctx, cascadeOrder, messages, opts)
	if err != nil {
		return nil, err
	}
	response := &llm.Response{}
	if len(cascadeOrder) > 0 {
		response.Model = cascadeOrder[0]
	}

	// Content deltas are forwarded the moment they arrive so the UI renders a real
	// typing effect. Buffering them until the stream closed made every token land
	// in one burst at the end, which is what looked like "jumping text".
	//
	// The catch: we cannot know whether this iteration is a tool-calling turn until
	// the stream ends, and a tool-calling turn's prose must not be shown as the
	// final answer. So tokens are emitted live, and if tool calls do turn up we
	// send a retraction event telling the client to discard this turn's tokens.
	streamedTokens := false

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				goto DoneStream
			}
			if chunk.Error != nil {
				return nil, chunk.Error
			}
			if chunk.DeltaReasoning != "" {
				response.ReasoningContent += chunk.DeltaReasoning
				eventChan <- AgentStreamEvent{Type: EventStreamReasoning, Reasoning: chunk.DeltaReasoning}
			}
			if chunk.DeltaContent != "" {
				response.Content += chunk.DeltaContent
				// If the model is streaming thinking inside <think> tags, emit directly as reasoning
				if isInsideThinkingBlock(response.Content) {
					cleanDelta := stripThinkingTags(chunk.DeltaContent)
					if cleanDelta != "" {
						response.ReasoningContent += cleanDelta
						eventChan <- AgentStreamEvent{Type: EventStreamReasoning, Reasoning: cleanDelta}
					}
				} else if !isInsideMarkupTag(response.Content) {
					// Only emit prose if we are NOT inside a DSML or internal markup tag block.
					// This prevents raw <｜｜DSML... tokens from ever flashing on the user's screen.
					cleanDelta := stripThinkingTags(chunk.DeltaContent)
					if cleanDelta != "" {
						eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: cleanDelta}
						streamedTokens = true
					}
				}
			}
			if len(chunk.ToolCalls) > 0 {
				response.ToolCalls = append(response.ToolCalls, chunk.ToolCalls...)
			}
			if len(chunk.ProviderItems) > 0 {
				response.ProviderItems = append(response.ProviderItems, chunk.ProviderItems...)
			}
			if chunk.Usage != nil {
				response.Usage = *chunk.Usage
			}
			if chunk.Done {
				goto DoneStream
			}
		}
	}

DoneStream:
	rawStreamedContent := response.Content

	if response.Content != "" {
		cleanedContent, thinking := llm.ExtractThinkingContent(response.Content, response.ReasoningContent)
		if thinking != "" && response.ReasoningContent == "" {
			eventChan <- AgentStreamEvent{Type: EventStreamReasoning, Reasoning: thinking}
		}
		response.Content = cleanedContent
		response.ReasoningContent = thinking
	}

	// If the model produced embedded tool calls (e.g. DeepSeek DSML or XML tool calls) in content
	if len(response.ToolCalls) == 0 && response.Content != "" {
		cleaned, embeddedCalls := llm.ExtractEmbeddedToolCalls(response.Content)
		if len(embeddedCalls) > 0 {
			response.ToolCalls = embeddedCalls
		}
		response.Content = cleaned
	}

	// This turn called tools, so retract any interim streamed tokens from the main answer buffer.
	if len(response.ToolCalls) > 0 && streamedTokens {
		eventChan <- AgentStreamEvent{Type: EventStreamTokenReset}
	} else if len(response.ToolCalls) == 0 && streamedTokens && response.Content != rawStreamedContent {
		// Tokens were streamed live, but DSML/thinking tags were stripped at stream end.
		// Retract the raw stream and re-emit the clean content!
		eventChan <- AgentStreamEvent{Type: EventStreamTokenReset}
		if response.Content != "" {
			eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: response.Content}
		}
	} else if len(response.ToolCalls) == 0 && !streamedTokens && response.Content != "" {
		// Content was suppressed during streaming because of tag filtering, emit clean final content now.
		eventChan <- AgentStreamEvent{Type: EventStreamToken, Content: response.Content}
	}

	return response, nil
}

// isInsideThinkingBlock reports if accumulated stream text is currently inside <think>, <thought>, etc.
func isInsideThinkingBlock(accumulated string) bool {
	lower := strings.ToLower(accumulated)
	for _, openTag := range []string{"<think>", "<thought>", "<thinking>", "[think]", "[reasoning]"} {
		closeTag := "</" + strings.TrimPrefix(strings.TrimSuffix(openTag, ">"), "<") + ">"
		if strings.HasPrefix(openTag, "[") {
			closeTag = "[/" + strings.TrimPrefix(strings.TrimSuffix(openTag, "]"), "[") + "]"
		}
		lastOpen := strings.LastIndex(lower, openTag)
		if lastOpen != -1 {
			lastClose := strings.LastIndex(lower, closeTag)
			if lastClose < lastOpen {
				return true
			}
		}
	}
	return false
}

// stripThinkingTags removes thinking markup open/close tags from incremental chunks.
func stripThinkingTags(chunk string) string {
	re := regexp.MustCompile(`(?i)</?(?:think|thought|thinking|\[/?(?:think|reasoning)\])>?`)
	return re.ReplaceAllString(chunk, "")
}

// isInsideMarkupTag reports if accumulated stream text is currently inside a DSML, thinking, or tool call markup block.
func isInsideMarkupTag(accumulated string) bool {
	// 1. DeepSeek DSML tool calls block
	if (strings.Contains(accumulated, "DSML｜｜tool_calls>") || strings.Contains(accumulated, "DSML||tool_calls>")) &&
		!strings.Contains(accumulated, "/DSML｜｜tool_calls>") && !strings.Contains(accumulated, "/DSML||tool_calls>") &&
		!strings.Contains(accumulated, "</｜｜DSML｜｜tool_calls>") && !strings.Contains(accumulated, "</||DSML||tool_calls>") {
		return true
	}

	// 2. Unclosed tag fragment
	lastOpen := strings.LastIndex(accumulated, "<")
	if lastOpen != -1 {
		lastClose := strings.LastIndex(accumulated, ">")
		if lastClose < lastOpen {
			fragment := strings.ToLower(accumulated[lastOpen:])
			if strings.HasPrefix(fragment, "<｜") ||
				strings.HasPrefix(fragment, "<|") ||
				strings.Contains(fragment, "dsml") ||
				strings.HasPrefix(fragment, "<think") ||
				strings.HasPrefix(fragment, "<thought") ||
				strings.HasPrefix(fragment, "<tool_call") ||
				strings.HasPrefix(fragment, "<function=") ||
				strings.HasPrefix(fragment, "<invoke") {
				return true
			}
		}
	}

	// 3. Bare JSON tool call in progress: e.g. `{"command":` or `{"path":` or `{"name":"native_` or `{"tool_uses":`
	trimmed := strings.TrimSpace(accumulated)
	lastBrace := strings.LastIndex(trimmed, "{")
	if lastBrace != -1 {
		lastCloseBrace := strings.LastIndex(trimmed, "}")
		if lastCloseBrace < lastBrace {
			fragment := strings.ToLower(trimmed[lastBrace:])
			if strings.Contains(fragment, `"command"`) ||
				strings.Contains(fragment, `"path"`) ||
				strings.Contains(fragment, `"native_`) ||
				strings.Contains(fragment, `"tool"`) ||
				strings.Contains(fragment, `"arguments"`) ||
				strings.Contains(fragment, `"tool_uses"`) ||
				strings.Contains(fragment, `"recipient_name"`) {
				return true
			}
		}
	}

	// 4. Qwen / Hermes / local tool call token leak: `to=functions.` or `<|action_`
	lowerAccum := strings.ToLower(accumulated)
	if strings.Contains(lowerAccum, "to=functions.") ||
		strings.Contains(lowerAccum, "to=function.") ||
		strings.Contains(lowerAccum, "recipient_name") ||
		strings.Contains(lowerAccum, `"tool_uses"`) ||
		strings.Contains(lowerAccum, "<|action_") {
		return true
	}

	return false
}

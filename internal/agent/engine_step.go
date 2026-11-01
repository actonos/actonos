package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

// ExecuteStepWithHistory runs a cognitive iteration of the ReAct state machine with short-term dialogue history.
func (e *Engine) ExecuteStepWithHistory(ctx context.Context, agentID string, userMessage string, history []llm.Message) (finalResp *llm.Response, err error) {
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
	ctx, cancelTurn := resolveTurnTimeout(ctx, DefaultTurnTimeout)
	defer cancelTurn()

	agent, err := e.agentMgr.Get(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", agentID, err)
	}

	if agent.Status == "" {
		agent.Status = StatusActive
	}
	if agent.Status != StatusActive {
		return nil, fmt.Errorf("agent %s is not active (status=%s)", agentID, agent.Status)
	}
	ctx = e.withSkillCatalog(ctx, agent)
	source := sourceFromContextOrMessage(ctx, userMessage, "chat")
	if err := e.checkBudget(ctx, agent, source); err != nil {
		return nil, err
	}
	maxConc := agent.DelegationScope.MaxConcurrentRuns
	if maxConc <= 0 {
		maxConc = DefaultMaxConcurrentRuns
	}
	if e.inFlight != nil && e.inFlight.count(agentID) >= maxConc {
		return nil, fmt.Errorf("%w: agent %s already has %d running turns", ErrConcurrentRunQuota, agentID, maxConc)
	}

	traceID := generateTraceID()
	ctx = tools.WithTraceID(ctx, traceID)
	run = e.startRun(ctx, traceID, agentID, userMessage, source)
	runID := traceID
	if run != nil {
		runID = run.ID
	}
	if e.inFlight != nil {
		_ = e.inFlight.track(agentID, runID, cancelTurn)
		defer e.inFlight.untrack(agentID, runID)
	}

	if e.bus != nil {
		e.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, agentID, map[string]string{
			"message": userMessage,
		}))
	}

	// 1. Build unified cognitive system prompt
	fullSystemPrompt, _ := e.buildCognitivePrompt(ctx, agentID, agent, userMessage)
	// fmt.Printf("\n================ [DEBUG SYSTEM PROMPT (%s)] ================\n%s\n============================================================\n\n", agentID, fullSystemPrompt)

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
	defaultMaxTokens := 32768
	if agent.ModelConfig.MaxTokens > 0 {
		opts.MaxTokens = &agent.ModelConfig.MaxTokens
	} else {
		opts.MaxTokens = &defaultMaxTokens
	}

	// 3. Attach authorized tools if registry available
	if e.tools != nil && len(agent.AuthorizedTools) > 0 {
		authorizedTools := agent.AuthorizedTools
		if allowed := tools.AllowedTools(ctx); allowed != nil {
			authorizedTools = allowed
		}
		opts.Tools = e.tools.ToLLMToolDefinitions(authorizedTools, tools.DeniedTools(ctx)...)
	}
	startTime := time.Now()
	var allExecutedToolCalls []llm.ToolCall
	totalUsage = llm.Usage{}
	maxIterations := 20
	consecutiveFailures := 0
	lastObservation := ""
	repeatedObservations := 0
	converged := false
	iterationsCompleted = 0

	for iter := 0; iter < maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			e.finishRun(ctx, run, RunCancelled, "context_cancelled", iterationsCompleted, totalUsage)
			return nil, fmt.Errorf("%w: %v", ErrRunCancelled, err)
		}
		iterationsCompleted = iter + 1
		targetModel := agent.ModelConfig.PrimaryModel
		if targetModel == "" {
			targetModel = "cascade-llm"
		}

		currentOpts := opts
		// On the final iteration, force convergence by omitting tools
		if iter == maxIterations-1 {
			currentOpts.Tools = nil
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "Iteration limit reached. Do not call any more tools. Synthesize all observations gathered above into your final comprehensive response to the user.",
			})
		}

		// CRITICAL FIX: Prune context BEFORE LLM call to ensure we send clean, complete message sequences
		if e.contextManager != nil {
			runID := ""
			if run != nil {
				runID = run.ID
			}
			messages = e.contextManager.PruneAndSnapshot(ctx, runID, messages, e.contextBudget(agent))
		}

		resp, err := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, currentOpts)
		if err != nil && llm.IsContextWindowError(err) {
			runID := ""
			if run != nil {
				runID = run.ID
			}
			messages = e.compactAfterContextWindowError(ctx, runID, agent, messages)
			resp, err = e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, currentOpts)
		}
		if err != nil {
			if e.bus != nil {
				e.bus.Publish(bus.NewEvent(bus.EventAgentActionFailed, agentID, err.Error()))
			}
			e.finishRun(ctx, run, RunFailed, "infrastructure_failure", iter+1, totalUsage)
			return nil, fmt.Errorf("llm completion: %w", err)
		}
		totalUsage = addUsage(totalUsage, resp.Usage)
		if run != nil {
			_ = e.runStore.AppendEvent(ctx, RunEvent{
				RunID: run.ID, TraceID: traceID, Step: iter + 1, Type: "llm",
				Status: "success", Data: map[string]any{"model": resp.Model, "tool_calls": len(resp.ToolCalls)},
			})
		}

		finalResp = resp
		if len(resp.ToolCalls) > 0 {
			allExecutedToolCalls = append(allExecutedToolCalls, resp.ToolCalls...)
		}
		if resp.Content != "" {
			cleanedContent, thinking := llm.ExtractThinkingContent(resp.Content, resp.ReasoningContent)
			resp.Content = cleanedContent
			resp.ReasoningContent = thinking
		}

		if len(resp.ToolCalls) == 0 && resp.Content != "" {
			cleaned, embeddedCalls := llm.ExtractEmbeddedToolCalls(resp.Content)
			if len(embeddedCalls) > 0 && currentOpts.Tools != nil {
				resp.ToolCalls = embeddedCalls
			}
			resp.Content = cleaned
		}

		finalResp = resp

		// If no tool calls requested, we reached the final response
		if len(resp.ToolCalls) == 0 || e.tools == nil || currentOpts.Tools == nil {
			converged = true
			break
		}

		// CRITICAL FIX: Build assistant message + ALL tool results ATOMICALLY to prevent race conditions
		// Collect all tool results first
		var toolMessages []llm.Message

		for _, tc := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				e.finishRun(ctx, run, RunCancelled, "context_cancelled", iter+1, totalUsage)
				return nil, fmt.Errorf("%w: %v", ErrRunCancelled, err)
			}
			toolResult, execErr := e.verifyAndExecuteTool(ctx, agentID, tc.Function.Name, tc.Function.Arguments)
			resultStr := ""
			if execErr != nil {
				resultStr = fmt.Sprintf("Error executing tool %s: %v", tc.Function.Name, execErr)
				if toolResult != nil && toolResult.Content != "" && toolResult.Content != resultStr {
					resultStr = fmt.Sprintf("%s\n%s", resultStr, toolResult.Content)
				}
				consecutiveFailures++
				var approvalErr *tools.ApprovalRequiredError
				if errors.As(execErr, &approvalErr) {
					// Persist the in-flight block (assistant + results already produced)
					// so the resumed transcript stays well-formed.
					checkpointMessages := append(append([]llm.Message{}, messages...), llm.Message{
						Role:             llm.RoleAssistant,
						Content:          "",
						ReasoningContent: resp.ReasoningContent,
						ToolCalls:        resp.ToolCalls,
						ProviderItems:    resp.ProviderItems,
					})
					checkpointMessages = append(checkpointMessages, toolMessages...)
					e.saveApprovalCheckpoint(ctx, run, agentID, userMessage, source, checkpointMessages, iter+1, totalUsage, tc)
					e.finishRun(ctx, run, RunApprovalPending, "approval_required", iter+1, totalUsage)
					return nil, execErr
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
				status := "success"
				if execErr != nil {
					status = "error"
				}
				_ = e.runStore.AppendEvent(ctx, RunEvent{
					RunID: run.ID, TraceID: traceID, Step: iter + 1, Type: "tool",
					Status: status, ToolName: tc.Function.Name,
					Data: map[string]any{"tool_call_id": tc.ID, "observation": truncateRunData(resultStr, 4096)},
				})
			}

			toolMessages = append(toolMessages, llm.Message{
				Role:       llm.RoleTool,
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    resultStr,
			})

			if consecutiveFailures >= 5 || repeatedObservations >= 3 || iter >= maxIterations-2 {
				// Don't cut off abruptly! Disable further tool calls so LLM processes the error
				// observations and synthesizes a direct diagnostic answer to the user in the next iteration.
				opts.Tools = nil
			}
		}

		// ATOMIC APPEND: assistant message + all tool results together
		// This prevents context pruning from splitting the assistant+tool sequence
		messages = append(messages, llm.Message{
			Role:             llm.RoleAssistant,
			Content:          "",
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
			ProviderItems:    resp.ProviderItems,
		})
		messages = append(messages, toolMessages...)
	}
	if !converged {
		// Attempt a final recovery turn with tools disabled so LLM explains the errors to the user
		finalOpts := opts
		finalOpts.Tools = nil
		lastResp, genErr := e.llm.CompleteWithCascade(ctx, cascadeOrder, messages, finalOpts)
		if genErr == nil && lastResp != nil && lastResp.Content != "" {
			finalResp = lastResp
			converged = true
			totalUsage = addUsage(totalUsage, lastResp.Usage)
		} else {
			e.finishRun(ctx, run, RunBlocked, "iteration_budget_exhausted", maxIterations, totalUsage)
			return nil, fmt.Errorf("agent %s reached the maximum of %d ReAct iterations without convergence", agentID, maxIterations)
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
	e.finishRun(ctx, run, RunCompleted, "goal_completed", iterationsCompleted, totalUsage)

	return finalResp, nil
}

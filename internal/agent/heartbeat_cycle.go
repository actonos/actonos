package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

// checkCycle runs the autonomous cognitive heartbeat iteration. manual is
// true only for operator-initiated pulses (TriggerManualPulse); it bypasses
// the trigger cooldown and active-hours window.
func (h *HeartbeatDaemon) checkCycle(ctx context.Context, manual bool) (run *HeartbeatRun) {
	h.executionMu.Lock()
	defer h.executionMu.Unlock()
	defer func() {
		if rec := recover(); rec != nil {
			run = &HeartbeatRun{
				AgentID:    "agent_system_core",
				ExecutedAt: time.Now().UTC(),
				Status:     "error",
				Summary:    fmt.Sprintf("heartbeat panic recovered: %v", rec),
			}
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			run.ID = "hb_" + hex.EncodeToString(b)
			h.recordRun(*run)
			slog.Error("heartbeat cycle panic recovered", "recover", rec)
		}
	}()

	now := time.Now().UTC()
	h.mu.Lock()
	if !manual {
		if !h.lastRun.IsZero() && now.Sub(h.lastRun) < minTriggerGap {
			h.mu.Unlock()
			slog.Debug("heartbeat cycle skipped: trigger cooldown active", "since_last_run", now.Sub(h.lastRun).String())
			return h.recordSkip("agent_system_core", "cooldown")
		}
		if !h.withinActiveHoursLocked(now) {
			h.mu.Unlock()
			slog.Debug("heartbeat cycle skipped: outside configured active hours")
			return h.recordSkip("agent_system_core", "active_hours")
		}
	}
	h.lastRun = now
	h.mu.Unlock()
	ctx, cancelTurn := resolveTurnTimeout(ctx, HeartbeatTurnTimeout)
	defer cancelTurn()
	ctx = WithExecutionSource(ctx, "heartbeat")

	primaryAgentID := "agent_system_core"

	// If no real LLM provider is configured (only local-stub or empty), skip autonomous execution to avoid spamming alerts.
	if h.engine == nil || !h.engine.HasConfiguredLLM() {
		slog.Debug("autonomous heartbeat skipped: no real LLM provider configured")
		run := &HeartbeatRun{
			AgentID:    primaryAgentID,
			ExecutedAt: time.Now().UTC(),
			Status:     "ok",
			Summary:    "Heartbeat nominal (waiting for LLM provider configuration).",
		}
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		run.ID = "hb_" + hex.EncodeToString(b)
		h.recordRun(*run)
		return run
	}

	// 1. Read standing directives from the SQLite-backed task manager.
	standingDirectives := ""
	if h.taskMgr != nil {
		if cfg, err := h.taskMgr.GetHeartbeatConfig(ctx); err == nil && cfg != nil {
			standingDirectives = cfg.Directives
		}
	}

	activeTask, humanWait := h.selectRunnableMission(ctx, primaryAgentID)

	run = &HeartbeatRun{
		AgentID:    primaryAgentID,
		ExecutedAt: time.Now().UTC(),
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	run.ID = "hb_" + hex.EncodeToString(b)

	if h.engine != nil {
		_, _ = h.engine.ReclaimStaleRuns(ctx, 10*time.Minute)
	}

	// Cheap classifier: only spend tokens when there is runnable work.
	// Blocked / approval-paused missions stay visible and must not burn a
	// completion call inventing a retry — even if standing directives exist.
	if activeTask == nil {
		if humanWait != "" {
			run.Status = "skipped"
			run.Summary = humanWait
			h.recordRun(*run)
			return run
		}
		if !hasActionableHeartbeatDirectives(standingDirectives) {
			h.mu.RLock()
			pe := h.proactiveEngine
			h.mu.RUnlock()
			if pe != nil {
				_, _ = pe.Scan(ctx)
			}

			run.Status = "ok"
			run.Summary = "System nominal. Zero tasks pending. No actionable heartbeat directives."
			h.recordRun(*run)
			h.checkCustomAgentPulses(ctx, now, manual)
			return run
		}
	}

	// CASE A: Active Task Execution with Session Resume Memory
	if activeTask != nil {
		assignedAgent := activeTask.AssignedAgentID
		if assignedAgent == "" || assignedAgent == "auto" {
			assignedAgent = primaryAgentID
		}
		run.AgentID = assignedAgent

		// Mark task as in_progress immediately
		if activeTask.Status == "pending" {
			activeTask.Status = "in_progress"
			_ = h.taskMgr.UpdateTask(ctx, *activeTask)
		}

		// Guard against a silent, invisible failure mode: the model can never
		// call the notify tool itself (denied below), so delivery relies
		// entirely on the task's own TargetChannel. If the operator's request
		// clearly asks to notify/send somewhere but the task is configured
		// with TargetChannel=="none", the completed work would otherwise
		// vanish with no visible trace. Warn loudly (once per task) so this
		// misconfiguration is diagnosable instead of silently dropped.
		if activeTask.TargetChannel == "none" && mentionsNotificationIntent(activeTask.Title+" "+activeTask.Description) {
			h.mu.Lock()
			state := h.stallTracker[activeTask.ID+":notify_misconfig"]
			if !state.notified {
				state.notified = true
				h.stallTracker[activeTask.ID+":notify_misconfig"] = state
				h.mu.Unlock()
				slog.Warn("mission directive appears to request notification delivery but TargetChannel is 'none'; the result will not be delivered anywhere",
					"task_id", activeTask.ID, "title", activeTask.Title)
			} else {
				h.mu.Unlock()
			}
		}

		// 1. Resume or create task working session
		var history []llm.Message
		convID := activeTask.SessionID
		if convID == "" {
			convID = fmt.Sprintf("conv_task_%s", activeTask.ID)
			activeTask.SessionID = convID
		}

		if h.sessionMgr != nil {
			realConvID, err := h.sessionMgr.GetOrCreateSession(
				ctx,
				"mission",
				activeTask.ID,
				activeTask.Title,
				activeTask.Description,
				assignedAgent,
			)
			if err == nil && realConvID != "" {
				convID = realConvID
			}

			// Load recent working memory of past steps
			history = h.sessionMgr.LoadRecentHistory(ctx, convID, 8)

			// Record incoming pulse step in session
			userPrompt := fmt.Sprintf(
				"[Heartbeat Task Step]\nTask: %s (Priority: %s, Current Progress: %d%%)\nDirective: %s\nStanding Directives: %s",
				activeTask.Title, activeTask.Priority, activeTask.Progress, activeTask.Description, standingDirectives,
			)
			_ = h.sessionMgr.SaveMessage(ctx, convID, assignedAgent, "user", userPrompt, nil)
		}

		var skills []SkillPromptEntry
		if h.engine != nil {
			skills = h.engine.SkillCatalogForAgent(ctx, assignedAgent)
		}
		workGoal := missionWorkGoal(activeTask.Title, activeTask.Description)

		// Suppress episodic memory for heartbeat task execution to prevent stale
		// memories from deleted tasks from contaminating the current task context.
		taskCtx := context.WithValue(ctx, "task_id", activeTask.ID)
		taskCtx = tools.WithTaskID(taskCtx, activeTask.ID)
		taskCtx = context.WithValue(taskCtx, "suppress_episodic_memory", false)
		taskCtx = context.WithValue(taskCtx, "heartbeat_headless_mode", true)
		taskCtx = WithExecutionSource(taskCtx, "heartbeat")
		taskCtx = WithSkillCatalog(taskCtx, skills)
		if h.cronSched != nil && h.cronSched.CountJobsForAgent(assignedAgent) >= DefaultCronJobsPerAgent {
			taskCtx = tools.WithDeniedTools(taskCtx, "native_cron_schedule")
		}
		resp, execErr := h.engine.ExecuteAutonomousGoal(taskCtx, assignedAgent, workGoal, history)
		h.adoptPersistedPlan(ctx, activeTask)
		if execErr != nil {
			var approvalErr *tools.ApprovalRequiredError
			if errors.As(execErr, &approvalErr) {
				run.Status = "approval_required"
				run.Summary = fmt.Sprintf("Mission '%s' paused: operator approval required for '%s'.", activeTask.Title, approvalErr.Approval.ToolName)
				activeTask.ExecutionLog = fmt.Sprintf("Paused: operator approval required for tool '%s'.", approvalErr.Approval.ToolName)
				h.persistMissionTask(ctx, activeTask)
				h.recordRun(*run)
				return run
			}
			if isTransientExecutionError(execErr) {
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Mission '%s' temporarily paused on rate/token limit: %v (will retry on next pulse)", activeTask.Title, execErr)
				slog.Info("heartbeat task encountered transient error; will retry automatically", "task_id", activeTask.ID, "error", execErr)
				activeTask.ExecutionLog = run.Summary
				h.persistMissionTask(ctx, activeTask)
				h.recordRun(*run)
				return run
			}
			run.Status = "error"
			run.Summary = fmt.Sprintf("Failed executing task '%s': %v", activeTask.Title, execErr)
			slog.Error("heartbeat task execution error", "task_id", activeTask.ID, "error", execErr)
			activeTask.FailCount++
			activeTask.ExecutionLog = run.Summary
			if activeTask.FailCount >= maxFailCyclesBeforeBlock {
				activeTask.Status = "blocked"
				run.Summary += " [blocked after repeated execution errors]"
			}
			h.persistMissionTask(ctx, activeTask)
			h.recordRun(*run)
			return run
		} else if resp != nil {
			run.TokensUsed = resp.Usage.TotalTokens
			content := strings.TrimSpace(resp.Content)

			// Persist step in session history
			if h.sessionMgr != nil {
				_ = h.sessionMgr.SaveMessage(ctx, convID, assignedAgent, "assistant", resp.Content, resp.ToolCalls)
			}

			fullCleaned := cleanFullContent(content)
			shortLog := shortSummary(content, 250)
			planDone := activeTask.Plan != nil && activeTask.Plan.AllStepsCompleted()
			planOpen := activeTask.Plan != nil && len(activeTask.Plan.Steps) > 0 && !planDone
			if activeTask.Plan != nil {
				if pct := activeTask.Plan.ProgressPercent(); pct > activeTask.Progress {
					activeTask.Progress = pct
				}
			}
			// A write only finishes the mission when the DAG has no remaining
			// steps. Prose in any language is never a completion signal.
			deliverableDone := !planOpen && HasDeliverableWrite(resp.ToolCalls)

			// Parse Task status transitions. Empty/canned replies never complete a
			// mission unless the durable DAG is already fully done — progress 100%
			// from completed plan steps is the completion signal, not a stall.
			if IsCannedOrEmptyCompletion(content) && !planDone && !deliverableDone && activeTask.Status != "completed" {
				activeTask.Status = "in_progress"
				activeTask.FailCount++
				activeTask.ExecutionLog = "Empty or canned success ignored; mission remains in progress."
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Mission '%s' produced no actionable output.", activeTask.Title)
				if activeTask.FailCount >= maxFailCyclesBeforeBlock {
					activeTask.Status = "blocked"
					run.Summary += " [blocked after empty completions]"
				}
			} else if planOpen {
				activeTask.Status = "in_progress"
				activeTask.ExecutionLog = shortLog
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Advanced mission '%s' to %d%%. %s", activeTask.Title, activeTask.Progress, shortLog)
			} else if planDone || activeTask.Status == "completed" {
				activeTask.Status = "completed"
				activeTask.Progress = 100
				activeTask.FailCount = 0
				if strings.TrimSpace(shortLog) != "" {
					activeTask.ExecutionLog = shortLog
				}
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Completed mission: '%s'. %s", activeTask.Title, shortLog)
			} else if deliverableDone || (strings.Contains(content, "[TASK_COMPLETED]") && h.engine.verifier.VerifyTaskCompletion(activeTask.Description, content, resp.ToolCalls)) {
				activeTask.Status = "completed"
				activeTask.Progress = 100
				activeTask.FailCount = 0
				if strings.TrimSpace(shortLog) != "" {
					activeTask.ExecutionLog = shortLog
				}
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Completed mission: '%s'. %s", activeTask.Title, shortLog)
			} else if strings.Contains(content, "[TASK_COMPLETED]") {
				activeTask.Status = "in_progress"
				activeTask.ExecutionLog = "Completion claim rejected by deterministic verification. " + shortLog
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Mission '%s' requires additional verification.", activeTask.Title)
			} else if strings.Contains(content, "[TASK_BLOCKED") {
				activeTask.Status = "blocked"
				activeTask.ExecutionLog = shortLog
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Mission '%s' blocked. %s", activeTask.Title, shortLog)
			} else {
				activeTask.Status = "in_progress"
				reProg := regexp.MustCompile(`\[PROGRESS:\s*(\d+)%?\]`)
				match := reProg.FindStringSubmatch(content)
				if len(match) > 1 {
					if pVal, err := strconv.Atoi(match[1]); err == nil && pVal > activeTask.Progress {
						activeTask.Progress = pVal
					}
				}
				activeTask.ExecutionLog = shortLog
				run.Status = "action_taken"
				run.Summary = fmt.Sprintf("Advanced mission '%s' to %d%%. %s", activeTask.Title, activeTask.Progress, shortLog)
			}

			// Stall detection: a task must not silently retry forever if it
			// never actually advances. Completed/blocked tasks clear their
			// stall state (an explicit terminal outcome, not a stall); an
			// in_progress task whose Progress hasn't changed across several
			// consecutive cycles gets a one-time escalation notice appended
			// so the operator can intervene instead of the daemon spending a
			// full model turn every cycle to make zero progress.
			stalled, stallCycles := h.trackTaskStall(activeTask.ID, activeTask.Status, activeTask.Progress)
			activeTask.StalledCycles = stallCycles
			if stalled && activeTask.Status == "in_progress" {
				stallNote := fmt.Sprintf(" [STALL WARNING: no progress advancement across %d consecutive heartbeat cycles — operator review recommended]", stallCycles)
				activeTask.ExecutionLog += stallNote
				activeTask.Status = "blocked"
				run.Summary += stallNote
				slog.Warn("mission task stalled: blocking after unchanged progress", "task_id", activeTask.ID, "stalled_cycles", stallCycles, "progress", activeTask.Progress)
			}

			h.persistMissionTask(ctx, activeTask)

			// Check if tool already notified user directly to prevent double dispatch
			alreadyNotified := false
			for _, tc := range resp.ToolCalls {
				if tc.Function.Name == "native_channel_notify" || tc.Function.Name == "channel_notify" {
					alreadyNotified = true
					break
				}
			}

			isRedundantToolReport := strings.HasPrefix(content, "The notification has been successfully sent") ||
				strings.HasPrefix(content, "Successfully dispatched proactive notification") ||
				strings.HasPrefix(content, "Notification sent")

			// Do not ping the operator between DAG steps; the runtime continues itself.
			skipInterim := activeTask.Status == "in_progress" && activeTask.Plan != nil && activeTask.Plan.HasReadyStep()

			// Proactively notify user with FULL UNTRUNCATED content through event bus if target channel configured
			if h.eventBus != nil && !alreadyNotified && !isRedundantToolReport && !skipInterim && activeTask.TargetChannel != "none" && activeTask.TargetChannel != "" {
				h.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, assignedAgent, map[string]any{
					"type":              "proactive_cron_notification",
					"job_name":          fmt.Sprintf("Mission: %s", activeTask.Title),
					"content":           fullCleaned,
					"target_channel":    activeTask.TargetChannel,
					"target_account_id": activeTask.TargetAccountID,
					"target_recipient":  "",
				}))
			}

			// Immediate follow-up only when there is new work: another pending
			// mission, or a remaining dependency-ready plan step. An in_progress
			// mission with no ready step must not be re-woken to repeat itself.
			if h.taskMgr != nil {
				pendingTasks, _ := h.taskMgr.ListTasks(ctx, "pending", "")
				readyMore := activeTask.Status == "in_progress" && activeTask.Plan != nil && activeTask.Plan.HasReadyStep()
				moreWork := (activeTask.Status == "completed" && len(pendingTasks) > 0) || readyMore
				if moreWork {
					slog.Info("queueing immediate next heartbeat cycle", "task_id", activeTask.ID, "status", activeTask.Status, "pending_count", len(pendingTasks), "ready_more", readyMore)
					go func() {
						time.Sleep(2 * time.Second)
						h.TriggerWakeup()
					}()
				}
			}
		}

		h.recordRun(*run)
		return run
	}

	// CASE B: Routine System Health & Zero-Noise Evaluation
	routineCtx := context.WithValue(ctx, "suppress_episodic_memory", false)
	routineCtx = context.WithValue(routineCtx, "heartbeat_headless_mode", true)
	routineCtx = WithExecutionSource(routineCtx, "heartbeat")
	if h.cronSched != nil && h.cronSched.CountJobsForAgent(primaryAgentID) >= DefaultCronJobsPerAgent {
		routineCtx = tools.WithDeniedTools(routineCtx, "native_cron_schedule")
	}
	backlogSummary := "All previous backlog missions are COMPLETED. There are ZERO pending or in-progress tasks."
	if h.taskMgr != nil {
		completedList, _ := h.taskMgr.ListTasks(ctx, "completed", "")
		if len(completedList) > 0 {
			backlogSummary = fmt.Sprintf("All %d previous backlog missions have been COMPLETED. There are 0 active or pending tasks in backlog.", len(completedList))
		}
	}
	// Prompt structure follows OpenClaw's documented heartbeat contract:
	// https://docs.openclaw.ai/vi/gateway/heartbeat — read/execute standing
	// directives strictly, never infer or repeat old work, and reply with the
	// bare acknowledgement token when nothing needs attention.
	prompt := BuildHeartbeatPulsePrompt(standingDirectives, backlogSummary)

	resp, execErr := h.engine.ExecuteStepWithHistory(routineCtx, primaryAgentID, prompt, nil)
	if execErr != nil {
		var approvalErr *tools.ApprovalRequiredError
		if errors.As(execErr, &approvalErr) {
			run.Status = "approval_required"
			run.Summary = fmt.Sprintf("Standing directive paused: operator approval required for '%s'.", approvalErr.Approval.ToolName)
			h.recordRun(*run)
			return run
		}
		run.Status = "error"
		run.Summary = execErr.Error()
		slog.Warn("heartbeat execution failed", "agent_id", primaryAgentID, "error", execErr)
	} else if resp != nil {
		run.TokensUsed = resp.Usage.TotalTokens
		trimmed := strings.TrimSpace(resp.Content)

		// Persist heartbeat pulse in session history
		if h.sessionMgr != nil {
			hbConvID, _ := h.sessionMgr.GetOrCreateSession(ctx, "system", "heartbeat", "Heartbeat Pulse", "Routine Heartbeat Pulse", primaryAgentID)
			_ = h.sessionMgr.SaveMessage(ctx, hbConvID, primaryAgentID, "user", fmt.Sprintf("[Heartbeat Pulse]\nStanding Directives: %s", standingDirectives), nil)
			_ = h.sessionMgr.SaveMessage(ctx, hbConvID, primaryAgentID, "assistant", resp.Content, resp.ToolCalls)
		}

		h.mu.RLock()
		ackMaxChars := h.ackMaxChars
		h.mu.RUnlock()
		isAck, alertText := classifyHeartbeatResponse(trimmed, ackMaxChars)

		// Weaker/faster models sometimes ignore the HEARTBEAT_OK contract
		// entirely and free-associate a conversational greeting or capability
		// menu instead of executing the standing directive or replying with
		// the ack token. Since the daemon never delivers such output as a
		// legitimate result (the model never called a tool and produced no
		// substantive content), treat it as noise rather than spamming the
		// user with a notification every cycle.
		if !isAck && len(resp.ToolCalls) == 0 && looksLikeIdleChatter(alertText) {
			slog.Warn("heartbeat produced an off-topic idle response instead of executing the directive; suppressing delivery",
				"agent_id", primaryAgentID, "content_preview", shortSummary(alertText, 200))
			isAck = true
		}

		if isAck {
			run.Status = "ok"
			run.Summary = "System nominal. Zero tasks pending. No proactive notification required."
			slog.Debug("heartbeat nominal (zero noise)", "agent_id", primaryAgentID)
		} else {
			run.Status = "action_taken"
			run.Summary = shortSummary(alertText, 250)
			slog.Info("heartbeat performed proactive action", "agent_id", primaryAgentID)

			alreadyNotified := false
			for _, tc := range resp.ToolCalls {
				if tc.Function.Name == "native_channel_notify" || tc.Function.Name == "channel_notify" {
					alreadyNotified = true
					break
				}
			}

			isRedundantToolReport := strings.HasPrefix(alertText, "The notification has been successfully sent") ||
				strings.HasPrefix(alertText, "Successfully dispatched proactive notification") ||
				strings.HasPrefix(alertText, "Notification sent")

			h.mu.RLock()
			targetChannel := h.targetChannel
			targetAccount := h.targetAccount
			h.mu.RUnlock()
			if h.eventBus != nil && !alreadyNotified && !isRedundantToolReport && targetChannel != "none" && targetChannel != "" {
				h.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, primaryAgentID, map[string]any{
					"type":              "proactive_cron_notification",
					"job_name":          "Heartbeat Pulse",
					"content":           cleanFullContent(alertText),
					"target_channel":    targetChannel,
					"target_account_id": targetAccount,
					"target_recipient":  "",
				}))
			}
		}
	}

	h.recordRun(*run)

	// Execute per-agent heartbeat pulses for custom active agents
	h.checkCustomAgentPulses(ctx, now, manual)

	return run
}

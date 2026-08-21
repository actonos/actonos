package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

// HeartbeatRun represents an execution record of a proactive heartbeat cycle.
type HeartbeatRun struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id"`
	ExecutedAt time.Time `json:"executed_at"`
	Status     string    `json:"status"` // "ok", "action_taken", "error"
	Summary    string    `json:"summary"`
	TokensUsed int       `json:"tokens_used"`
}

// SessionHistoryProvider defines capabilities for loading and persisting task conversational sessions.
type SessionHistoryProvider interface {
	GetOrCreateSession(ctx context.Context, channelID, senderID, senderName, firstMessage, agentID string) (string, error)
	LoadRecentHistory(ctx context.Context, convID string, limit int) []llm.Message
	SaveMessage(ctx context.Context, convID, agentID, role, content string, toolCalls any) error
}

// ApprovalListProvider defines capabilities for inspecting pending system approvals.
type ApprovalListProvider interface {
	List(ctx context.Context, status string, limit int) ([]tools.ApprovalRequest, error)
}

// HeartbeatDaemon monitors proactive trigger rules, executes cognitive self-driving checks, and manages autonomous agent pulse.
type HeartbeatDaemon struct {
	mu            sync.RWMutex
	executionMu   sync.Mutex
	agentMgr      *AgentManager
	engine        *Engine
	eventBus      *bus.EventBus
	db            *sql.DB
	taskMgr       *TaskManager
	sessionMgr    SessionHistoryProvider
	approvalMgr   ApprovalListProvider
	interval      time.Duration
	enabled       bool
	targetChannel string
	targetAccount string
	ackMaxChars   int
	activeStart   string
	activeEnd     string
	activeTZ      string
	stopCh        chan struct{}
	triggerCh     chan struct{}
	lastRun       time.Time
	running       bool
	stallTracker  map[string]taskStallState
}

// taskStallState tracks in-memory (non-persisted) consecutive-cycle progress
// for a mission task so a task that never advances can be flagged for
// operator attention instead of silently retrying forever every cycle.
type taskStallState struct {
	lastProgress  int
	stalledCycles int
	notified      bool
}

// maxStalledCyclesBeforeEscalation is how many consecutive heartbeat cycles a
// task may report unchanged progress before the daemon escalates a one-time
// stall notice to the operator instead of retrying silently forever.
const maxStalledCyclesBeforeEscalation = 3

// NewHeartbeatDaemon creates a new HeartbeatDaemon.
func NewHeartbeatDaemon(
	agentMgr *AgentManager,
	engine *Engine,
	eventBus *bus.EventBus,
	db *sql.DB,
	_ string,
	interval time.Duration,
) *HeartbeatDaemon {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &HeartbeatDaemon{
		agentMgr:      agentMgr,
		engine:        engine,
		eventBus:      eventBus,
		db:            db,
		interval:      interval,
		enabled:       true,
		targetChannel: "all",
		targetAccount: "all",
		ackMaxChars:   defaultAckMaxChars,
		stopCh:        make(chan struct{}),
		triggerCh:     make(chan struct{}, 1),
		stallTracker:  make(map[string]taskStallState),
	}
}

// SetTaskManager injects the task backlog coordinator.
func (h *HeartbeatDaemon) SetTaskManager(tm *TaskManager) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.taskMgr = tm
}

// SetSessionManager injects the working session persistence provider.
func (h *HeartbeatDaemon) SetSessionManager(sm SessionHistoryProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessionMgr = sm
}

// SetApprovalManager injects the pending approvals provider.
func (h *HeartbeatDaemon) SetApprovalManager(am ApprovalListProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.approvalMgr = am
}

// TriggerWakeup schedules an immediate heartbeat evaluation without blocking.
func (h *HeartbeatDaemon) TriggerWakeup() {
	select {
	case h.triggerCh <- struct{}{}:
	default:
	}
}

// SyncConfig dynamically adjusts heartbeat interval and active status.
func (h *HeartbeatDaemon) SyncConfig(cfg HeartbeatConfig) {
	h.mu.Lock()
	if cfg.IntervalMinutes > 0 {
		h.interval = time.Duration(cfg.IntervalMinutes) * time.Minute
	}
	h.enabled = cfg.Enabled
	if cfg.TargetChannel != "" {
		h.targetChannel = cfg.TargetChannel
	}
	if cfg.TargetAccountID != "" {
		h.targetAccount = cfg.TargetAccountID
	}
	if cfg.AckMaxChars > 0 {
		h.ackMaxChars = cfg.AckMaxChars
	}
	h.activeStart = cfg.ActiveHoursStart
	h.activeEnd = cfg.ActiveHoursEnd
	h.activeTZ = cfg.ActiveHoursTimezone
	h.mu.Unlock()

	slog.Info("heartbeat daemon synchronized with config", "interval", h.interval.String(), "enabled", cfg.Enabled)
	h.TriggerWakeup()
}

// Start launches the autonomous heartbeat evaluation loop.
func (h *HeartbeatDaemon) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true

	// Sync with persisted config if task manager is ready
	if h.taskMgr != nil {
		if cfg, err := h.taskMgr.GetHeartbeatConfig(ctx); err == nil && cfg != nil {
			if cfg.IntervalMinutes > 0 {
				h.interval = time.Duration(cfg.IntervalMinutes) * time.Minute
			}
			h.enabled = cfg.Enabled
			if cfg.TargetChannel != "" {
				h.targetChannel = cfg.TargetChannel
			}
			if cfg.TargetAccountID != "" {
				h.targetAccount = cfg.TargetAccountID
			}
			if cfg.AckMaxChars > 0 {
				h.ackMaxChars = cfg.AckMaxChars
			}
			h.activeStart = cfg.ActiveHoursStart
			h.activeEnd = cfg.ActiveHoursEnd
			h.activeTZ = cfg.ActiveHoursTimezone
		}
	}
	h.mu.Unlock()

	slog.Info("autonomous heartbeat daemon started", "interval", h.interval.String(), "enabled", h.enabled)
	go h.loop(ctx)

	// Launch initial evaluation pulse shortly after startup
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-time.After(3 * time.Second):
			h.TriggerWakeup()
		}
	}()
}

func (h *HeartbeatDaemon) loop(ctx context.Context) {
	h.mu.RLock()
	interval := h.interval
	h.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-h.triggerCh:
			h.mu.RLock()
			enabled := h.enabled
			curInterval := h.interval
			h.mu.RUnlock()

			if curInterval != interval {
				interval = curInterval
				ticker.Reset(interval)
			}

			if enabled {
				h.checkCycle(ctx, false)
			}
		case <-ticker.C:
			h.mu.RLock()
			enabled := h.enabled
			curInterval := h.interval
			h.mu.RUnlock()

			if curInterval != interval {
				interval = curInterval
				ticker.Reset(interval)
			}

			if enabled {
				h.checkCycle(ctx, false)
			}
		}
	}
}

// TriggerManualPulse executes an immediate on-demand heartbeat pulse. Unlike
// scheduled/triggered cycles, a manual pulse always bypasses the trigger
// cooldown and active-hours window because the operator explicitly asked
// for it right now (mirrors OpenClaw's `system event --mode now`).
func (h *HeartbeatDaemon) TriggerManualPulse(ctx context.Context) (*HeartbeatRun, error) {
	slog.Info("manual heartbeat pulse triggered by user")
	run := h.checkCycle(ctx, true)
	return run, nil
}

// minTriggerGap coalesces trigger storms (e.g. a task mutation and an
// approval decision firing TriggerWakeup within moments of each other) so a
// full autonomous agent turn is not run once per event. Scheduled ticks and
// manual pulses are never subject to this gap.
const minTriggerGap = 15 * time.Second

// checkCycle runs the autonomous cognitive heartbeat iteration. manual is
// true only for operator-initiated pulses (TriggerManualPulse); it bypasses
// the trigger cooldown and active-hours window.
func (h *HeartbeatDaemon) checkCycle(ctx context.Context, manual bool) *HeartbeatRun {
	h.executionMu.Lock()
	defer h.executionMu.Unlock()

	now := time.Now().UTC()
	h.mu.Lock()
	if !manual {
		if !h.lastRun.IsZero() && now.Sub(h.lastRun) < minTriggerGap {
			h.mu.Unlock()
			slog.Debug("heartbeat cycle skipped: trigger cooldown active", "since_last_run", now.Sub(h.lastRun).String())
			return nil
		}
		if !h.withinActiveHoursLocked(now) {
			h.mu.Unlock()
			slog.Debug("heartbeat cycle skipped: outside configured active hours")
			return nil
		}
	}
	h.lastRun = now
	h.mu.Unlock()

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

	var activeTask *AutonomousTask
	if h.taskMgr != nil {
		// Priority order: in_progress first, then pending
		inProg, _ := h.taskMgr.ListTasks(ctx, "in_progress", "")
		for i := range inProg {
			t := &inProg[i]
			if t.CreatedBy == "system" {
				continue
			}
			// Check if this in_progress task is currently paused waiting for operator approval
			if h.approvalMgr != nil {
				pendingApprovals, err := h.approvalMgr.List(ctx, "pending", 50)
				if err == nil && len(pendingApprovals) > 0 {
					isPendingApproval := false
					for _, pa := range pendingApprovals {
						if pa.AgentID == t.AssignedAgentID || pa.AgentID == primaryAgentID {
							isPendingApproval = true
							break
						}
					}
					if isPendingApproval {
						slog.Info("task execution is paused waiting for operator approval; skipping cycle", "task_id", t.ID)
						continue
					}
				}
			}
			activeTask = t
			break
		}
		if activeTask == nil {
			pending, _ := h.taskMgr.ListTasks(ctx, "pending", "")
			for i := range pending {
				if pending[i].CreatedBy == "system" {
					continue
				}
				candidateAgent := pending[i].AssignedAgentID
				if candidateAgent == "" || candidateAgent == "auto" {
					candidateAgent = primaryAgentID
				}
				// Only skip launching THIS task if an approval is pending for
				// its own assigned agent — an unrelated approval elsewhere in
				// the system must never block the entire backlog from making
				// progress.
				if h.approvalMgr != nil {
					pendingApprovals, err := h.approvalMgr.List(ctx, "pending", 50)
					if err == nil && len(pendingApprovals) > 0 {
						isBlockedByApproval := false
						for _, pa := range pendingApprovals {
							if pa.AgentID == candidateAgent {
								isBlockedByApproval = true
								break
							}
						}
						if isBlockedByApproval {
							slog.Info("task launch deferred: its assigned agent has a pending approval", "task_id", pending[i].ID, "agent_id", candidateAgent)
							continue
						}
					}
				}
				activeTask = &pending[i]
				if activeTask != nil {
					break
				}
			}
		}
	}

	run := &HeartbeatRun{
		AgentID:    primaryAgentID,
		ExecutedAt: time.Now().UTC(),
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	run.ID = "hb_" + hex.EncodeToString(b)

	// A heartbeat without a task or an actionable scratchpad must stay entirely
	// silent. Calling the model here lets it invent work such as a cron schedule.
	if activeTask == nil && !hasActionableHeartbeatDirectives(standingDirectives) {
		run.Status = "ok"
		run.Summary = "System nominal. Zero tasks pending. No actionable heartbeat directives."
		h.recordRun(*run)
		return run
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

		prompt := BuildHeartbeatMissionPrompt(activeTask.Title, activeTask.Description, standingDirectives)

		// Suppress episodic memory for heartbeat task execution to prevent stale
		// memories from deleted tasks from contaminating the current task context.
		taskCtx := context.WithValue(ctx, "task_id", activeTask.ID)
		taskCtx = context.WithValue(taskCtx, "suppress_episodic_memory", true)
		taskCtx = context.WithValue(taskCtx, "heartbeat_headless_mode", true)
		taskCtx = tools.WithDeniedTools(taskCtx, "native_cron_schedule")
		resp, execErr := h.engine.ExecuteAutonomousGoal(taskCtx, assignedAgent, prompt, history)
		if execErr != nil {
			var approvalErr *tools.ApprovalRequiredError
			if errors.As(execErr, &approvalErr) {
				run.Status = "approval_required"
				run.Summary = fmt.Sprintf("Mission '%s' paused: operator approval required for '%s'.", activeTask.Title, approvalErr.Approval.ToolName)
				activeTask.ExecutionLog = fmt.Sprintf("Paused: operator approval required for tool '%s'.", approvalErr.Approval.ToolName)
				if h.taskMgr != nil {
					_ = h.taskMgr.UpdateTask(ctx, *activeTask)
				}
				h.recordRun(*run)
				return run
			}
			run.Status = "error"
			run.Summary = fmt.Sprintf("Failed executing task '%s': %v", activeTask.Title, execErr)
			slog.Error("heartbeat task execution error", "task_id", activeTask.ID, "error", execErr)
		} else if resp != nil {
			run.TokensUsed = resp.Usage.TotalTokens
			content := strings.TrimSpace(resp.Content)

			// Persist step in session history
			if h.sessionMgr != nil {
				_ = h.sessionMgr.SaveMessage(ctx, convID, assignedAgent, "assistant", resp.Content, resp.ToolCalls)
			}

			fullCleaned := cleanFullContent(content)
			shortLog := shortSummary(content, 250)

			// Parse Task status transitions
			if strings.Contains(content, "[TASK_COMPLETED]") && h.engine.verifier.VerifyTaskCompletion(activeTask.Description, content, resp.ToolCalls) {
				activeTask.Status = "completed"
				activeTask.Progress = 100
				activeTask.ExecutionLog = shortLog
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
				// Extract progress percentage if present
				reProg := regexp.MustCompile(`\[PROGRESS:\s*(\d+)%?\]`)
				match := reProg.FindStringSubmatch(content)
				if len(match) > 1 {
					if pVal, err := strconv.Atoi(match[1]); err == nil && pVal > activeTask.Progress {
						activeTask.Progress = pVal
					}
				} else if activeTask.Progress < 50 {
					activeTask.Progress = 50
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
			if stalled && activeTask.Status == "in_progress" {
				stallNote := fmt.Sprintf(" [STALL WARNING: no progress advancement across %d consecutive heartbeat cycles — operator review recommended]", stallCycles)
				activeTask.ExecutionLog += stallNote
				run.Summary += stallNote
				slog.Warn("mission task stalled: no progress across multiple heartbeat cycles", "task_id", activeTask.ID, "stalled_cycles", stallCycles, "progress", activeTask.Progress)
			}

			if h.taskMgr != nil {
				_ = h.taskMgr.UpdateTask(ctx, *activeTask)
			}

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

			// Proactively notify user with FULL UNTRUNCATED content through event bus if target channel configured
			if h.eventBus != nil && !alreadyNotified && !isRedundantToolReport && activeTask.TargetChannel != "none" && activeTask.TargetChannel != "" {
				h.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, assignedAgent, map[string]any{
					"type":              "proactive_cron_notification",
					"job_name":          fmt.Sprintf("Mission: %s", activeTask.Title),
					"content":           fullCleaned,
					"target_channel":    activeTask.TargetChannel,
					"target_account_id": activeTask.TargetAccountID,
					"target_recipient":  "",
				}))
			}

			// If active task was completed and there are more pending tasks waiting in backlog, trigger immediate next cycle
			if h.taskMgr != nil && activeTask.Status == "completed" {
				pendingTasks, _ := h.taskMgr.ListTasks(ctx, "pending", "")
				if len(pendingTasks) > 0 {
					slog.Info("queueing immediate next heartbeat cycle for pending backlog tasks", "pending_count", len(pendingTasks))
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
	routineCtx := context.WithValue(ctx, "suppress_episodic_memory", true)
	routineCtx = context.WithValue(routineCtx, "heartbeat_headless_mode", true)
	routineCtx = tools.WithDeniedTools(routineCtx, "native_cron_schedule")
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
	return run
}

// heartbeatOKToken is OpenClaw's canonical acknowledgement token: when it
// appears at the very start or end of a routine heartbeat reply (with a
// short enough remainder), the cycle is treated as silent/nominal and
// nothing is delivered to the user.
// See https://docs.openclaw.ai/vi/gateway/heartbeat.
const heartbeatOKToken = "HEARTBEAT_OK"

// defaultAckMaxChars bounds how much leading/trailing commentary may
// accompany HEARTBEAT_OK before a reply is treated as a real alert instead of
// a silent acknowledgement (OpenClaw default: 300).
const defaultAckMaxChars = 300

// classifyHeartbeatResponse implements OpenClaw's heartbeat response
// contract: HEARTBEAT_OK (and standard nominal/OK variations) is treated as an
// acknowledgement when it appears at the boundary of the trimmed reply or matches
// nominal status patterns AND the remaining content is within ackMaxChars. Returns
// (isAck, alertText) — alertText is only meaningful when isAck is false.
func classifyHeartbeatResponse(content string, ackMaxChars int) (bool, string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return true, ""
	}
	if ackMaxChars <= 0 {
		ackMaxChars = defaultAckMaxChars
	}

	nominalTokens := []string{
		heartbeatOKToken,
		"SYSTEM_NOMINAL",
		"ALL_NOMINAL",
		"ZERO_TASKS_PENDING",
	}
	for _, tok := range nominalTokens {
		if rest, ok := stripBoundaryToken(trimmed, tok, true); ok && len(rest) <= ackMaxChars {
			return true, ""
		}
		if rest, ok := stripBoundaryToken(trimmed, tok, false); ok && len(rest) <= ackMaxChars {
			return true, ""
		}
	}

	// Double-barrier safety layer: Check if response is an affirmative/nominal status without new actionable alerts
	lower := strings.ToLower(trimmed)
	cleanLower := strings.Trim(lower, " .!;\n\r\t*`\"'")
	nominalExactPhrases := []string{
		"heartbeat ok",
		"heartbeat_ok",
		"ok",
		"system ok",
		"everything is ok",
		"everything is nominal",
		"system nominal",
		"system is nominal",
		"all systems nominal",
		"all systems operational",
		"zero tasks pending",
		"no actionable tasks",
		"no actionable items",
		"no proactive notification required",
		"không có vấn đề gì",
		"mọi thứ đều ổn",
		"hệ thống bình thường",
		"hệ thống hoạt động bình thường",
		"không cần hành động",
		"không có tác vụ nào cần xử lý",
		"tất cả đều ổn",
	}
	for _, phrase := range nominalExactPhrases {
		if cleanLower == phrase {
			return true, ""
		}
	}

	return false, trimmed
}

// notificationIntentKeywords are common Vietnamese/English phrasings a user
// uses when they expect a task's result to be delivered somewhere (a chat
// channel, Telegram, etc). Used only to decide whether to WARN about a
// TargetChannel=="none" misconfiguration — never to infer or override the
// actual delivery target, which always remains an explicit structured field.
var notificationIntentKeywords = []string{
	"gửi thông báo", "gửi tới", "gửi đến", "báo cho", "thông báo cho", "gửi cho",
	"send to", "send it to", "notify", "send a message", "send this to",
}

// mentionsNotificationIntent reports whether text appears to ask for the
// result to be delivered/sent somewhere.
func mentionsNotificationIntent(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range notificationIntentKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// idleChatterPatterns match common conversational greeting / capability-menu
// / "how can I help" openers that a model sometimes free-associates instead
// of following the heartbeat directive-execution-or-HEARTBEAT_OK contract.
// These are unambiguous signs of an off-topic, non-substantive reply: a real
// alert never opens by greeting the user or asking what they need.
var idleChatterPatterns = []string{
	"chào bieber", "chào anh", "chào bạn", "xin chào",
	"tôi đã sẵn sàng", "tôi đang sẵn sàng", "em đã sẵn sàng", "sẵn sàng hỗ trợ",
	"bạn cần tôi hỗ trợ", "bạn muốn tôi hỗ trợ", "anh cần em hỗ trợ", "cần hỗ trợ gì",
	"how can i help", "how can i assist", "what would you like", "i'm ready to help",
	"i am ready to help", "ready to assist", "what can i do for you",
}

// looksLikeIdleChatter reports whether content is a short conversational
// greeting/self-introduction with no substantive directive execution result.
// Substantive content (e.g. stories, reports, analysis longer than 250 characters)
// is never treated as idle chatter.
func looksLikeIdleChatter(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	// Substantive content is not idle chatter
	if len([]rune(trimmed)) > 250 {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, pat := range idleChatterPatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

// stripBoundaryToken removes token from the start (prefix=true) or end
// (prefix=false) of s, requiring a word boundary so e.g. "HEARTBEAT_OKAY"
// does not falsely match "HEARTBEAT_OK". Returns the remaining trimmed text
// and whether the token was actually found at that boundary.
func stripBoundaryToken(s, token string, prefix bool) (string, bool) {
	if prefix {
		if !strings.HasPrefix(s, token) {
			return "", false
		}
		if len(s) > len(token) && isTokenBoundaryRune(rune(s[len(token)])) {
			return "", false
		}
		return strings.TrimSpace(s[len(token):]), true
	}
	if !strings.HasSuffix(s, token) {
		return "", false
	}
	if len(s) > len(token) && isTokenBoundaryRune(rune(s[len(s)-len(token)-1])) {
		return "", false
	}
	return strings.TrimSpace(s[:len(s)-len(token)]), true
}

func isTokenBoundaryRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// parseHHMM parses an "HH:MM" clock time into minutes-since-midnight,
// accepting "24:00" as the end of a day.
func parseHHMM(v string) (int, bool) {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	hh, errH := strconv.Atoi(parts[0])
	mm, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || hh < 0 || hh > 24 || mm < 0 || mm > 59 || (hh == 24 && mm != 0) {
		return 0, false
	}
	return hh*60 + mm, true
}

// trackTaskStall updates the in-memory (non-persisted, resets on restart)
// per-task progress tracker and reports whether this task has now reached
// maxStalledCyclesBeforeEscalation consecutive cycles with unchanged
// progress. A terminal status (anything other than in_progress) clears the
// tracked state entirely, since a completed/blocked/failed outcome is an
// explicit result, not a stall. Only escalates once per stall streak.
func (h *HeartbeatDaemon) trackTaskStall(taskID, status string, progress int) (escalate bool, cycles int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if status != "in_progress" {
		delete(h.stallTracker, taskID)
		return false, 0
	}

	state := h.stallTracker[taskID]
	if progress > state.lastProgress {
		state = taskStallState{lastProgress: progress}
	} else {
		state.stalledCycles++
	}
	h.stallTracker[taskID] = state

	if state.stalledCycles >= maxStalledCyclesBeforeEscalation && !state.notified {
		state.notified = true
		h.stallTracker[taskID] = state
		return true, state.stalledCycles
	}
	return false, state.stalledCycles
}

// withinActiveHoursLocked reports whether now falls inside the configured
// active-hours window. Must be called with h.mu already held. When no window
// is configured, heartbeat runs 24/7 (OpenClaw's default). An explicit
// zero-width window (start == end) always evaluates to outside the window.
func (h *HeartbeatDaemon) withinActiveHoursLocked(now time.Time) bool {
	if h.activeStart == "" && h.activeEnd == "" {
		return true
	}
	startMin, okS := parseHHMM(h.activeStart)
	endMin, okE := parseHHMM(h.activeEnd)
	if !okS || !okE {
		// Misconfigured window: fail open rather than silently going quiet forever.
		return true
	}
	if startMin == endMin {
		return false
	}
	loc := time.UTC
	if h.activeTZ != "" {
		if l, err := time.LoadLocation(h.activeTZ); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	nowMin := local.Hour()*60 + local.Minute()
	if startMin < endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	// Wrap-around window (e.g. 22:00-06:00).
	return nowMin >= startMin || nowMin < endMin
}

func cleanFullContent(content string) string {
	cleaned := strings.ReplaceAll(content, "[TASK_COMPLETED]", "")
	reProg := regexp.MustCompile(`\[PROGRESS:\s*\d+%?\]`)
	cleaned = reProg.ReplaceAllString(cleaned, "")
	reBlocked := regexp.MustCompile(`\[TASK_BLOCKED:[^\]]*\]`)
	cleaned = reBlocked.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func shortSummary(content string, maxLen int) string {
	cleaned := cleanFullContent(content)
	if maxLen > 3 && len(cleaned) > maxLen {
		return cleaned[:maxLen-3] + "..."
	}
	return cleaned
}

func hasActionableHeartbeatDirectives(content string) bool {
	if strings.TrimSpace(content) == legacyDefaultHeartbeatDirective {
		return false
	}

	inComment := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		for inComment || strings.HasPrefix(trimmed, "<!--") {
			start := 0
			if inComment {
				start = 0
			}
			if end := strings.Index(trimmed[start:], "-->"); end >= 0 {
				trimmed = strings.TrimSpace(trimmed[start+end+3:])
				inComment = false
				continue
			}
			inComment = true
			trimmed = ""
			break
		}
		if trimmed == "" ||
			regexp.MustCompile(`^#+(\s|$)`).MatchString(trimmed) ||
			regexp.MustCompile(`^[-*+]\s*(\[[\sXx]?\]\s*)?$`).MatchString(trimmed) ||
			regexp.MustCompile("^```[A-Za-z0-9_-]*$").MatchString(trimmed) {
			continue
		}
		return true
	}
	return false
}

func (h *HeartbeatDaemon) recordRun(run HeartbeatRun) {
	if h.db == nil {
		return
	}

	query := `
	INSERT INTO heartbeat_runs (id, agent_id, executed_at, status, summary, tokens_used)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := h.db.Exec(query, run.ID, run.AgentID, run.ExecutedAt, run.Status, run.Summary, run.TokensUsed)
	if err != nil {
		slog.Warn("failed to record heartbeat run in sqlite", "error", err)
	}
}

// GetRecentRuns returns the most recent heartbeat runs from SQLite.
func (h *HeartbeatDaemon) GetRecentRuns(ctx context.Context, limit int) ([]HeartbeatRun, error) {
	if h.db == nil {
		return []HeartbeatRun{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT id, agent_id, executed_at, status, summary, tokens_used
		FROM heartbeat_runs
		ORDER BY executed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []HeartbeatRun
	for rows.Next() {
		var r HeartbeatRun
		if err := rows.Scan(&r.ID, &r.AgentID, &r.ExecutedAt, &r.Status, &r.Summary, &r.TokensUsed); err == nil {
			runs = append(runs, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []HeartbeatRun{}
	}
	return runs, nil
}

// Stop gracefully terminates the heartbeat daemon.
func (h *HeartbeatDaemon) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	h.running = false
	close(h.stopCh)
	slog.Info("autonomous heartbeat daemon stopped")
}

package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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
	mu           sync.RWMutex
	executionMu  sync.Mutex
	agentMgr     *AgentManager
	engine       *Engine
	eventBus     *bus.EventBus
	db           *sql.DB
	taskMgr      *TaskManager
	sessionMgr   SessionHistoryProvider
	approvalMgr  ApprovalListProvider
	workspaceDir string
	interval     time.Duration
	enabled      bool
	stopCh       chan struct{}
	triggerCh    chan struct{}
	lastRun      time.Time
	running      bool
}

// NewHeartbeatDaemon creates a new HeartbeatDaemon.
func NewHeartbeatDaemon(
	agentMgr *AgentManager,
	engine *Engine,
	eventBus *bus.EventBus,
	db *sql.DB,
	workspaceDir string,
	interval time.Duration,
) *HeartbeatDaemon {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if workspaceDir == "" {
		workspaceDir = "./data/workspace"
	}
	return &HeartbeatDaemon{
		agentMgr:     agentMgr,
		engine:       engine,
		eventBus:     eventBus,
		db:           db,
		workspaceDir: workspaceDir,
		interval:     interval,
		enabled:      true,
		stopCh:       make(chan struct{}),
		triggerCh:    make(chan struct{}, 1),
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
				h.checkCycle(ctx)
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
				h.checkCycle(ctx)
			}
		}
	}
}

// TriggerManualPulse executes an immediate on-demand heartbeat pulse.
func (h *HeartbeatDaemon) TriggerManualPulse(ctx context.Context) (*HeartbeatRun, error) {
	slog.Info("manual heartbeat pulse triggered by user")
	run := h.checkCycle(ctx)
	return run, nil
}

// checkCycle runs the autonomous cognitive heartbeat iteration.
func (h *HeartbeatDaemon) checkCycle(ctx context.Context) *HeartbeatRun {
	h.executionMu.Lock()
	defer h.executionMu.Unlock()

	h.mu.Lock()
	h.lastRun = time.Now().UTC()
	h.mu.Unlock()

	primaryAgentID := "agent_system_core"

	// 1. Read standing directives
	heartbeatMDPath := filepath.Join(h.workspaceDir, "HEARTBEAT.md")
	standingDirectives := "Monitor background tasks, verify system health, maintain Zero-Noise if nominal."
	if data, err := os.ReadFile(heartbeatMDPath); err == nil && len(data) > 0 {
		standingDirectives = string(data)
	}

	var activeTask *AutonomousTask
	if h.taskMgr != nil {
		// Priority order: in_progress first, then pending
		inProg, _ := h.taskMgr.ListTasks(ctx, "in_progress", "")
		for i := range inProg {
			t := &inProg[i]
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
			if len(pending) > 0 {
				if h.approvalMgr != nil {
					pendingApprovals, err := h.approvalMgr.List(ctx, "pending", 50)
					if err == nil && len(pendingApprovals) > 0 {
						slog.Info("pending approvals exist in system; waiting for resolution before launching new tasks", "count", len(pendingApprovals))
					} else {
						activeTask = &pending[0]
					}
				} else {
					activeTask = &pending[0]
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

		prompt := fmt.Sprintf(`[AUTONOMOUS MISSION EXECUTION CYCLE]
Task ID: %s | Title: %s | Priority: %s | Progress: %d%%
Task Directive: %s
Standing Directives: %s

YOU ARE EXECUTING THIS MISSION AUTONOMOUSLY.
Review your previous dialogue history for context so you do not repeat already completed actions.
CRITICAL INSTRUCTIONS:
1. EXECUTE ONLY the current task directive above. DO NOT reference, continue, or act on any unrelated topics, previous tasks, or stale memories. If your history mentions tasks unrelated to this directive, IGNORE them completely.
2. Carry out the next logical action using your authorized tools.
3. DO NOT call 'native_channel_notify' tool because the ActonOS Mission Coordinator will automatically deliver your summary and status updates to '%s'.
4. At the end of your response, specify task progress:
   - If completely finished, end with: "[TASK_COMPLETED]" followed by a concise executive summary of the result.
   - If ongoing, end with: "[PROGRESS: X%%]" (where X is an integer 1-99) followed by a short note on what was accomplished.
   - If blocked on missing requirements or errors, end with: "[TASK_BLOCKED: reason]".
5. Keep your output professional, structured, and factual. Do not include meta-filler like "The notification has been sent".`,
			activeTask.ID, activeTask.Title, activeTask.Priority, activeTask.Progress,
			activeTask.Description, standingDirectives, activeTask.TargetChannel,
		)

		// Suppress episodic memory for heartbeat task execution to prevent stale
		// memories from deleted tasks from contaminating the current task context.
		taskCtx := context.WithValue(ctx, "task_id", activeTask.ID)
		taskCtx = context.WithValue(taskCtx, "suppress_episodic_memory", true)
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

			// If there are more pending tasks waiting in backlog, trigger immediate next cycle
			if h.taskMgr != nil {
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
	backlogSummary := "All previous backlog missions are COMPLETED. There are ZERO pending or in-progress tasks."
	if h.taskMgr != nil {
		completedList, _ := h.taskMgr.ListTasks(ctx, "completed", "")
		if len(completedList) > 0 {
			backlogSummary = fmt.Sprintf("All %d previous backlog missions have been COMPLETED. There are 0 active or pending tasks in backlog.", len(completedList))
		}
	}

	prompt := fmt.Sprintf(
		"[AUTONOMOUS HEARTBEAT BRAIN CYCLE]\nCurrent UTC Time: %s\nBacklog Status: %s\nStanding Directives:\n%s\n\n"+
			"ROUTINE EVALUATION INSTRUCTIONS:\n"+
			"1. ALL past missions are finished. DO NOT restart, continue, or execute any old completed missions or past tasks.\n"+
			"2. Only evaluate current system health and the standing directives above.\n"+
			"3. If everything is nominal and no proactive action or user notification is needed, reply exactly 'HEARTBEAT_OK'.\n"+
			"4. If an action or alert is necessary, execute it and provide a concise summary. DO NOT call 'native_channel_notify' tool as the system will route your response automatically.",
		time.Now().UTC().Format(time.RFC3339),
		backlogSummary,
		standingDirectives,
	)

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

		if strings.Contains(trimmed, "HEARTBEAT_OK") || trimmed == "" {
			run.Status = "ok"
			run.Summary = "System nominal. Zero tasks pending. No proactive notification required."
			slog.Debug("heartbeat nominal (zero noise)", "agent_id", primaryAgentID)
		} else {
			run.Status = "action_taken"
			run.Summary = shortSummary(trimmed, 250)
			slog.Info("heartbeat performed proactive action", "agent_id", primaryAgentID)

			alreadyNotified := false
			for _, tc := range resp.ToolCalls {
				if tc.Function.Name == "native_channel_notify" || tc.Function.Name == "channel_notify" {
					alreadyNotified = true
					break
				}
			}

			isRedundantToolReport := strings.HasPrefix(trimmed, "The notification has been successfully sent") ||
				strings.HasPrefix(trimmed, "Successfully dispatched proactive notification") ||
				strings.HasPrefix(trimmed, "Notification sent")

			if h.eventBus != nil && !alreadyNotified && !isRedundantToolReport {
				h.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, primaryAgentID, map[string]any{
					"type":              "proactive_cron_notification",
					"job_name":          "Heartbeat Pulse",
					"content":           cleanFullContent(trimmed),
					"target_channel":    "all",
					"target_account_id": "all",
					"target_recipient":  "",
				}))
			}
		}
	}

	h.recordRun(*run)
	return run
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

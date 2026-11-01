package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
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
	mu              sync.RWMutex
	executionMu     sync.Mutex
	agentMgr        *AgentManager
	engine          *Engine
	eventBus        *bus.EventBus
	db              *sql.DB
	taskMgr         *TaskManager
	sessionMgr      SessionHistoryProvider
	approvalMgr     ApprovalListProvider
	interval        time.Duration
	enabled         bool
	targetChannel   string
	targetAccount   string
	ackMaxChars     int
	activeStart     string
	activeEnd       string
	activeTZ        string
	stopCh          chan struct{}
	triggerCh       chan struct{}
	lastRun         time.Time
	running         bool
	stallTracker    map[string]taskStallState
	lastAgentPulse  map[string]time.Time
	cronSched       *CronScheduler
	proactiveEngine *ProactiveEngine
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
		agentMgr:       agentMgr,
		engine:         engine,
		eventBus:       eventBus,
		db:             db,
		interval:       interval,
		enabled:        true,
		targetChannel:  "all",
		targetAccount:  "all",
		ackMaxChars:    defaultAckMaxChars,
		stopCh:         make(chan struct{}),
		triggerCh:      make(chan struct{}, 1),
		stallTracker:   make(map[string]taskStallState),
		lastAgentPulse: make(map[string]time.Time),
	}
}

// SetProactiveEngine attaches the proactive anomaly detection engine.
func (h *HeartbeatDaemon) SetProactiveEngine(pe *ProactiveEngine) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.proactiveEngine = pe
}

// SetCronScheduler attaches cron quota inspection so pulses may create jobs within a cap.
func (h *HeartbeatDaemon) SetCronScheduler(cs *CronScheduler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cronSched = cs
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

// LastRunAt returns the timestamp of the last attempted (non-skip-return) cycle.
func (h *HeartbeatDaemon) LastRunAt() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastRun
}

// Interval returns the configured pulse interval.
func (h *HeartbeatDaemon) Interval() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.interval
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

	h.syncTriggerRules(ctx)

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

// adoptPersistedPlan copies the engine-owned DAG and plan-derived progress onto
// the in-memory heartbeat task so a later UpdateTask cannot rewind completed
// steps. Status is left to the heartbeat completion policy.
func (h *HeartbeatDaemon) adoptPersistedPlan(ctx context.Context, task *AutonomousTask) {
	if h.taskMgr == nil || task == nil {
		return
	}
	latest, err := h.taskMgr.GetTask(ctx, task.ID)
	if err != nil || latest == nil {
		return
	}
	if latest.Plan != nil {
		task.Plan = latest.Plan
	}
	if latest.Progress > task.Progress {
		task.Progress = latest.Progress
	}
}

// trackTaskStall updates the in-memory (non-persisted, resets on restart)
// per-task progress tracker and reports whether this task has now reached
// maxStalledCyclesBeforeEscalation consecutive cycles with unchanged
// progress. A terminal status (anything other than in_progress) clears the
// tracked state entirely, since a completed/blocked/failed outcome is an
// explicit result, not a stall. Only escalates once per stall streak.

func (h *HeartbeatDaemon) persistMissionTask(ctx context.Context, task *AutonomousTask) {
	if h.taskMgr == nil || task == nil {
		return
	}
	h.adoptPersistedPlan(ctx, task)
	_ = h.taskMgr.UpdateTask(ctx, *task)
}

func (h *HeartbeatDaemon) trackTaskStall(taskID, status string, progress int) (escalate bool, cycles int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if status != "in_progress" {
		delete(h.stallTracker, taskID)
		return false, 0
	}

	state := h.stallTracker[taskID]
	if progress == 0 || progress > state.lastProgress {
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

func isLegacySystemMission(t AutonomousTask) bool {
	return t.CreatedBy == "system"
}

func (h *HeartbeatDaemon) agentHasPendingApproval(ctx context.Context, agentID string) bool {
	if h.approvalMgr == nil || agentID == "" {
		return false
	}
	pendingApprovals, err := h.approvalMgr.List(ctx, "pending", 50)
	if err != nil || len(pendingApprovals) == 0 {
		return false
	}
	for _, pa := range pendingApprovals {
		if pa.AgentID == agentID {
			return true
		}
	}
	return false
}

// selectRunnableMission picks in_progress then pending work that is not a
// legacy system seed and is not paused on a pending approval. humanWait is
// set when the only remaining missions need an operator.
func (h *HeartbeatDaemon) selectRunnableMission(ctx context.Context, primaryAgentID string) (*AutonomousTask, string) {
	if h.taskMgr == nil {
		return nil, ""
	}
	humanWait := ""
	inProg, _ := h.taskMgr.ListTasks(ctx, "in_progress", "")
	for i := range inProg {
		t := &inProg[i]
		if isLegacySystemMission(*t) {
			continue
		}
		assigned := t.AssignedAgentID
		if assigned == "" || assigned == "auto" {
			assigned = primaryAgentID
		}
		if h.agentHasPendingApproval(ctx, assigned) {
			humanWait = "paused: operator approval required"
			slog.Info("task execution is paused waiting for operator approval; skipping cycle", "task_id", t.ID)
			continue
		}
		return t, ""
	}
	pending, _ := h.taskMgr.ListTasks(ctx, "pending", "")
	for i := range pending {
		if isLegacySystemMission(pending[i]) {
			continue
		}
		candidateAgent := pending[i].AssignedAgentID
		if candidateAgent == "" || candidateAgent == "auto" {
			candidateAgent = primaryAgentID
		}
		if h.agentHasPendingApproval(ctx, candidateAgent) {
			humanWait = "paused: operator approval required"
			slog.Info("task launch deferred: its assigned agent has a pending approval", "task_id", pending[i].ID, "agent_id", candidateAgent)
			continue
		}
		return &pending[i], ""
	}
	blocked, _ := h.taskMgr.ListTasks(ctx, "blocked", "")
	for i := range blocked {
		if isLegacySystemMission(blocked[i]) {
			continue
		}
		return nil, "blocked: operator review required"
	}
	return nil, humanWait
}

func (h *HeartbeatDaemon) syncTriggerRules(ctx context.Context) {
	if h.agentMgr == nil || h.cronSched == nil {
		return
	}
	agents, err := h.agentMgr.List(ctx)
	if err != nil {
		return
	}
	for _, a := range agents {
		if a.Status != StatusActive {
			continue
		}
		for i, rule := range a.TriggerRules {
			if rule.Type != "cron_schedule" || strings.TrimSpace(rule.Expression) == "" {
				continue
			}
			jobID := fmt.Sprintf("trigger_%s_%d", a.AgentID, i)
			_ = h.cronSched.RegisterJob(CronJob{
				ID:       jobID,
				AgentID:  a.AgentID,
				Name:     "trigger:" + a.Name,
				CronExpr: rule.Expression,
				Prompt:   strings.TrimSpace(rule.Filter),
				Enabled:  true,
			})
		}
	}
}

func (h *HeartbeatDaemon) recordSkip(agentID, reason string) *HeartbeatRun {
	run := &HeartbeatRun{
		AgentID:    agentID,
		ExecutedAt: time.Now().UTC(),
		Status:     "skipped",
		Summary:    reason,
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	run.ID = "hb_" + hex.EncodeToString(b)
	h.recordRun(*run)
	return run
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

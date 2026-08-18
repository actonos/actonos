package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/robfig/cron/v3"
)

// CronJob defines a scheduled autonomous task executed by an agent.
type CronJob struct {
	ID              string    `json:"id"`
	AgentID         string    `json:"agent_id"`
	Name            string    `json:"name"`
	CronExpr        string    `json:"cron_expr"`
	Prompt          string    `json:"prompt"`
	TargetChannel   string    `json:"target_channel"`    // "telegram", "discord", "whatsapp", "webhook", "all"
	TargetAccountID string    `json:"target_account_id"` // specific account ID or "all"
	TargetRecipient string    `json:"target_recipient"`
	Enabled         bool      `json:"enabled"`
	LastRun         time.Time `json:"last_run,omitempty"`
	NextRun         time.Time `json:"next_run,omitempty"`
	entryID         cron.EntryID
}

// CronScheduler orchestrates background cron-triggered agent executions and proactive push.
type CronScheduler struct {
	mu                     sync.RWMutex
	cron                   *cron.Cron
	engine                 *Engine
	eventBus               *bus.EventBus
	db                     *sql.DB
	jobs                   map[string]*CronJob
	defaultRecipientGetter func(channel string) string
}

// NewCronScheduler creates a CronScheduler instance with optional SQLite persistence.
func NewCronScheduler(engine *Engine, eventBus *bus.EventBus, db ...*sql.DB) *CronScheduler {
	cs := &CronScheduler{
		cron:     cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor))),
		engine:   engine,
		eventBus: eventBus,
		jobs:     make(map[string]*CronJob),
	}
	if len(db) > 0 && db[0] != nil {
		cs.db = db[0]
		cs.initDB()
		cs.loadJobsFromDB()
	}
	return cs
}

// SetDefaultRecipientGetter configures a provider to resolve default channel recipient IDs.
func (cs *CronScheduler) SetDefaultRecipientGetter(fn func(channel string) string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.defaultRecipientGetter = fn
}

// GetDefaultRecipient returns the default recipient ID for a given channel.
func (cs *CronScheduler) GetDefaultRecipient(channel string) string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.defaultRecipientGetter != nil {
		return cs.defaultRecipientGetter(channel)
	}
	return ""
}

// SetDB attaches a database and hydrates stored cron jobs.
func (cs *CronScheduler) SetDB(db *sql.DB) {
	cs.mu.Lock()
	cs.db = db
	cs.mu.Unlock()
	if db != nil {
		cs.initDB()
		cs.loadJobsFromDB()
	}
}

func (cs *CronScheduler) initDB() {
	if cs.db == nil {
		return
	}
	query := `
	CREATE TABLE IF NOT EXISTS cron_jobs (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		name TEXT NOT NULL,
		cron_expr TEXT NOT NULL,
		prompt TEXT NOT NULL,
		target_channel TEXT NOT NULL DEFAULT 'telegram',
		target_account_id TEXT NOT NULL DEFAULT 'all',
		target_recipient TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		last_run TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`
	_, err := cs.db.Exec(query)
	if err != nil {
		slog.Error("failed to initialize cron_jobs table", "error", err)
	}
	// Run non-destructive column migration for existing DB
	_, _ = cs.db.Exec("ALTER TABLE cron_jobs ADD COLUMN target_account_id TEXT NOT NULL DEFAULT 'all'")
}

func (cs *CronScheduler) loadJobsFromDB() {
	if cs.db == nil {
		return
	}
	rows, err := cs.db.Query("SELECT id, agent_id, name, cron_expr, prompt, target_channel, target_account_id, target_recipient, enabled, last_run FROM cron_jobs")
	if err != nil {
		slog.Error("failed to load cron jobs from database", "error", err)
		return
	}
	defer rows.Close()

	cs.mu.Lock()
	defer cs.mu.Unlock()

	for rows.Next() {
		var job CronJob
		var enabledInt int
		var lastRun sql.NullTime
		if err := rows.Scan(&job.ID, &job.AgentID, &job.Name, &job.CronExpr, &job.Prompt, &job.TargetChannel, &job.TargetAccountID, &job.TargetRecipient, &enabledInt, &lastRun); err != nil {
			continue
		}
		if job.TargetAccountID == "" {
			job.TargetAccountID = "all"
		}
		job.Enabled = (enabledInt == 1)
		if lastRun.Valid {
			job.LastRun = lastRun.Time
		}

		if job.Enabled {
			jobCopy := job
			entryID, err := cs.cron.AddFunc(job.CronExpr, func() {
				cs.executeJob(&jobCopy)
			})
			if err == nil {
				jobCopy.entryID = entryID
				entry := cs.cron.Entry(entryID)
				jobCopy.NextRun = entry.Next
				cs.jobs[jobCopy.ID] = &jobCopy
				slog.Info("hydrated persistent cron job from db", "id", jobCopy.ID, "agent", jobCopy.AgentID, "cron", jobCopy.CronExpr)
			} else {
				slog.Warn("failed to reschedule loaded cron job", "id", job.ID, "cron", job.CronExpr, "error", err)
				cs.jobs[job.ID] = &job
			}
		} else {
			cs.jobs[job.ID] = &job
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating cron jobs from database", "error", err)
	}
}

// Start begins the cron scheduler.
func (cs *CronScheduler) Start(ctx context.Context) {
	cs.cron.Start()
	slog.Info("autonomous cron scheduler started")
}

// Stop terminates the scheduler.
func (cs *CronScheduler) Stop() {
	cs.cron.Stop()
	slog.Info("autonomous cron scheduler stopped")
}

// RegisterJob adds or updates a scheduled proactive agent job and persists it to SQLite.
func (cs *CronScheduler) RegisterJob(job CronJob) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// If job already exists, remove previous cron entry
	if existing, exists := cs.jobs[job.ID]; exists && existing.entryID != 0 {
		cs.cron.Remove(existing.entryID)
	}

	jobCopy := job
	if jobCopy.TargetChannel == "" {
		jobCopy.TargetChannel = "telegram"
	}
	if jobCopy.TargetRecipient == "" && cs.defaultRecipientGetter != nil {
		jobCopy.TargetRecipient = cs.defaultRecipientGetter(jobCopy.TargetChannel)
	}

	if job.Enabled {
		entryID, err := cs.cron.AddFunc(job.CronExpr, func() {
			cs.executeJob(&jobCopy)
		})
		if err != nil {
			return fmt.Errorf("invalid cron expression %q: %w", job.CronExpr, err)
		}

		jobCopy.entryID = entryID
		entry := cs.cron.Entry(entryID)
		jobCopy.NextRun = entry.Next
	}

	cs.jobs[jobCopy.ID] = &jobCopy

	// Persist to SQLite database
	if cs.db != nil {
		enabledInt := 0
		if jobCopy.Enabled {
			enabledInt = 1
		}
		now := time.Now().UTC()
		if jobCopy.TargetAccountID == "" {
			jobCopy.TargetAccountID = "all"
		}
		_, err := cs.db.Exec(`
			INSERT INTO cron_jobs (id, agent_id, name, cron_expr, prompt, target_channel, target_account_id, target_recipient, enabled, last_run, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				agent_id = excluded.agent_id,
				name = excluded.name,
				cron_expr = excluded.cron_expr,
				prompt = excluded.prompt,
				target_channel = excluded.target_channel,
				target_account_id = excluded.target_account_id,
				target_recipient = excluded.target_recipient,
				enabled = excluded.enabled,
				updated_at = excluded.updated_at;
		`, jobCopy.ID, jobCopy.AgentID, jobCopy.Name, jobCopy.CronExpr, jobCopy.Prompt, jobCopy.TargetChannel, jobCopy.TargetAccountID, jobCopy.TargetRecipient, enabledInt, jobCopy.LastRun, now, now)
		if err != nil {
			slog.Error("failed to persist cron job to db", "id", jobCopy.ID, "error", err)
		}
	}

	slog.Info("registered proactive cron job", "id", jobCopy.ID, "agent", jobCopy.AgentID, "cron", jobCopy.CronExpr, "target_channel", jobCopy.TargetChannel, "target_account", jobCopy.TargetAccountID)
	return nil
}

// RemoveJob removes a scheduled job and removes it from SQLite.
func (cs *CronScheduler) RemoveJob(jobID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if job, exists := cs.jobs[jobID]; exists {
		if job.entryID != 0 {
			cs.cron.Remove(job.entryID)
		}
		delete(cs.jobs, jobID)
	}

	if cs.db != nil {
		_, err := cs.db.Exec("DELETE FROM cron_jobs WHERE id = ?", jobID)
		if err != nil {
			slog.Error("failed to delete cron job from db", "id", jobID, "error", err)
		}
	}
}

// ListJobs returns all registered cron jobs with their next run times.
func (cs *CronScheduler) ListJobs() []CronJob {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make([]CronJob, 0, len(cs.jobs))
	for _, j := range cs.jobs {
		item := *j
		if j.entryID != 0 {
			entry := cs.cron.Entry(j.entryID)
			item.NextRun = entry.Next
		}
		result = append(result, item)
	}
	return result
}

// RegisterCron implements tools.CronSchedulerProvider.
func (cs *CronScheduler) RegisterCron(id, agentID, cronExpr, prompt, targetChannel, targetAccountID, targetRecipient string) error {
	if targetChannel == "" {
		targetChannel = "telegram"
	}
	if targetAccountID == "" {
		targetAccountID = "all"
	}
	if targetRecipient == "" && cs.defaultRecipientGetter != nil {
		targetRecipient = cs.defaultRecipientGetter(targetChannel)
	}
	return cs.RegisterJob(CronJob{
		ID:              id,
		Name:            id,
		AgentID:         agentID,
		CronExpr:        cronExpr,
		Prompt:          prompt,
		TargetChannel:   targetChannel,
		TargetAccountID: targetAccountID,
		TargetRecipient: targetRecipient,
		Enabled:         true,
	})
}

// RemoveCron implements tools.CronSchedulerProvider.
func (cs *CronScheduler) RemoveCron(id string) {
	cs.RemoveJob(id)
}

// ListCrons implements tools.CronSchedulerProvider.
func (cs *CronScheduler) ListCrons() []map[string]any {
	jobs := cs.ListJobs()
	var res []map[string]any
	for _, j := range jobs {
		res = append(res, map[string]any{
			"id":                j.ID,
			"name":              j.Name,
			"agent_id":          j.AgentID,
			"cron_expr":         j.CronExpr,
			"prompt":            j.Prompt,
			"target_channel":    j.TargetChannel,
			"target_account_id": j.TargetAccountID,
			"target_recipient":  j.TargetRecipient,
			"enabled":           j.Enabled,
			"next_run":          j.NextRun,
			"last_run":          j.LastRun,
		})
	}
	return res
}

// BuildCronExecutionPrompt formats the autonomous trigger prompt so the LLM knows it is generating a notification FOR the user.
func BuildCronExecutionPrompt(job *CronJob) string {
	return fmt.Sprintf(`[AUTONOMOUS PROACTIVE NOTIFICATION TRIGGER]
Task Name: %s
Task Directive / Reminder Content: %s
Push Channel: %s

YOU ARE EXECUTING AN AUTOMATED NOTIFICATION / REPORT TO YOUR OWNER.
CRITICAL INSTRUCTIONS:
1. Output the complete, actual message/content directly as your final response text.
2. DO NOT invoke 'native_channel_notify' tool because the ActonOS Cron Engine will automatically push your response text directly to the target channel (%s).
3. DO NOT output meta-commentary, conversational filler, or status updates like "The notification has been sent via Telegram" or "Dispatched successfully". Simply output the actual content itself.
4. Address the owner respectfully and naturally in their language (Vietnamese if applicable).`, job.Name, job.Prompt, job.TargetChannel, job.TargetChannel)
}

// CronExecutionRecord represents a past execution of a cron task.
type CronExecutionRecord struct {
	ID         string    `json:"id"`
	JobID      string    `json:"job_id"`
	AgentID    string    `json:"agent_id"`
	Status     string    `json:"status"` // "success", "failed"
	Prompt     string    `json:"prompt"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	TokensUsed int       `json:"tokens_used"`
	ExecutedAt time.Time `json:"executed_at"`
}

func (cs *CronScheduler) recordExecution(rec CronExecutionRecord) {
	if cs.db == nil {
		return
	}
	if rec.ID == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		rec.ID = fmt.Sprintf("ceh_%d_%x", time.Now().UnixNano(), b)
	}
	_, err := cs.db.Exec(`
		INSERT OR REPLACE INTO cron_execution_history (id, job_id, agent_id, status, prompt, output, error, duration_ms, tokens_used, executed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rec.ID, rec.JobID, rec.AgentID, rec.Status, rec.Prompt, rec.Output, rec.Error, rec.DurationMS, rec.TokensUsed, rec.ExecutedAt)
	if err != nil {
		slog.Warn("failed to record cron execution in sqlite", "error", err, "job_id", rec.JobID)
	}
}

// GetExecutionHistory returns past execution runs for a specific cron job.
func (cs *CronScheduler) GetExecutionHistory(jobID string, limit int) ([]CronExecutionRecord, error) {
	if cs.db == nil {
		return []CronExecutionRecord{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := cs.db.Query(`
		SELECT id, job_id, agent_id, status, prompt, output, error, duration_ms, tokens_used, executed_at
		FROM cron_execution_history
		WHERE job_id = ?
		ORDER BY executed_at DESC
		LIMIT ?
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CronExecutionRecord
	for rows.Next() {
		var r CronExecutionRecord
		var out, errStr sql.NullString
		if err := rows.Scan(&r.ID, &r.JobID, &r.AgentID, &r.Status, &r.Prompt, &out, &errStr, &r.DurationMS, &r.TokensUsed, &r.ExecutedAt); err == nil {
			r.Output = out.String
			r.Error = errStr.String
			records = append(records, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// ListAllExecutionHistory returns recent execution history across all cron jobs.
func (cs *CronScheduler) ListAllExecutionHistory(limit int) ([]CronExecutionRecord, error) {
	if cs.db == nil {
		return []CronExecutionRecord{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := cs.db.Query(`
		SELECT id, job_id, agent_id, status, prompt, output, error, duration_ms, tokens_used, executed_at
		FROM cron_execution_history
		ORDER BY executed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CronExecutionRecord
	for rows.Next() {
		var r CronExecutionRecord
		var out, errStr sql.NullString
		if err := rows.Scan(&r.ID, &r.JobID, &r.AgentID, &r.Status, &r.Prompt, &out, &errStr, &r.DurationMS, &r.TokensUsed, &r.ExecutedAt); err == nil {
			r.Output = out.String
			r.Error = errStr.String
			records = append(records, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (cs *CronScheduler) executeJob(job *CronJob) {
	slog.Info("executing proactive cron job", "job_id", job.ID, "agent_id", job.AgentID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startTime := time.Now()

	cs.mu.Lock()
	job.LastRun = time.Now().UTC()
	if cs.db != nil {
		_, _ = cs.db.Exec("UPDATE cron_jobs SET last_run = ?, updated_at = ? WHERE id = ?", job.LastRun, job.LastRun, job.ID)
	}
	cs.mu.Unlock()

	var responseContent string
	var tokensUsed int
	var execErr error

	if cs.engine != nil {
		executionPrompt := BuildCronExecutionPrompt(job)
		resp, err := cs.engine.ExecuteStep(ctx, job.AgentID, executionPrompt)
		if err != nil {
			execErr = err
			slog.Error("proactive cron job failed", "job_id", job.ID, "error", err)
		} else if resp != nil {
			responseContent = resp.Content
			tokensUsed = resp.Usage.TotalTokens
		}
	} else {
		responseContent = fmt.Sprintf("Simulated proactive output for %s", job.Name)
	}

	duration := time.Since(startTime).Milliseconds()

	// Record execution in history
	rec := CronExecutionRecord{
		ID:         fmt.Sprintf("ceh_%d_%s", time.Now().UnixNano(), job.ID),
		JobID:      job.ID,
		AgentID:    job.AgentID,
		Prompt:     job.Prompt,
		Output:     responseContent,
		DurationMS: duration,
		TokensUsed: tokensUsed,
		ExecutedAt: job.LastRun,
	}
	if execErr != nil {
		rec.Status = "failed"
		rec.Error = execErr.Error()
	} else {
		rec.Status = "success"
	}
	cs.recordExecution(rec)

	// Suppress redundant tool delivery if LLM just returned a tool dispatch status report
	isRedundantToolReport := strings.HasPrefix(responseContent, "The notification has been successfully sent") ||
		strings.HasPrefix(responseContent, "Successfully dispatched proactive notification") ||
		strings.HasPrefix(responseContent, "Notification sent")

	// Publish Proactive Outbound Notification Event to EventBus if not suppressed and target configured
	if cs.eventBus != nil && responseContent != "" && !isRedundantToolReport && job.TargetChannel != "none" && job.TargetChannel != "" {
		cs.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, job.AgentID, map[string]any{
			"type":              "proactive_cron_notification",
			"job_id":            job.ID,
			"job_name":          job.Name,
			"target_channel":    job.TargetChannel,
			"target_account_id": job.TargetAccountID,
			"target_recipient":  job.TargetRecipient,
			"content":           responseContent,
			"timestamp":         time.Now().UTC(),
		}))
	}
}

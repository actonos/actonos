package agent

import (
	"context"
	"fmt"
	"log/slog"
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
	TargetChannel   string    `json:"target_channel"` // "telegram", "discord", "whatsapp", "webhook"
	TargetRecipient string    `json:"target_recipient"`
	Enabled         bool      `json:"enabled"`
	LastRun         time.Time `json:"last_run,omitempty"`
	NextRun         time.Time `json:"next_run,omitempty"`
	entryID         cron.EntryID
}

// CronScheduler orchestrates background cron-triggered agent executions and proactive push.
type CronScheduler struct {
	mu       sync.RWMutex
	cron     *cron.Cron
	engine   *Engine
	eventBus *bus.EventBus
	jobs     map[string]*CronJob
}

// NewCronScheduler creates a CronScheduler instance.
func NewCronScheduler(engine *Engine, eventBus *bus.EventBus) *CronScheduler {
	return &CronScheduler{
		cron:     cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor))),
		engine:   engine,
		eventBus: eventBus,
		jobs:     make(map[string]*CronJob),
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

// RegisterJob adds or updates a scheduled proactive agent job.
func (cs *CronScheduler) RegisterJob(job CronJob) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// If job already exists, remove previous cron entry
	if existing, exists := cs.jobs[job.ID]; exists && existing.entryID != 0 {
		cs.cron.Remove(existing.entryID)
	}

	if !job.Enabled {
		cs.jobs[job.ID] = &job
		return nil
	}

	jobCopy := job
	entryID, err := cs.cron.AddFunc(job.CronExpr, func() {
		cs.executeJob(&jobCopy)
	})
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", job.CronExpr, err)
	}

	jobCopy.entryID = entryID
	entry := cs.cron.Entry(entryID)
	jobCopy.NextRun = entry.Next

	cs.jobs[jobCopy.ID] = &jobCopy
	slog.Info("registered proactive cron job", "id", jobCopy.ID, "agent", jobCopy.AgentID, "cron", jobCopy.CronExpr)
	return nil
}

// RemoveJob removes a scheduled job.
func (cs *CronScheduler) RemoveJob(jobID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if job, exists := cs.jobs[jobID]; exists {
		if job.entryID != 0 {
			cs.cron.Remove(job.entryID)
		}
		delete(cs.jobs, jobID)
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

func (cs *CronScheduler) executeJob(job *CronJob) {
	slog.Info("executing proactive cron job", "job_id", job.ID, "agent_id", job.AgentID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cs.mu.Lock()
	job.LastRun = time.Now().UTC()
	cs.mu.Unlock()

	var responseContent string
	if cs.engine != nil {
		resp, err := cs.engine.ExecuteStep(ctx, job.AgentID, job.Prompt)
		if err != nil {
			slog.Error("proactive cron job failed", "job_id", job.ID, "error", err)
			return
		}
		if resp != nil {
			responseContent = resp.Content
		}
	} else {
		responseContent = fmt.Sprintf("Simulated proactive output for %s", job.Name)
	}

	// Publish Proactive Outbound Notification Event to EventBus
	if cs.eventBus != nil && responseContent != "" {
		cs.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, job.AgentID, map[string]any{
			"type":             "proactive_cron_notification",
			"job_id":           job.ID,
			"job_name":         job.Name,
			"target_channel":   job.TargetChannel,
			"target_recipient": job.TargetRecipient,
			"content":          responseContent,
			"timestamp":        time.Now().UTC(),
		}))
	}
}

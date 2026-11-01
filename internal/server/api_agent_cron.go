package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/channels"
	"github.com/go-chi/chi/v5"
)

// Proactive Cron Job Handlers
func (s *Server) handleListCronJobs(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"jobs": []any{}, "count": 0})
		return
	}

	jobs := s.cronSched.ListJobs()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"jobs":  jobs,
		"count": len(jobs),
	})
}

func (s *Server) handleSaveCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondError(w, http.StatusNotImplemented, "CRON_NOT_AVAILABLE", "cron scheduler not configured")
		return
	}

	var req struct {
		ID              string `json:"id"`
		AgentID         string `json:"agent_id"`
		Name            string `json:"name"`
		CronExpr        string `json:"cron_expr"`
		Prompt          string `json:"prompt"`
		TargetChannel   string `json:"target_channel"`
		Channel         string `json:"channel"`
		TargetRecipient string `json:"target_recipient"`
		Recipient       string `json:"recipient"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	targetChan := req.TargetChannel
	if targetChan == "" {
		targetChan = req.Channel
	}
	if targetChan == "" {
		targetChan = "telegram"
	}

	targetRec := req.TargetRecipient
	if targetRec == "" {
		targetRec = req.Recipient
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	job := agent.CronJob{
		ID:              req.ID,
		AgentID:         req.AgentID,
		Name:            req.Name,
		CronExpr:        req.CronExpr,
		Prompt:          req.Prompt,
		TargetChannel:   targetChan,
		TargetRecipient: targetRec,
		Enabled:         enabled,
	}

	if job.ID == "" {
		job.ID = fmt.Sprintf("job_%d", time.Now().Unix())
	}
	if job.Name == "" {
		job.Name = job.ID
	}
	if job.AgentID == "" {
		job.AgentID = "agent_system_core"
	}

	if err := s.cronSched.RegisterJob(job); err != nil {
		s.respondError(w, http.StatusBadRequest, "SCHEDULE_JOB_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "scheduled",
		"job":    job,
	})
}

func (s *Server) handleDeleteCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondError(w, http.StatusNotImplemented, "CRON_NOT_AVAILABLE", "cron scheduler not configured")
		return
	}

	jobID := chi.URLParam(r, "id")
	s.cronSched.RemoveJob(jobID)
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "job_id": jobID})
}

func (s *Server) handleRunCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondError(w, http.StatusNotImplemented, "CRON_NOT_AVAILABLE", "cron scheduler not configured")
		return
	}

	jobID := chi.URLParam(r, "id")
	jobs := s.cronSched.ListJobs()
	var targetJob *agent.CronJob
	for _, j := range jobs {
		if j.ID == jobID {
			targetJob = &j
			break
		}
	}

	if targetJob == nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "cron job not found")
		return
	}

	// Trigger agent execution in background with proactive notification framing
	go func(job agent.CronJob) {
		agentID := job.AgentID
		if agentID == "" || agentID == "default" {
			agentID = "agent_system_core"
		}
		executionPrompt := agent.BuildCronExecutionPrompt(&job)
		resp, err := s.engine.ExecuteStep(context.Background(), agentID, executionPrompt)
		if err != nil {
			return
		}

		targetChan := job.TargetChannel
		if targetChan == "" {
			targetChan = "all"
		}

		if s.channelMgr != nil {
			_ = s.channelMgr.SendMessage(context.Background(), channels.OutboundMessage{
				ChannelID: targetChan,
				Recipient: job.TargetRecipient,
				Content:   fmt.Sprintf("⏰ **[Cron Reminder: %s]**\n\n%s", job.Name, resp.Content),
			})
		}
	}(*targetJob)

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "triggered",
		"message": fmt.Sprintf("Cron task '%s' triggered successfully", targetJob.Name),
	})
}

func (s *Server) handleListAllCronHistory(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondJSON(w, http.StatusOK, []agent.CronExecutionRecord{})
		return
	}
	history, err := s.cronSched.ListAllExecutionHistory(50)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if history == nil {
		history = []agent.CronExecutionRecord{}
	}
	s.respondJSON(w, http.StatusOK, history)
}

func (s *Server) handleGetCronJobHistory(w http.ResponseWriter, r *http.Request) {
	if s.cronSched == nil {
		s.respondJSON(w, http.StatusOK, []agent.CronExecutionRecord{})
		return
	}
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_ID", "job id is required")
		return
	}
	history, err := s.cronSched.GetExecutionHistory(jobID, 50)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if history == nil {
		history = []agent.CronExecutionRecord{}
	}
	s.respondJSON(w, http.StatusOK, history)
}

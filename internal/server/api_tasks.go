package server

import (
	"context"
	"net/http"

	"github.com/actonos/actonos/internal/agent"
	"github.com/go-chi/chi/v5"
)

// GET /api/tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"tasks": []agent.AutonomousTask{},
			"count": 0,
		})
		return
	}

	status := r.URL.Query().Get("status")
	priority := r.URL.Query().Get("priority")

	tasks, err := s.taskMgr.ListTasks(r.Context(), status, priority)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TASK_QUERY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// POST /api/tasks
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "TASK_MGR_UNAVAILABLE", "task manager is not initialized")
		return
	}

	var req agent.AutonomousTask
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Title == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_TITLE", "task title is required")
		return
	}

	created, err := s.taskMgr.CreateTask(r.Context(), req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TASK_CREATE_FAILED", err.Error())
		return
	}

	if s.heartbeat != nil {
		s.heartbeat.TriggerWakeup()
	}

	s.respondJSON(w, http.StatusCreated, created)
}

// GET /api/tasks/{id}
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "TASK_MGR_UNAVAILABLE", "task manager is not initialized")
		return
	}

	id := chi.URLParam(r, "id")
	task, err := s.taskMgr.GetTask(r.Context(), id)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
		return
	}

	s.respondJSON(w, http.StatusOK, task)
}

// PUT /api/tasks/{id}
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "TASK_MGR_UNAVAILABLE", "task manager is not initialized")
		return
	}

	id := chi.URLParam(r, "id")
	var req agent.AutonomousTask
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	req.ID = id

	if err := s.taskMgr.UpdateTask(r.Context(), req); err != nil {
		s.respondError(w, http.StatusInternalServerError, "TASK_UPDATE_FAILED", err.Error())
		return
	}

	if s.heartbeat != nil {
		s.heartbeat.TriggerWakeup()
	}

	updated, _ := s.taskMgr.GetTask(r.Context(), id)
	s.respondJSON(w, http.StatusOK, updated)
}

// DELETE /api/tasks/{id}
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "TASK_MGR_UNAVAILABLE", "task manager is not initialized")
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.taskMgr.DeleteTask(r.Context(), id); err != nil {
		s.respondError(w, http.StatusInternalServerError, "TASK_DELETE_FAILED", err.Error())
		return
	}

	if s.heartbeat != nil {
		s.heartbeat.TriggerWakeup()
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// GET /api/heartbeat/config
func (s *Server) handleGetHeartbeatConfig(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondJSON(w, http.StatusOK, agent.HeartbeatConfig{
			Enabled:         true,
			IntervalMinutes: 5,
			Directives:      "Autonomous standing supervisor.",
			TargetChannel:   "all",
			TargetAccountID: "all",
			AutoDelegate:    true,
			ZeroNoise:       true,
		})
		return
	}

	cfg, err := s.taskMgr.GetHeartbeatConfig(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CONFIG_QUERY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, cfg)
}

// PUT /api/heartbeat/config
func (s *Server) handleSaveHeartbeatConfig(w http.ResponseWriter, r *http.Request) {
	if s.taskMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "TASK_MGR_UNAVAILABLE", "task manager is not initialized")
		return
	}

	var req agent.HeartbeatConfig
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.taskMgr.SaveHeartbeatConfig(r.Context(), req); err != nil {
		s.respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
		return
	}

	if s.heartbeat != nil {
		s.heartbeat.SyncConfig(req)
	}

	s.respondJSON(w, http.StatusOK, req)
}

// POST /api/heartbeat/trigger (Immediate manual cognitive pulse)
func (s *Server) handleTriggerHeartbeatPulse(w http.ResponseWriter, r *http.Request) {
	if s.heartbeat == nil {
		s.respondError(w, http.StatusServiceUnavailable, "HEARTBEAT_UNAVAILABLE", "heartbeat daemon is not running")
		return
	}

	go func() {
		_, _ = s.heartbeat.TriggerManualPulse(context.Background())
	}()

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "triggered",
		"message": "Heartbeat pulse cycle initiated in background",
	})
}

// GET /api/heartbeat/runs
func (s *Server) handleListHeartbeatRuns(w http.ResponseWriter, r *http.Request) {
	if s.heartbeat == nil {
		s.respondJSON(w, http.StatusOK, []agent.HeartbeatRun{})
		return
	}

	runs, err := s.heartbeat.GetRecentRuns(r.Context(), 30)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "HEARTBEAT_RUNS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, runs)
}

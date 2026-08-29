package server

import (
	"net/http"
	"strconv"

	"github.com/actonos/actonos/internal/agent"
	"github.com/go-chi/chi/v5"
)

// GET /api/ops/anomalies
func (s *Server) handleListAnomalies(w http.ResponseWriter, r *http.Request) {
	if s.proactiveEngine == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"anomalies": []agent.SystemAnomaly{},
			"count":     0,
		})
		return
	}

	status := r.URL.Query().Get("status")
	severity := r.URL.Query().Get("severity")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	items, err := s.proactiveEngine.ListAnomalies(r.Context(), status, severity, limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "ANOMALY_QUERY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"anomalies": items,
		"count":     len(items),
	})
}

// POST /api/ops/anomalies/{id}/act
func (s *Server) handleActOnAnomaly(w http.ResponseWriter, r *http.Request) {
	if s.proactiveEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "PROACTIVE_UNAVAILABLE", "proactive engine is not initialized")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Action string `json:"action"` // "auto_task", "resolve", "ignore"
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	createdTask, err := s.proactiveEngine.ActOnAnomaly(r.Context(), id, req.Action)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "ACT_ANOMALY_FAILED", err.Error())
		return
	}

	if s.heartbeat != nil && createdTask != nil {
		s.heartbeat.TriggerWakeup()
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":       "action_applied",
		"action":       req.Action,
		"created_task": createdTask,
	})
}

// POST /api/ops/anomalies/scan
func (s *Server) handleTriggerAnomalyScan(w http.ResponseWriter, r *http.Request) {
	if s.proactiveEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "PROACTIVE_UNAVAILABLE", "proactive engine is not initialized")
		return
	}

	items, err := s.proactiveEngine.Scan(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "SCAN_FAILED", err.Error())
		return
	}

	if s.heartbeat != nil {
		s.heartbeat.TriggerWakeup()
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "scan_completed",
		"anomalies": items,
		"count":     len(items),
	})
}

// GET /api/ops/anomalies/config
func (s *Server) handleGetProactiveConfig(w http.ResponseWriter, r *http.Request) {
	if s.proactiveEngine == nil {
		s.respondJSON(w, http.StatusOK, agent.ProactiveConfig{
			Enabled:              true,
			ScanIntervalMinutes:  15,
			AutoCreateTasks:      false,
			DiskThresholdPercent: 80.0,
		})
		return
	}

	cfg, err := s.proactiveEngine.GetConfig(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CONFIG_QUERY_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, cfg)
}

// PUT /api/ops/anomalies/config
func (s *Server) handleUpdateProactiveConfig(w http.ResponseWriter, r *http.Request) {
	if s.proactiveEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "PROACTIVE_UNAVAILABLE", "proactive engine is not initialized")
		return
	}

	var req agent.ProactiveConfig
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.proactiveEngine.SaveConfig(r.Context(), req); err != nil {
		s.respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, req)
}

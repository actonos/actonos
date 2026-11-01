package server

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
)

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	metrics, err := s.hal.GetMetrics(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "METRICS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleGetTailscale(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"enabled":   false,
		})
		return
	}

	status, err := s.tailscale.GetStatus(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TAILSCALE_STATUS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleWifiScan(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	networks, err := s.hal.ScanWifi(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "WIFI_SCAN_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"networks": networks,
		"count":    len(networks),
	})
}

func (s *Server) handleWifiConnect(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.SSID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "ssid is required")
		return
	}

	if err := s.hal.ConnectWifi(r.Context(), req.SSID, req.Password); err != nil {
		s.respondError(w, http.StatusBadRequest, "WIFI_CONNECT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "connected",
		"ssid":   req.SSID,
	})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	approval, err := s.requestAdminAction(r.Context(), "system_restart", map[string]any{})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

// ==============================================================================
// Production LLM Provider Management & Real Latency Testing
// ==============================================================================

func (s *Server) handleGetTokenUsage(w http.ResponseWriter, r *http.Request) {
	if s.tokenTracker == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"total_tokens":   0,
			"total_cost_usd": 0.0,
			"today_tokens":   0,
			"today_cost_usd": 0.0,
			"month_tokens":   0,
			"month_cost_usd": 0.0,
			"by_model":       []any{},
			"by_agent":       []any{},
			"daily_trend":    []any{},
		})
		return
	}

	summary, err := s.tokenTracker.GetSummary(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TOKEN_QUERY_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, summary)
}

func (s *Server) handleGetTokenHistory(w http.ResponseWriter, r *http.Request) {
	if s.tokenTracker == nil {
		s.respondJSON(w, http.StatusOK, []any{})
		return
	}

	limit := 50
	agentID := r.URL.Query().Get("agent_id")
	source := r.URL.Query().Get("source")

	records, err := s.tokenTracker.GetHistory(r.Context(), limit, agentID, source)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TOKEN_HISTORY_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, records)
}

func (s *Server) handleGetHeartbeatHistory(w http.ResponseWriter, r *http.Request) {
	if s.heartbeat == nil {
		s.respondJSON(w, http.StatusOK, []any{})
		return
	}

	runs, err := s.heartbeat.GetRecentRuns(r.Context(), 30)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "HEARTBEAT_QUERY_FAILED", err.Error())
		return
	}
	if runs == nil {
		runs = []agent.HeartbeatRun{}
	}
	s.respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(s.startTime).Seconds()
	activeAgents := 0
	if s.agentMgr != nil {
		agents, _ := s.agentMgr.List(r.Context())
		for _, a := range agents {
			if a.Status == agent.StatusActive {
				activeAgents++
			}
		}
	}

	var totalTokens int64
	var totalCost float64
	if s.tokenTracker != nil {
		if sum, err := s.tokenTracker.GetSummary(r.Context()); err == nil {
			totalTokens = sum.TotalTokens
			totalCost = sum.TotalCostUSD
		}
	}

	var out strings.Builder
	out.WriteString("# HELP actonos_uptime_seconds Total runtime uptime in seconds\n")
	out.WriteString("# TYPE actonos_uptime_seconds counter\n")
	out.WriteString(fmt.Sprintf("actonos_uptime_seconds %f\n", uptime))

	out.WriteString("# HELP actonos_goroutines Current active goroutines\n")
	out.WriteString("# TYPE actonos_goroutines gauge\n")
	out.WriteString(fmt.Sprintf("actonos_goroutines %d\n", runtime.NumGoroutine()))

	out.WriteString("# HELP actonos_memory_alloc_bytes Memory currently allocated\n")
	out.WriteString("# TYPE actonos_memory_alloc_bytes gauge\n")
	out.WriteString(fmt.Sprintf("actonos_memory_alloc_bytes %d\n", m.Alloc))

	out.WriteString("# HELP actonos_agents_active Number of active running agents\n")
	out.WriteString("# TYPE actonos_agents_active gauge\n")
	out.WriteString(fmt.Sprintf("actonos_agents_active %d\n", activeAgents))

	out.WriteString("# HELP actonos_tokens_total Total LLM tokens consumed\n")
	out.WriteString("# TYPE actonos_tokens_total counter\n")
	out.WriteString(fmt.Sprintf("actonos_tokens_total %d\n", totalTokens))

	out.WriteString("# HELP actonos_cost_usd_total Total estimated LLM cost in USD\n")
	out.WriteString("# TYPE actonos_cost_usd_total counter\n")
	out.WriteString(fmt.Sprintf("actonos_cost_usd_total %f\n", totalCost))

	var droppedEvents uint64
	if s.bus != nil {
		droppedEvents = s.bus.DroppedEvents()
	}
	out.WriteString("# HELP actonos_eventbus_dropped_total Subscriber event deliveries dropped due to backpressure\n")
	out.WriteString("# TYPE actonos_eventbus_dropped_total counter\n")
	out.WriteString(fmt.Sprintf("actonos_eventbus_dropped_total %d\n", droppedEvents))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out.String()))
}

// ==============================================================================
// Identity & User Profile Handlers
// ==============================================================================

func (s *Server) handleGetIdentity(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"user_name":           "Operator",
			"user_role":           "System Administrator & Architect",
			"language":            "en",
			"timezone":            "Asia/Ho_Chi_Minh",
			"communication_style": "adaptive, natural, empathetic & sharp",
			"bio":                 "Owner of the ActonOS local intelligence kernel.",
			"custom_instructions": "Provide intelligent, natural, and empathetic responses. Act as a trusted senior engineering partner. Proactively solve problems and avoid robotic or stiff clichés.",
			"soul":                "",
		})
		return
	}

	profile := s.profileMgr.GetProfile()
	soul := s.profileMgr.GetSoul()

	s.respondJSON(w, http.StatusOK, map[string]any{
		"user_name":           profile.UserName,
		"user_role":           profile.UserRole,
		"language":            profile.Language,
		"timezone":            profile.Timezone,
		"communication_style": profile.CommunicationStyle,
		"bio":                 profile.Bio,
		"custom_instructions": profile.CustomInstructions,
		"soul":                soul,
		"updated_at":          profile.UpdatedAt,
	})
}

func (s *Server) handleSaveIdentity(w http.ResponseWriter, r *http.Request) {
	if s.profileMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "PROFILE_MANAGER_UNAVAILABLE", "profile manager not initialized")
		return
	}

	var req struct {
		UserName           string `json:"user_name"`
		UserRole           string `json:"user_role"`
		Language           string `json:"language"`
		Timezone           string `json:"timezone"`
		CommunicationStyle string `json:"communication_style"`
		Bio                string `json:"bio"`
		CustomInstructions string `json:"custom_instructions"`
		Soul               string `json:"soul"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	current := s.profileMgr.GetProfile()
	if req.UserName != "" {
		current.UserName = req.UserName
	}
	if req.UserRole != "" {
		current.UserRole = req.UserRole
	}
	if req.Language != "" {
		current.Language = req.Language
	}
	if req.Timezone != "" {
		current.Timezone = req.Timezone
	}
	if req.CommunicationStyle != "" {
		current.CommunicationStyle = req.CommunicationStyle
	}
	if req.Bio != "" {
		current.Bio = req.Bio
	}
	if req.CustomInstructions != "" {
		current.CustomInstructions = req.CustomInstructions
	}

	if err := s.profileMgr.UpdateProfile(r.Context(), current); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_PROFILE_FAILED", err.Error())
		return
	}

	if req.Soul != "" {
		_ = s.profileMgr.SaveSoul(r.Context(), req.Soul)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":     "saved",
		"profile":    current,
		"updated_at": current.UpdatedAt,
	})
}

func (s *Server) handleGetModelsCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := llm.GetCatalogResponse()
	s.respondJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleGetLLMHealth(w http.ResponseWriter, r *http.Request) {
	if s.llmRouter == nil {
		s.respondError(w, http.StatusServiceUnavailable, "LLM_ROUTER_UNAVAILABLE", "llm router is not initialized")
		return
	}
	reports := s.llmRouter.GetHealthReport()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"providers": reports,
		"count":     len(reports),
	})
}

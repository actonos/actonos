package server

import (
	"context"
	"net/http"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/system"
)

// HealthReport is the liveness/readiness payload.
type HealthReport struct {
	Status            string            `json:"status"`
	Version           string            `json:"version"`
	GitCommit         string            `json:"git_commit"`
	BuildTime         string            `json:"build_time"`
	UptimeSeconds     uint64            `json:"uptime_seconds"`
	RuntimeMode       string            `json:"runtime_mode"`
	AgentsActive      int               `json:"agents_active"`
	TailscaleConnected bool             `json:"tailscale_connected"`
	TailscaleIP       string            `json:"tailscale_ip"`
	Components        map[string]string `json:"components"`
}

func (s *Server) evaluateHealth(ctx context.Context) HealthReport {
	runtimeMode := "docker"
	if s.hal != nil {
		runtimeMode = s.hal.RuntimeMode()
	}
	activeAgents := 0
	if s.agentMgr != nil {
		agents, _ := s.agentMgr.List(ctx)
		for _, a := range agents {
			if a.Status == agent.StatusActive {
				activeAgents++
			}
		}
	}
	tailscaleConnected := false
	tailscaleIP := ""
	if s.tailscale != nil {
		st, _ := s.tailscale.GetStatus(ctx)
		if st != nil {
			tailscaleConnected = st.Connected
			tailscaleIP = st.IP
		}
	}

	components := map[string]string{}
	status := "healthy"

	if s.llmRouter == nil || !s.llmRouter.HasRealProvider() {
		components["llm"] = "unhealthy"
		status = "degraded"
	} else {
		components["llm"] = "healthy"
	}

	if s.heartbeat != nil {
		last := s.heartbeat.LastRunAt()
		interval := s.heartbeat.Interval()
		if interval <= 0 {
			interval = 5 * time.Minute
		}
		if !last.IsZero() && time.Since(last) > 2*interval {
			components["heartbeat"] = "degraded"
			if status == "healthy" {
				status = "degraded"
			}
		} else {
			components["heartbeat"] = "healthy"
		}
	} else {
		components["heartbeat"] = "stopped"
	}

	if s.embedding != nil {
		st, err := s.embedding.Status(ctx)
		if err != nil || !st.ServiceReady || st.Dead > 0 {
			components["embedding"] = "degraded"
			if status == "healthy" {
				status = "degraded"
			}
		} else {
			components["embedding"] = "healthy"
		}
	}

	if system.WritesFrozen(s.dataDir) {
		components["disk"] = "unhealthy"
		status = "unhealthy"
	} else {
		components["disk"] = "healthy"
	}

	return HealthReport{
		Status:             status,
		Version:            s.version,
		GitCommit:          s.gitCommit,
		BuildTime:          s.buildTime,
		UptimeSeconds:      uint64(time.Since(s.startTime).Seconds()),
		RuntimeMode:        runtimeMode,
		AgentsActive:       activeAgents,
		TailscaleConnected: tailscaleConnected,
		TailscaleIP:        tailscaleIP,
		Components:         components,
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	report := s.evaluateHealth(r.Context())
	code := http.StatusOK
	if report.Status == "unhealthy" {
		code = http.StatusServiceUnavailable
	}
	s.respondJSON(w, code, report)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	report := s.evaluateHealth(r.Context())
	code := http.StatusOK
	if report.Status != "healthy" {
		code = http.StatusServiceUnavailable
	}
	s.respondJSON(w, code, report)
}

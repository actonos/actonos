package server

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/actonos/actonos/internal/system"
)

// DashboardSummary represents the live aggregated statistics payload.
type DashboardSummary struct {
	Metrics      *system.SystemMetrics  `json:"metrics"`
	Tailscale    *system.TailscaleStatus `json:"tailscale"`
	AgentsCount  int                    `json:"agents_count"`
	AgentsActive int                    `json:"agents_active"`
	ToolsCount   int                    `json:"tools_count"`
	ToolsNative  int                    `json:"tools_native"`
	ToolsMCP     int                    `json:"tools_mcp"`
	ToolsSkills  int                    `json:"tools_skills"`
	ToolsWASM    int                    `json:"tools_wasm"`
	CronCount    int                    `json:"cron_count"`
	Storage      map[string]int64       `json:"storage"`
	RecentAudit  []AuditLogResponse     `json:"recent_audit"`
	Timestamp    time.Time              `json:"timestamp"`
}

type AuditLogResponse struct {
	Timestamp       string `json:"timestamp"`
	TraceID         string `json:"trace_id"`
	AgentID         string `json:"agent_id"`
	ToolName        string `json:"tool_name"`
	RiskLevel       string `json:"risk_level"`
	ExecutionTimeMs int    `json:"execution_time_ms"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
}

func (s *Server) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 1. Hardware metrics
	var metrics *system.SystemMetrics
	if s.hal != nil {
		m, err := s.hal.GetMetrics(ctx)
		if err == nil {
			metrics = m
		}
	}

	// 2. Tailscale node status
	var tsNode *system.TailscaleStatus
	if s.tailscale != nil {
		ts, err := s.tailscale.GetStatus(ctx)
		if err == nil {
			tsNode = ts
		}
	}

	// 3. Agent statistics
	var agentsCount, agentsActive int
	if s.agentMgr != nil {
		agentsList, err := s.agentMgr.List(ctx)
		if err == nil {
			agentsCount = len(agentsList)
			for _, ag := range agentsList {
				if ag.Status == "active" {
					agentsActive++
				}
			}
		}
	}

	// 4. Tools statistics
	var toolsCount, toolsNative, toolsMCP, toolsSkills, toolsWASM int
	if s.toolReg != nil {
		toolsList := s.toolReg.List()
		toolsCount = len(toolsList)
		for _, t := range toolsList {
			switch t.Category {
			case "native":
				toolsNative++
			case "mcp":
				toolsMCP++
			case "skill":
				toolsSkills++
			case "wasm":
				toolsWASM++
			}
		}
	}

	// 5. Cron tasks count
	var cronCount int
	if s.cronSched != nil {
		cronCount = len(s.cronSched.ListJobs())
	}

	// 6. Storage usage breakdown
	dataDir := "./data"
	storageSize := getDirSize(filepath.Join(dataDir, "storage"))
	vectorsSize := getDirSize(filepath.Join(dataDir, "vectors"))
	workspaceSize := getDirSize(filepath.Join(dataDir, "workspace"))
	logsSize := getDirSize(filepath.Join(dataDir, "logs"))

	storageMap := map[string]int64{
		"storage_bytes":   storageSize,
		"vectors_bytes":   vectorsSize,
		"workspace_bytes": workspaceSize,
		"logs_bytes":      logsSize,
		"total_bytes":     storageSize + vectorsSize + workspaceSize + logsSize,
	}

	// 7. Recent audit logs from file
	var recentLogs []AuditLogResponse
	if auditLogger, err := system.NewAuditLogger(dataDir); err == nil {
		defer auditLogger.Close()
		entries, err := auditLogger.ReadRecentEntries(5)
		if err == nil {
			for _, e := range entries {
				recentLogs = append(recentLogs, AuditLogResponse{
					Timestamp:       e.Timestamp,
					TraceID:         e.TraceID,
					AgentID:         e.AgentID,
					ToolName:        e.ToolName,
					RiskLevel:       e.RiskLevel,
					ExecutionTimeMs: int(e.ExecutionTimeMS),
					Status:          e.Status,
					Error:           e.Error,
				})
			}
		}
	}

	summary := DashboardSummary{
		Metrics:      metrics,
		Tailscale:    tsNode,
		AgentsCount:  agentsCount,
		AgentsActive: agentsActive,
		ToolsCount:   toolsCount,
		ToolsNative:  toolsNative,
		ToolsMCP:     toolsMCP,
		ToolsSkills:  toolsSkills,
		ToolsWASM:    toolsWASM,
		CronCount:    cronCount,
		Storage:      storageMap,
		RecentAudit:  recentLogs,
		Timestamp:    time.Now().UTC(),
	}

	s.respondJSON(w, http.StatusOK, summary)
}

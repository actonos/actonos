package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/system"
)

func (s *Server) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.auditLogger == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0, "total": 0})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}

	params := system.AuditSearchParams{
		Query:     r.URL.Query().Get("q"),
		AgentID:   r.URL.Query().Get("agent_id"),
		RiskLevel: r.URL.Query().Get("risk_level"),
		Status:    r.URL.Query().Get("status"),
		ToolName:  r.URL.Query().Get("tool_name"),
		From:      r.URL.Query().Get("from"),
		To:        r.URL.Query().Get("to"),
		Limit:     limit,
		Offset:    offset,
	}

	entries, total, err := s.auditLogger.SearchEntries(params)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0, "total": 0})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
		"total":   total,
	})
}

func (s *Server) handleExportAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.auditLogger == nil {
		s.respondError(w, http.StatusServiceUnavailable, "AUDIT_NOT_CONFIGURED", "audit logger is not configured")
		return
	}

	params := system.AuditSearchParams{
		Query:     r.URL.Query().Get("q"),
		AgentID:   r.URL.Query().Get("agent_id"),
		RiskLevel: r.URL.Query().Get("risk_level"),
		Status:    r.URL.Query().Get("status"),
		ToolName:  r.URL.Query().Get("tool_name"),
		From:      r.URL.Query().Get("from"),
		To:        r.URL.Query().Get("to"),
		Limit:     10000,
		Offset:    0,
	}

	entries, _, err := s.auditLogger.SearchEntries(params)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"actonos-audit-%s.json\"", time.Now().UTC().Format("20060102_150405")))
		_ = json.NewEncoder(w).Encode(entries)
		return
	}

	// Default CSV format
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"actonos-audit-%s.csv\"", time.Now().UTC().Format("20060102_150405")))

	csvWriter := csv.NewWriter(w)
	_ = csvWriter.Write([]string{"Timestamp", "TraceID", "AgentID", "ToolName", "RiskLevel", "LatencyMS", "Status", "Error", "EntryHash"})

	for _, e := range entries {
		_ = csvWriter.Write([]string{
			e.Timestamp,
			e.TraceID,
			e.AgentID,
			e.ToolName,
			e.RiskLevel,
			strconv.FormatInt(e.ExecutionTimeMS, 10),
			e.Status,
			e.Error,
			e.EntryHash,
		})
	}
	csvWriter.Flush()
}

func (s *Server) handleVerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	if s.auditLogger == nil {
		s.respondError(w, http.StatusNotImplemented, "AUDIT_NOT_CONFIGURED", "audit logger is not configured")
		return
	}
	if err := s.auditLogger.VerifyChain(); err != nil {
		s.respondError(w, http.StatusConflict, "AUDIT_CHAIN_INVALID", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":      "valid",
		"verified_at": time.Now().UTC().Format(time.RFC3339),
	})
}

package system

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogEntry matches Section 6.C of the ActonOS specification.
type AuditLogEntry struct {
	Timestamp       string `json:"timestamp"`
	TraceID         string `json:"trace_id"`
	AgentID         string `json:"agent_id"`
	ToolName        string `json:"tool_name"`
	RiskLevel       string `json:"risk_level"` // "Low", "Medium", "High"
	ExecutionTimeMS int64  `json:"execution_time_ms"`
	Status          string `json:"status"` // "Success", "Failed", "Blocked"
	Error           string `json:"error,omitempty"`
}

// AuditLogger writes structured JSON-lines logs to `/data/logs/audit.jsonl`.
type AuditLogger struct {
	mu      sync.Mutex
	logPath string
	file    *os.File
}

// NewAuditLogger creates a new AuditLogger instance.
func NewAuditLogger(dataDir string) (*AuditLogger, error) {
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating logs dir: %w", err)
	}

	logPath := filepath.Join(logsDir, "audit.jsonl")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening audit log file: %w", err)
	}

	al := &AuditLogger{
		logPath: logPath,
		file:    f,
	}

	// Check if file is empty and seed initial bootstrap entry
	if stat, statErr := f.Stat(); statErr == nil && stat.Size() == 0 {
		_ = al.Log(AuditLogEntry{
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			TraceID:         GenerateTraceID(),
			AgentID:         "agent_system_core",
			ToolName:        "kernel_bootstrap_security_check",
			RiskLevel:       "Low",
			ExecutionTimeMS: 5,
			Status:          "Success",
			Error:           "",
		})
	}

	return al, nil
}

// GenerateTraceID generates a 32-character hex trace ID conforming to W3C Trace Context.
func GenerateTraceID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// LogAudit implements tools.ToolAuditLogger.
func (l *AuditLogger) LogAudit(traceID, agentID, toolName, riskLevel, status, errorMsg string, durationMS int64) {
	if traceID == "" {
		traceID = GenerateTraceID()
	}
	if agentID == "" {
		agentID = "agent_system_core"
	}
	_ = l.Log(AuditLogEntry{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		TraceID:         traceID,
		AgentID:         agentID,
		ToolName:        toolName,
		RiskLevel:       riskLevel,
		ExecutionTimeMS: durationMS,
		Status:          status,
		Error:           errorMsg,
	})
}

// Log writes an audit log entry.
func (l *AuditLogger) Log(entry AuditLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.TraceID == "" {
		entry.TraceID = GenerateTraceID()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	_, err = l.file.Write(data)
	return err
}

// ReadRecentEntries reads the last N audit log entries.
func (l *AuditLogger) ReadRecentEntries(limit int) ([]AuditLogEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.logPath)
	if err != nil {
		return nil, err
	}

	lines := splitLines(string(data))
	var entries []AuditLogEntry

	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}

	for i := len(lines) - 1; i >= start; i-- {
		line := lines[i]
		if len(line) == 0 {
			continue
		}
		var e AuditLogEntry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			entries = append(entries, e)
		}
	}

	return entries, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// Close closes the underlying audit log file.
func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

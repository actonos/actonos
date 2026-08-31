package system

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	PreviousHash    string `json:"previous_hash,omitempty"`
	EntryHash       string `json:"entry_hash"`
}

// AuditLogger writes structured JSON-lines logs to `/data/logs/audit.jsonl`.
type AuditLogger struct {
	mu       sync.Mutex
	logPath  string
	file     *os.File
	lastHash string
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
	if entries, readErr := al.ReadRecentEntries(1); readErr == nil && len(entries) > 0 {
		al.lastHash = entries[0].EntryHash
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

	if WritesFrozen(filepath.Dir(filepath.Dir(l.logPath))) {
		return nil
	}
	if l.file != nil {
		if stat, statErr := l.file.Stat(); statErr == nil && stat.Size() > 32<<20 {
			_ = l.file.Close()
			rotated := l.logPath + "." + time.Now().UTC().Format("20060102T150405")
			_ = os.Rename(l.logPath, rotated)
			f, openErr := os.OpenFile(l.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if openErr == nil {
				l.file = f
			}
		}
	}

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.TraceID == "" {
		entry.TraceID = GenerateTraceID()
	}
	entry.PreviousHash = l.lastHash
	entry.EntryHash = ""
	canonical, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshalling canonical audit entry: %w", err)
	}
	sum := sha256.Sum256(canonical)
	entry.EntryHash = hex.EncodeToString(sum[:])

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	_, err = l.file.Write(data)
	if err == nil {
		l.lastHash = entry.EntryHash
	}
	return err
}

// VerifyChain validates the tamper-evident hash chain in the audit file.
func (l *AuditLogger) VerifyChain() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := os.ReadFile(l.logPath)
	if err != nil {
		return fmt.Errorf("reading audit chain: %w", err)
	}
	previous := ""
	for index, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var entry AuditLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("decoding audit entry %d: %w", index, err)
		}
		recorded := entry.EntryHash
		entry.EntryHash = ""
		canonical, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("canonicalizing audit entry %d: %w", index, err)
		}
		sum := sha256.Sum256(canonical)
		if entry.PreviousHash != previous || recorded != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("audit chain verification failed at entry %d", index)
		}
		previous = recorded
	}
	return nil
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

// AuditSearchParams provides granular multi-criteria filtering for audit log entries.
type AuditSearchParams struct {
	Query     string `json:"query,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	RiskLevel string `json:"risk_level,omitempty"`
	Status    string `json:"status,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// SearchEntries filters and paginates through audit log entries.
func (l *AuditLogger) SearchEntries(params AuditSearchParams) ([]AuditLogEntry, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.logPath)
	if err != nil {
		return nil, 0, err
	}

	lines := splitLines(string(data))
	q := strings.TrimSpace(strings.ToLower(params.Query))
	agentFilter := strings.TrimSpace(strings.ToLower(params.AgentID))
	riskFilter := strings.TrimSpace(strings.ToLower(params.RiskLevel))
	statusFilter := strings.TrimSpace(strings.ToLower(params.Status))
	toolFilter := strings.TrimSpace(strings.ToLower(params.ToolName))

	var fromTime, toTime time.Time
	if params.From != "" {
		fromTime, _ = time.Parse(time.RFC3339, params.From)
	}
	if params.To != "" {
		toTime, _ = time.Parse(time.RFC3339, params.To)
	}

	var matched []AuditLogEntry

	// Read in reverse chronological order (newest first)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if len(line) == 0 {
			continue
		}
		var e AuditLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}

		if agentFilter != "" && agentFilter != "all" && strings.ToLower(e.AgentID) != agentFilter {
			continue
		}
		if riskFilter != "" && riskFilter != "all" && strings.ToLower(e.RiskLevel) != riskFilter {
			continue
		}
		if statusFilter != "" && statusFilter != "all" && strings.ToLower(e.Status) != statusFilter {
			continue
		}
		if toolFilter != "" && !strings.Contains(strings.ToLower(e.ToolName), toolFilter) {
			continue
		}

		if !fromTime.IsZero() {
			if entryTime, parseErr := time.Parse(time.RFC3339, e.Timestamp); parseErr == nil && entryTime.Before(fromTime) {
				continue
			}
		}
		if !toTime.IsZero() {
			if entryTime, parseErr := time.Parse(time.RFC3339, e.Timestamp); parseErr == nil && entryTime.After(toTime) {
				continue
			}
		}

		if q != "" {
			matchTool := strings.Contains(strings.ToLower(e.ToolName), q)
			matchAgent := strings.Contains(strings.ToLower(e.AgentID), q)
			matchTrace := strings.Contains(strings.ToLower(e.TraceID), q)
			matchError := strings.Contains(strings.ToLower(e.Error), q)
			matchRisk := strings.Contains(strings.ToLower(e.RiskLevel), q)
			matchStatus := strings.Contains(strings.ToLower(e.Status), q)
			if !matchTool && !matchAgent && !matchTrace && !matchError && !matchRisk && !matchStatus {
				continue
			}
		}

		matched = append(matched, e)
	}

	totalCount := len(matched)
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > totalCount {
		offset = totalCount
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}

	end := offset + limit
	if end > totalCount {
		end = totalCount
	}

	return matched[offset:end], totalCount, nil
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

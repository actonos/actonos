package system

import (
	"testing"
)

func TestAuditLoggerSearchEntries(t *testing.T) {
	tempDir := t.TempDir()
	al, err := NewAuditLogger(tempDir)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	// Seed multiple audit entries
	al.LogAudit("trace_1", "agent_dev", "native_file_read", "Low", "Success", "", 12)
	al.LogAudit("trace_2", "agent_sec", "native_exec", "High", "Blocked", "Forbidden sudo", 45)
	al.LogAudit("trace_3", "agent_dev", "native_file_write", "Medium", "Success", "", 28)
	al.LogAudit("trace_4", "agent_ops", "native_sysinfo", "Low", "Success", "", 5)

	// Test 1: Search by Query
	res, total, err := al.SearchEntries(AuditSearchParams{
		Query: "sudo",
	})
	if err != nil {
		t.Fatalf("search entries failed: %v", err)
	}
	if total != 1 || len(res) != 1 {
		t.Fatalf("expected 1 result for query 'sudo', got %d (total: %d)", len(res), total)
	}
	if res[0].TraceID != "trace_2" {
		t.Errorf("expected trace_2, got %s", res[0].TraceID)
	}

	// Test 2: Filter by AgentID and RiskLevel
	res, total, err = al.SearchEntries(AuditSearchParams{
		AgentID:   "agent_dev",
		RiskLevel: "Medium",
	})
	if err != nil {
		t.Fatalf("search entries failed: %v", err)
	}
	if total != 1 || len(res) != 1 {
		t.Fatalf("expected 1 result for agent_dev + Medium risk, got %d", len(res))
	}
	if res[0].ToolName != "native_file_write" {
		t.Errorf("expected native_file_write, got %s", res[0].ToolName)
	}

	// Test 3: Pagination
	res, total, err = al.SearchEntries(AuditSearchParams{
		Limit:  2,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("search entries failed: %v", err)
	}
	if total < 4 {
		t.Errorf("expected at least 4 total entries, got %d", total)
	}
	if len(res) != 2 {
		t.Errorf("expected page limit of 2, got %d", len(res))
	}
}

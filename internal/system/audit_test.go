package system

import "testing"

func TestAuditLoggerVerifyChain(t *testing.T) {
	logger, err := NewAuditLogger(t.TempDir())
	if err != nil {
		t.Fatalf("creating audit logger: %v", err)
	}
	defer logger.Close()
	logger.LogAudit("trace-one", "agent", "native_file_read", "Low", "Success", "", 2)
	logger.LogAudit("trace-one", "agent", "native_file_write", "High", "Blocked", "approval required", 1)
	if err := logger.VerifyChain(); err != nil {
		t.Fatalf("valid audit chain rejected: %v", err)
	}
}

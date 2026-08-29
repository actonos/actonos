package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyDirectiveOutcome(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "directive_test_*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// 1. file_exists test
	testFile := filepath.Join(tempDir, "report.md")
	if ok, _, _ := VerifyDirectiveOutcome(ctx, tempDir, "file_exists:report.md"); ok {
		t.Fatal("expected file_exists to fail when file does not exist")
	}

	// Create empty file
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("writing empty file: %v", err)
	}
	if ok, _, _ := VerifyDirectiveOutcome(ctx, tempDir, "file_exists:report.md"); ok {
		t.Fatal("expected file_exists to fail when file is 0 bytes")
	}

	// Write content
	if err := os.WriteFile(testFile, []byte("# SEO Report\nAll metrics nominal."), 0644); err != nil {
		t.Fatalf("writing file content: %v", err)
	}
	if ok, msg, err := VerifyDirectiveOutcome(ctx, tempDir, "file_exists:report.md"); !ok || err != nil {
		t.Fatalf("expected file_exists to pass, got ok=%v, msg=%s, err=%v", ok, msg, err)
	}

	// 2. file_contains test
	if ok, _, _ := VerifyDirectiveOutcome(ctx, tempDir, "file_contains:report.md|Missing Section"); ok {
		t.Fatal("expected file_contains to fail for missing substring")
	}
	if ok, msg, err := VerifyDirectiveOutcome(ctx, tempDir, "file_contains:report.md|SEO Report"); !ok || err != nil {
		t.Fatalf("expected file_contains to pass, got ok=%v, msg=%s, err=%v", ok, msg, err)
	}

	// 3. dir_not_empty test
	subDir := filepath.Join(tempDir, "reports")
	_ = os.MkdirAll(subDir, 0755)
	if ok, _, _ := VerifyDirectiveOutcome(ctx, tempDir, "dir_not_empty:reports"); ok {
		t.Fatal("expected dir_not_empty to fail for empty dir")
	}
	_ = os.WriteFile(filepath.Join(subDir, "item.json"), []byte("{}"), 0644)
	if ok, msg, err := VerifyDirectiveOutcome(ctx, tempDir, "dir_not_empty:reports"); !ok || err != nil {
		t.Fatalf("expected dir_not_empty to pass, got ok=%v, msg=%s, err=%v", ok, msg, err)
	}

	// 4. http_status test
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	if ok, msg, err := VerifyDirectiveOutcome(ctx, tempDir, "http_status:"+srv.URL+"/health|200"); !ok || err != nil {
		t.Fatalf("expected http_status 200 to pass, got ok=%v, msg=%s, err=%v", ok, msg, err)
	}
	if ok, _, _ := VerifyDirectiveOutcome(ctx, tempDir, "http_status:"+srv.URL+"/non-existent|200"); ok {
		t.Fatal("expected http_status to fail when server returns 404")
	}
}

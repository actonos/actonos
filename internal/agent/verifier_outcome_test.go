package agent

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOutcomeVerifier_FileExists(t *testing.T) {
	tempDir := t.TempDir()
	verifier := NewOutcomeVerifier(nil, nil)
	ctx := context.Background()

	// 1. Non-existent file
	res := verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertFileExists,
		Target:   "missing.txt",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected failure for non-existent file")
	}

	// 2. Empty file (0 bytes)
	emptyPath := filepath.Join(tempDir, "empty.txt")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	res = verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertFileExists,
		Target:   "empty.txt",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected failure for 0-byte file")
	}

	// 3. Valid file with content
	validPath := filepath.Join(tempDir, "valid.txt")
	if err := os.WriteFile(validPath, []byte("Hello ActonOS"), 0644); err != nil {
		t.Fatal(err)
	}
	res = verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertFileExists,
		Target:   "valid.txt",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected valid file to pass: %s", res.Message)
	}
}

func TestOutcomeVerifier_FileContains(t *testing.T) {
	tempDir := t.TempDir()
	verifier := NewOutcomeVerifier(nil, nil)
	ctx := context.Background()

	filePath := filepath.Join(tempDir, "report.md")
	if err := os.WriteFile(filePath, []byte("# ActonOS Report\nStatus: All systems nominal\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Pass case
	res := verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertFileContains,
		Target:   "report.md",
		Expected: "All systems nominal",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected match to pass: %s", res.Message)
	}

	// Fail case
	res = verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertFileContains,
		Target:   "report.md",
		Expected: "System failure detected",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected missing substring to fail")
	}
}

func TestOutcomeVerifier_DirNotEmpty(t *testing.T) {
	tempDir := t.TempDir()
	verifier := NewOutcomeVerifier(nil, nil)
	ctx := context.Background()

	subDir := filepath.Join(tempDir, "output")
	_ = os.MkdirAll(subDir, 0755)

	// Empty dir
	res := verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertDirNotEmpty,
		Target:   "output",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected empty dir to fail")
	}

	// Non-empty dir
	_ = os.WriteFile(filepath.Join(subDir, "item.json"), []byte("{}"), 0644)
	res = verifier.VerifyAssertion(ctx, tempDir, OutcomeAssertion{
		Kind:     AssertDirNotEmpty,
		Target:   "output",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected non-empty dir to pass: %s", res.Message)
	}
}

func TestOutcomeVerifier_HTTPStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	verifier := NewOutcomeVerifier(nil, nil)
	ctx := context.Background()

	// 200 OK
	res := verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertHTTPStatus,
		Target:   ts.URL + "/health",
		Expected: "200",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected HTTP 200 to pass: %s", res.Message)
	}

	// 404 Expected
	res = verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertHTTPStatus,
		Target:   ts.URL + "/notfound",
		Expected: "404",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected HTTP 404 match to pass: %s", res.Message)
	}

	// Mismatch
	res = verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertHTTPStatus,
		Target:   ts.URL + "/notfound",
		Expected: "200",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected HTTP mismatch to fail")
	}
}

func TestOutcomeVerifier_JSONSchema(t *testing.T) {
	verifier := NewOutcomeVerifier(nil, nil)
	ctx := context.Background()

	validJSON := `{"id": "usr_123", "name": "Bieber", "active": true}`
	res := verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertJSONSchema,
		Target:   validJSON,
		Expected: "id,name,active",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected valid json to pass: %s", res.Message)
	}

	// Missing required key
	res = verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertJSONSchema,
		Target:   validJSON,
		Expected: "id,name,email",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected missing key 'email' to fail")
	}
}

func TestOutcomeVerifier_SQLCount(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, _ = db.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT);`)
	_, _ = db.Exec(`INSERT INTO users VALUES ('u1', 'Alice'), ('u2', 'Bob'), ('u3', 'Charlie');`)

	verifier := NewOutcomeVerifier(db, nil)
	ctx := context.Background()

	// Condition: > 2 (should pass, count=3)
	res := verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertSQLCount,
		Target:   "SELECT COUNT(*) FROM users",
		Expected: "> 2",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected SQL count > 2 to pass: %s", res.Message)
	}

	// Condition: == 3 (should pass)
	res = verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertSQLCount,
		Target:   "SELECT COUNT(*) FROM users",
		Expected: "= 3",
		Critical: true,
	})
	if !res.Passed {
		t.Fatalf("expected SQL count = 3 to pass: %s", res.Message)
	}

	// Condition: > 5 (should fail)
	res = verifier.VerifyAssertion(ctx, "", OutcomeAssertion{
		Kind:     AssertSQLCount,
		Target:   "SELECT COUNT(*) FROM users",
		Expected: "> 5",
		Critical: true,
	})
	if res.Passed {
		t.Fatal("expected SQL count > 5 to fail")
	}
}

func TestOutcomeVerifier_ParseAssertionString(t *testing.T) {
	verifier := NewOutcomeVerifier(nil, nil)

	cases := []struct {
		rule     string
		wantKind AssertionKind
		wantTar  string
		wantExp  string
	}{
		{"file_exists:data/report.md", AssertFileExists, "data/report.md", ""},
		{"file_contains:data/log.txt|ERROR_CODE_500", AssertFileContains, "data/log.txt", "ERROR_CODE_500"},
		{"http_status:http://localhost:8080/health|200", AssertHTTPStatus, "http://localhost:8080/health", "200"},
		{"sql_count:SELECT COUNT(*) FROM tasks|> 0", AssertSQLCount, "SELECT COUNT(*) FROM tasks", "> 0"},
		{"dir_not_empty:workspace/build", AssertDirNotEmpty, "workspace/build", ""},
	}

	for _, c := range cases {
		a, err := verifier.ParseAssertionString(c.rule)
		if err != nil {
			t.Fatalf("ParseAssertionString(%q) failed: %v", c.rule, err)
		}
		if a.Kind != c.wantKind || a.Target != c.wantTar || a.Expected != c.wantExp {
			t.Fatalf("ParseAssertionString(%q) = %+v, want kind=%s tar=%s exp=%s", c.rule, a, c.wantKind, c.wantTar, c.wantExp)
		}
	}
}

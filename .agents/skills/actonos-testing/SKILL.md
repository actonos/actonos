---
name: actonos-testing
description: "Skill for writing and running tests for ActonOS. Covers unit tests, integration tests, test patterns, mocking, and coverage."
---

# ActonOS Testing Skill

Use this skill when writing or running tests for ActonOS.

## Test Categories

| Category | Location | Build Tag | Command |
|:---|:---|:---|:---|
| Unit Tests | `internal/**/*_test.go` | (none) | `make test-unit` |
| Integration Tests | `tests/integration/` or `*_integ_test.go` | `integration` | `make test-integ` |
| E2E / Smoke Tests | `tests/e2e/` | `e2e` | `go test -tags=e2e ./tests/e2e/...` |

## Running Tests

```bash
# All tests
make test

# Unit tests only (fast, no external dependencies)
make test-unit
# Equivalent to: CGO_ENABLED=0 go test -race -count=1 -coverprofile=build/coverage.out ./internal/...

# Integration tests
make test-integ
# Equivalent to: CGO_ENABLED=0 go test -race -count=1 -tags=integration ./tests/...

# Specific package
go test -v ./internal/agent/...

# Specific test
go test -v -run TestDecayScore ./internal/memory/...

# With coverage
go test -coverprofile=build/coverage.out ./internal/...
go tool cover -html=build/coverage.out -o build/coverage.html
```

## Test Patterns

### Table-Driven Tests (Preferred)

```go
func TestDecayScore(t *testing.T) {
    tests := []struct {
        name     string
        elapsed  time.Duration
        lambda   float64
        weight   float64
        expected float64
    }{
        {
            name:     "recent_memory_high_score",
            elapsed:  1 * time.Hour,
            lambda:   24.0,
            weight:   1.0,
            expected: 0.959,
        },
        {
            name:     "old_memory_decayed",
            elapsed:  72 * time.Hour,
            lambda:   24.0,
            weight:   1.0,
            expected: 0.049,
        },
        {
            name:     "zero_elapsed_full_score",
            elapsed:  0,
            lambda:   24.0,
            weight:   1.0,
            expected: 1.0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := decay.Score(tt.elapsed, tt.lambda, tt.weight)
            if math.Abs(got-tt.expected) > 0.01 {
                t.Errorf("Score(%v, %v, %v) = %v, want %v",
                    tt.elapsed, tt.lambda, tt.weight, got, tt.expected)
            }
        })
    }
}
```

### Mock Interfaces

Define interfaces in production code, implement mocks in tests:

```go
// internal/llm/provider.go (production)
type LLMProvider interface {
    Complete(ctx context.Context, messages []Message, opts Options) (*Response, error)
    StreamComplete(ctx context.Context, messages []Message, opts Options) (<-chan Token, error)
}

// internal/llm/provider_test.go (test)
type MockLLMProvider struct {
    CompleteFunc       func(ctx context.Context, messages []Message, opts Options) (*Response, error)
    CompleteCalled     int
}

func (m *MockLLMProvider) Complete(ctx context.Context, messages []Message, opts Options) (*Response, error) {
    m.CompleteCalled++
    if m.CompleteFunc != nil {
        return m.CompleteFunc(ctx, messages, opts)
    }
    return &Response{Content: "mock response"}, nil
}

func NewMockLLM(response *Response) *MockLLMProvider {
    return &MockLLMProvider{
        CompleteFunc: func(_ context.Context, _ []Message, _ Options) (*Response, error) {
            return response, nil
        },
    }
}
```

### Test Helpers

```go
// internal/testutil/helpers.go
package testutil

func NewTestDB(t *testing.T) *memory.DB {
    t.Helper()
    dir := t.TempDir()
    db, err := memory.Open(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatalf("failed to open test DB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return db
}

func NewTestServer(t *testing.T) *server.Server {
    t.Helper()
    db := NewTestDB(t)
    mgr := agent.NewManager(db)
    return server.New(mgr, db)
}
```

### Integration Test Example

```go
//go:build integration

package integration_test

import (
    "context"
    "testing"
    "d:/Projects/ActonOS/internal/memory"
)

func TestHybridSearch_FTS5AndVector(t *testing.T) {
    db := testutil.NewTestDB(t)
    hybrid := memory.NewHybridSearch(db)

    // Seed test data
    ctx := context.Background()
    hybrid.Index(ctx, "doc1", "ActonOS is an AI agent operating system")
    hybrid.Index(ctx, "doc2", "Docker containers run on Alpine Linux")
    hybrid.Index(ctx, "doc3", "The agent engine uses ReAct loop for reasoning")

    // Search
    results, err := hybrid.Search(ctx, "AI agent reasoning", 2)
    if err != nil {
        t.Fatalf("Search() error: %v", err)
    }

    if len(results) != 2 {
        t.Fatalf("expected 2 results, got %d", len(results))
    }

    // First result should be the most relevant
    if results[0].DocID != "doc3" && results[0].DocID != "doc1" {
        t.Errorf("unexpected top result: %s", results[0].DocID)
    }
}
```

### HTTP Handler Tests

```go
func TestHandleListAgents(t *testing.T) {
    srv := testutil.NewTestServer(t)

    // Create test agent
    ctx := context.Background()
    srv.AgentManager.Create(ctx, agent.Manifest{Name: "Test Agent"})

    // Make request
    req := httptest.NewRequest("GET", "/api/agents", nil)
    req.Header.Set("Authorization", "Bearer test-token")
    w := httptest.NewRecorder()

    srv.ServeHTTP(w, req)

    if w.Code != 200 {
        t.Fatalf("expected 200, got %d", w.Code)
    }

    var resp struct {
        Data struct {
            Agents []agent.Manifest `json:"agents"`
        } `json:"data"`
    }
    json.NewDecoder(w.Body).Decode(&resp)

    if len(resp.Data.Agents) != 1 {
        t.Errorf("expected 1 agent, got %d", len(resp.Data.Agents))
    }
}
```

## Coverage Targets

| Package | Target | Priority |
|:---|:---|:---|
| `internal/memory/` | 80%+ | Critical (data integrity) |
| `internal/agent/` | 70%+ | High (core logic) |
| `internal/auth/` | 80%+ | Critical (security) |
| `internal/server/` | 60%+ | Medium (handlers) |
| `internal/tools/` | 60%+ | Medium |
| `internal/sandbox/` | 70%+ | High (security) |

## Test File Naming

```
internal/
├── agent/
│   ├── engine.go
│   ├── engine_test.go            ← Unit tests
│   └── engine_integ_test.go      ← Integration tests (build tag)
├── memory/
│   ├── decay.go
│   └── decay_test.go             ← Unit tests
```

## Mandatory Security Cases

Changes touching tools, agents, workspace APIs, MCP, or sandboxing must test:

1. Unauthorized tools are rejected at execution time.
2. Medium/High actions pause with a durable approval.
3. Approval hashes reject modified arguments and expired/rejected decisions.
4. Lexical and symlink path escapes are rejected.
5. Loopback, private, link-local, metadata, and redirect SSRF targets are rejected.
6. Destructive commands are denied before process startup.
7. Non-zero command exits are classified as failures.
8. Repeated observations/failures terminate rather than loop forever.
9. Trace IDs correlate run, tool, approval, and audit records.
10. Linux sandbox builds compile with cgroup v2 attachment enabled.
11. Streaming tests cover token deltas and fragmented tool-call reconstruction.
12. Audit tests validate hash-chain integrity and tamper detection.
13. Durable resume tests prove the approved tool executes once and the original
    run continues from its saved state.
14. MCP HTTP tests cover JSON and `data:` SSE JSON-RPC responses.
15. Provider streaming tests cover OpenAI-compatible, Anthropic, and Gemini.
16. Realtime WebSocket tests cover authenticated upgrade, snapshot shape and disconnect cleanup.
17. MCP administration tests prove environment secrets never appear in list responses and enable/disable restores persisted configuration.

Security package target: `internal/security/` must maintain at least 90% coverage.

The autonomous-kernel baseline established on 2026-08-18 is: agent 71.5%,
server 60.6%, tools 60.3%, sandbox 87.8%, security 92.3%, and memory 83.3%.
Changes must not reduce a package below its target.

Linux CI must run `CGO_ENABLED=1 go test -race -count=1 ./internal/...`.
Provider-storage tests must prove secrets are absent from metadata files,
legacy plaintext is migrated and removed, and Vault deletion works. Backup
tests must query committed WAL data from the downloaded snapshot.

## Reference Files

- [docs/DEVELOPMENT.md](../../../docs/DEVELOPMENT.md) — Testing section
- [Makefile](../../../Makefile) — Test targets

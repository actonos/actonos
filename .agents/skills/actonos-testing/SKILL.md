---

## Frontend UX verification matrix

Authenticated browser tests must cover the dashboard shell, command palette, density persistence, Operations baseline, and axe checks at 390×844, 768×1024, and 1440×900. Keep snapshots in `web/tests/authenticated.spec.ts-snapshots/`; regenerate only for intentional visual changes. Include keyboard navigation and reduced-motion assertions for newly added overlays or menus.
The authenticated suite also smoke-checks all primary routes for heading visibility and horizontal-overflow regressions.
It must also run axe against every authenticated primary route after route transitions settle, reload hash-query deep links, and exercise the narrow-layout Chat session drawer.
name: actonos-testing
description: "Skill for writing and running tests for ActonOS. Covers unit tests, integration tests, test patterns, mocking, and coverage."
---

# ActonOS Testing Skill

Use this skill when writing or running tests for ActonOS.

## Test Categories

| Category | Location | Build Tag | Command |
|:---|:---|:---|:---|
| Unit Tests | `internal/**/*_test.go` | (none) | `make test-unit` |
| Integration Tests | `*_os_test.go` / `*_integ_test.go` under `internal/` | `integration` | `make test-integ` |
| Benchmark Eval Suite | `evals/` (46 tasks, mock gate) | (none) | `./evals/run.sh` / `.\evals\run.ps1` |
| E2E / Smoke Tests | `web/tests/*.spec.ts` | (none) | `cd web && npm run test:e2e` |
| Frontend Unit | `web/src/**/*.test.{ts,tsx}` | (none) | `cd web && npm test` |
| Frontend Browser | `web/tests/*.spec.ts` | (none) | `cd web && npm run test:e2e` |
| Frontend Quality | `web/` | (none) | `cd web && npm run quality` |

## Running Tests

```bash
# While working: build + vet + typecheck, optionally one package's tests, short output
make check-fast PKG=./internal/<pkg>/...
make check-web

# Final gate (lint, Go tests with coverage floors, web quality incl. Playwright); run alone
make check-full

# All Go tests
make test

# Unit tests only (fast, no external dependencies)
make test-unit
# Equivalent to: CGO_ENABLED=0 go test -count=1 -coverprofile=build/coverage.out ./internal/...
# then go run ./scripts/cover-gate.go build/coverage.out

# Cognition & Reliability Benchmark Suite (mock mode, CI gate; pass rate does not measure model quality)
./evals/run.sh
# on Windows PowerShell:
.\evals\run.ps1

# Live model benchmark evaluation (needs provider keys in the environment; the user runs it)
go run ./evals/runner --mode=live --model=anthropic/claude-sonnet-4.5 --output=eval_report.md --fail-under=90.0

# Integration tests
make test-integ
# Equivalent to: CGO_ENABLED=0 go test -count=1 -tags=integration ./internal/...

# Specific package
go test -v ./internal/agent/...

# Specific test
go test -v -run TestName -count=1 ./internal/memory/...

# Race detector (CGO; Linux CI runs it for ./internal/...)
make test-race

# With coverage
go test -coverprofile=build/coverage.out ./internal/...
go tool cover -html=build/coverage.out -o build/coverage.html

# Frontend lint, locale parity, strict visible-text audit, Vitest, build,
# bundle budget, Playwright smoke tests, and axe accessibility checks
cd web
npm run quality
npx tsc --noEmit

# Playwright (starts Vite itself; API calls are mocked per test). Run it alone, not next to Go tests
npm run test:e2e
```

Frontend mutation tests must cover `202 approval_required` separately from
successful completion. Realtime tests must verify malformed-frame handling,
reconnect behavior, and shared snapshot consumption. Modal tests must assert
dialog semantics and asynchronous close behavior.

`npm run quality` must execute the hardcoded text audit with `--fail` and the
Playwright suite. Browser tests must use axe and reject serious or critical
accessibility violations. Production frontend lint must reject explicit `any`
and unused variables.

## Test Patterns (copy a real example, do not invent helpers)

There is no shared `testutil` package. Each package keeps its own small helpers next to its tests; find the closest existing one before writing a new one.

| Need | Real example |
|:---|:---|
| Table-driven unit test | any `*_test.go` in `internal/memory/` (for example `decay_test.go`), `internal/mailcal/policy/policy_test.go` |
| Temporary SQLite DB | `setupTestDB(t)` in `internal/agent/manager_test.go`, `internal/auth/system_auth_test.go`, `internal/channels/session_test.go`; `memory.Open(filepath.Join(t.TempDir(), "x.db"))` |
| Full HTTP server | `newTestServer(t)` in `internal/server/server_test.go` (auth disabled via `DisableAuthForTest`), then drive `srv.Router()` with `httptest` |
| Mail and calendar service with a provider | `newFixture(t, nil)` in `internal/mailcal/testutil_test.go`; write path: `newWriteFixture` in `write_test.go` |
| WASM plugin from a guest package | `refplugintest.BuildGuest` / `InstanceOf` in `internal/mailcal/refplugin/refplugintest`; hand-assembled module in `internal/plugin/abi_module_test.go` |
| Sandbox | needs real bubblewrap; tests skip themselves when namespaces are unavailable |
| Fakes for interfaces | define the small interface in production code, a struct with function fields in the `_test.go` file |
| Fuzzing | `FuzzEvaluate...` in `internal/mailcal/policy`, `FuzzParse...` in `internal/mailcal/provkit/icskit`; run `go test -run '^$' -fuzz '^FuzzName$' -fuzztime=20s ./pkg` |
| Web component | Vitest + Testing Library next to the component; wait for `i18nReady` (already in `src/test/setup.ts`); call `cleanup` in `afterEach` when using `it.each` |
| Browser flow | `web/tests/*.spec.ts` with `page.route` mocks |

Conventions: table-driven with `t.Run`, `t.Helper()` in helpers, `t.TempDir()` and `t.Cleanup()` for resources, no sleeps (inject a clock or a `sleep` func), and a test that proves the bad case (an invariant is only tested when a mutation of it fails the test).

## Coverage Floors

The floors live in `scripts/cover-gate.go` (memory 80, auth 80, security 90, plugin 70, channels 70, agent 70, sandbox 70, server 60, tools 60). `make test-unit` enforces them; never lower one to make a change pass. `internal/server` and `internal/security` sit close to their floors, so new code there needs its own tests.

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

Linux CI must run `CGO_ENABLED=1 go test -race -count=1 ./internal/...`.
Provider-storage tests must prove secrets are absent from metadata files,
legacy plaintext is migrated and removed, and Vault deletion works. Backup
tests must query committed WAL data from the downloaded snapshot.

## Reference Files

- [docs/DEVELOPMENT.md](../../../docs/DEVELOPMENT.md) — Testing section
- [Makefile](../../../Makefile) — Test targets

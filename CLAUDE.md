# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

ActonOS is an AI-agent "operating system" kernel: one Go daemon (`cmd/actond`) plus a React 19 SPA (`web/`) that the daemon serves. Detailed rules also live in `.agents/AGENTS.md` and `.agents/rules/` (coding standards, verification checklist, source registry of routes/pages) — read the relevant ones before changing APIs or UI.

## Commands

Go lives at `/usr/local/go/bin` on this machine and may not be on `PATH`.

```bash
make deps            # go mod download + npm ci
make dev             # scripts/dev.sh: actond on :8080 + Vite on :5173 (Vite proxies /api)
make lint            # scripts/lint.sh: go vet, gofmt, golangci-lint (if installed), go mod tidy check, tsc, eslint
make test-unit       # go test ./internal/... with coverage, then scripts/cover-gate.go (per-package floors)
make test-race       # CGO_ENABLED=1 go test -race ./internal/...
make build           # web build -> internal/server/dist, then ./build/actond   (build-only: skip the web step)
make plugins         # build the mail/calendar provider plugins into build/plugins/<id>/
make conformance     # ... and certify each (cmd/mailcal-conformance writes conformance.json beside plugin.wasm)
scripts/eval-office.sh <provider/model>   # office eval suite (mock gate; live runs are user-run)
make build-embedding # build/embeddingd (CGO + `-tags ORT`, needs ONNX Runtime at runtime)

go test ./internal/agent -run TestName -count=1     # single Go test
go test -tags=integration ./internal/...            # integration-tagged tests
go run ./scripts/cover-gate.go build/coverage.out   # re-check coverage floors

cd web && npm run lint            # eslint --max-warnings=0
cd web && npx vitest run src/lib/api.test.ts        # single web test file
cd web && npm run quality         # lint, i18n checks, vitest, build, bundle budget, Playwright e2e
go run ./evals/runner --mode=mock --output=eval_report.md   # mock harness only
```

The `Makefile` builds with `CGO_ENABLED=0`; only `test-race` and `build-embedding` need CGO. CI (`.github/workflows/ci.yml`) runs vet, tests, cover-gate and the web checks.

## Configuration gotchas

- `actond` reads flags and `ACTON_*` env vars (`ACTON_DATA_DIR`, `ACTON_LISTEN_ADDR`, `ACTON_LOG_LEVEL`, `ACTON_EMBEDDING_URL`). The `DATA_DIR`, `PORT`, `LISTEN_ADDR`, `LOG_LEVEL` names used in `scripts/dev.sh` and `docs/DEVELOPMENT.md` are **not read** — dev data actually lands in `./data`.
- Env vars the code does read: `RUNTIME_MODE=docker`, `DISABLE_TAILSCALE=true`, `ACTONOS_ALLOW_INSECURE_EXEC=1` (dev only, enables the unsandboxed subshell), `ACTONOS_ALLOW_UNSANDBOXED_MCP`, `ACTONOS_CGROUP_BASE` / `ACTONOS_CGROUP_OPTIONAL`.
- `web/node_modules` may have been installed on another OS; if `vite build` fails with a missing Tailwind/oxide native binding, run `npm ci` (it wipes `node_modules`) or build from a copy.

## Architecture

**Wiring.** `cmd/actond/main.go` constructs every subsystem in order (SQLite `memory.DB` → vector store → vault → audit logger → agent manager/engine → tools/MCP/plugins → channels → notifications → `server.NewServer(Config)`) and passes them explicitly; there is no DI container. `internal/server/router.go` registers every route (`api_<subsystem>.go` hold handlers). JSON responses are wrapped as `{"data": ...}` / `{"error": {"code","message"}}`. The built SPA is `go:embed`-ed from `internal/server/dist` (gitignored).

**Agent loop.** `internal/agent/engine.go` is the ReAct loop; `planner.go` builds a DAG of `PlanStep`s executed in concurrent "burst pulses"; `verifier*.go` runs deterministic outcome assertions (file_exists, json_schema, sql_count, …) so a task is only "done" when evidence exists; `heartbeat.go` + `proactive.go` + `reflection.go` are the autonomous daemons (missions, anomaly scans, self-review); `swarm.go` delegates to sub-agents. Prompts are assembled in `prompt_*.go`.

**Single execution boundary.** Every tool call from agents, API, cron or channels must go through `tools.ToolRegistry.Execute` (`internal/tools/registry.go`) so policy, risk tier (`risk.go`), human approval (`approval.go`), tracing, sandboxing and audit cannot be bypassed. Tools come from three sources: native tools (`native_tools.go`), MCP servers (`mcp_client.go`), and WASM plugins (`internal/plugin`) / skill folders (`skill_watcher.go`, `hub.go`).

**Sandbox (fail-closed).** `sandbox.AutoDetectSandbox`: bubblewrap + cgroup v2 on bare-metal Linux; the host subshell only inside Docker (`RUNTIME_MODE=docker`) or with `ACTONOS_ALLOW_INSECURE_EXEC=1`; otherwise an "unavailable" sandbox that refuses to run commands.

**Plugins.** `internal/plugin` runs WASM (wazero, pure Go) tools, channel adapters and connectors. Guests talk to the host through `host_api.go` syscalls; `security_gate.go` enforces the manifest allowlist for network, secrets, storage and bus topics, and `internal/security` blocks private/loopback egress. Channel plugins are bridged into `internal/channels` (`ChannelManager`, `MessageRouter`, pairing) and publish inbound messages on the event bus.

**Memory.** `internal/memory`: SQLite (`modernc.org/sqlite`, pure Go) with FTS5 plus chromem-go vectors, combined in `HybridEngine`. Embeddings come from the separate CGO `cmd/embeddingd` helper over loopback so `actond` stays static. Memories have importance tiers, pinning and Ebbinghaus decay (`decay.go`); the vault (`vault.go`) is AES-256-GCM.

**Mail & calendar.** `internal/mailcal` serves mail and calendar through WASM provider plugins (`internal/mailcal/plugins/*`, ABI `mail@1`/`calendar@1` in `internal/mailcal/abi`, additive-only, golden-tested). The core decides, plugins execute and are always untrusted: tools with an external effect (send/delete mail, calendar writes) pass a mandatory `ToolGate` inside `ToolRegistry.Execute` (always High risk, no grants, `needs_approval` cannot be bypassed even by cron), `native_mail_send` takes only an immutable `draft_id`, and auto-send is closed by default (master switch, `policy` rule engine, only humans approve rule proposals). New providers are built from `provkit` and add no core file (see `docs/MAILCAL-PLUGIN-GUIDE.md`; `plugins/echomail` is the worked example). Per-package tests for every shipped plugin run in `internal/mailcal/allplugins_test.go`.

**Other pieces.** `internal/bus` (in-process pub/sub that glues components), `internal/llm` (provider adapters + `ModelCascadeRouter` with per-task-kind routing and fallback), `internal/system` (HAL for bare-metal vs Docker, audit hash-chain log, OTA with atomic symlink swap, backup/restore, notifications, tsnet), `internal/auth` (OAuth 2.1 PKCE, DCR, token refresher), `internal/workspace` (DB-backed user file store).

## Conventions that bite

- Changing a JSON-exposed Go struct → update `web/src/lib/types.ts`. Adding/changing an endpoint → `router.go`, `docs/API.md`, `.agents/rules/source-registry.md`. New user-facing text → keys in **both** `web/src/locales/en` and `vi` (new namespace: also `web/src/lib/i18n.ts`). User-visible changes → `CHANGELOG.md` under `[Unreleased]`.
- Web locales are lazy-loaded per language (`web/src/lib/i18n.ts`); render and tests must wait for `i18nReady` (already done in `main.tsx` and `src/test/setup.ts`).
- Web CI enforces: eslint with zero warnings and no `any`, locale parity (`check:i18n`), zero hardcoded UI strings (`audit:i18n`), no emoji in UI, named exports only, entry-bundle budget in `web/scripts/check-bundle.mjs`.
- Go: wrap errors with `fmt.Errorf("...: %w", err)`, `slog` not `fmt`/`log`, `ctx` first parameter, platform code in `_linux.go`/`_windows.go` files or build tags. Commits use Conventional Commits scoped by package, and **never** include `Co-Authored-By` (or any other attribution/"Generated with" trailer) in commit messages or PR descriptions.
- Coverage floors per package are in `scripts/cover-gate.go`; `internal/server` and `internal/security` sit only slightly above theirs, so new code there needs tests.
- `evals/` runs in **mock** mode only (it pre-creates the artifacts it asserts on); its pass rate says nothing about model quality, and `--mode=live` is not implemented.

## Testing patterns

- Server tests build a full server with `newTestServer(t)` (`internal/server/server_test.go`, auth disabled via `DisableAuthForTest`) and drive `srv.Router()` with `httptest`.
- Sandbox tests need real bubblewrap and skip themselves when namespaces are unavailable.
- Plugin ABI tests use a hand-assembled WASM module (`internal/plugin/abi_module_test.go`); websocket syscalls are tested by injecting a connection because the real dial path refuses loopback.

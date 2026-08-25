# ActonOS Development Guide

> Complete A-to-Z guide for developing, building, and testing ActonOS.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Repository Setup](#repository-setup)
- [Project Structure](#project-structure)
- [Development Workflow](#development-workflow)
- [Building from Source](#building-from-source)
- [Testing Strategy](#testing-strategy)
- [Code Style & Linting](#code-style--linting)
- [Debugging Tips](#debugging-tips)
- [Commit Conventions](#commit-conventions)
- [Common Tasks](#common-tasks)

---

## Prerequisites

### Required Tools

| Tool | Minimum Version | Installation |
|:---|:---|:---|
| **Go** | 1.26+ | [go.dev/dl](https://go.dev/dl/) |
| **Node.js** | 22 LTS+ | [nodejs.org](https://nodejs.org/) or via `nvm` |
| **npm** | 10+ | Bundled with Node.js |
| **Make** | 4.0+ | Pre-installed on Linux/macOS; use `choco install make` on Windows |
| **Git** | 2.40+ | [git-scm.com](https://git-scm.com/) |

### Optional Tools

| Tool | Purpose |
|:---|:---|
| **Docker** 24+ | Building container images, running in Docker mode |
| **golangci-lint** | Advanced Go linting (installed automatically by `scripts/lint.sh`) |
| **air** | Go live-reload for development (`go install github.com/air-verse/air@latest`) |

### System Requirements (Development Machine)

- **RAM**: 4 GB minimum, 8 GB recommended
- **Disk**: 2 GB free for build artifacts
- **OS**: Linux (recommended), macOS, or Windows (via WSL2)

---

## Repository Setup

```bash
# 1. Clone the repository
git clone https://github.com/actonos/actonos.git
cd actonos

# 2. Install all dependencies (Go modules + Node packages)
make deps

# 3. Verify the setup
go version          # Should show 1.26+
node --version      # Should show v22+
make version        # Should show current version (e.g., 0.1.0)

# 4. Run in development mode
make dev
```

### Environment Variables (Optional)

Create a `.env` file in the project root for local development:

```bash
# .env (not committed to git)
RUNTIME_MODE=docker           # Force Docker mode on dev machine
LOG_LEVEL=debug               # debug | info | warn | error
LISTEN_ADDR=:8080             # HTTP listen address
DATA_DIR=./dev-data           # Local data directory (instead of /data)
DISABLE_TAILSCALE=true        # Skip Tailscale in development
ACTONOS_ALLOW_INSECURE_EXEC=1 # Dev-only: allow unsandboxed command exec (never production)
ACTON_EMBEDDING_URL=http://127.0.0.1:8091
ACTON_EMBEDDING_MODEL_DIR=./build/models/multilingual-e5-small/614241f622f53c4eeff9890bdc4f31cfecc418b3
ONNXRUNTIME_SHARED_LIBRARY_PATH=/path/to/libonnxruntime.so.1.28.0
```

---

## Project Structure

```
actonos/
├── cmd/
│   ├── actond/
│   │   └── main.go                 # Static core daemon entrypoint
│   └── embeddingd/                 # Loopback ONNX embedding helper (CGO)
│       ├── main.go                 # ORT build-tag implementation
│       └── disabled.go             # Non-CGO/default build stub
│
├── internal/                       # Private application code
│   ├── agent/                      # AI Agent Engine
│   │   ├── engine.go               # ReAct state machine
│   │   ├── manager.go              # Agent CRUD & manifest management
│   │   ├── swarm.go                # Sub-agent spawning & delegation
│   │   ├── planner.go              # Task decomposition & planning
│   │   ├── verifier.go             # Deterministic AST & invariant checker
│   │   ├── reflection.go           # Background fact & learning extraction
│   │   ├── profile.go              # User persona & dynamic preferences
│   │   ├── heartbeat.go            # Proactive task daemon
│   │   ├── context.go              # Context window & token pruning
│   │   └── types.go                # Shared types and interfaces
│   │
│   ├── auth/                       # Authentication & Authorization
│   │   ├── oauth2.go               # PKCE authorization flow (S256)
│   │   ├── dcr.go                  # Dynamic Client Registration
│   │   ├── delegation.go           # Scope manifest & zero-trust delegation
│   │   └── token_refresher.go      # Background token refresh daemon
│   │
│   ├── bus/                        # Event-Driven Message Bus
│   │   ├── eventbus.go             # Go channel pub/sub
│   │   └── messages.go             # Unified message format
│   │
│   ├── channels/                   # Communication Channel Core
│   │   ├── adapter.go              # ChannelAdapter interface
│   │   ├── manager.go              # Dynamic ChannelManager & dispatch
│   │   ├── router.go               # Inbound message routing
│   │   ├── session.go              # Session state tracking
│   │   ├── pairing.go              # 6-digit security pairing codes
│   │   └── webhook.go              # Generic webhook adapter
│   │
│   ├── llm/                        # LLM Provider Interface
│   │   ├── provider.go             # LLMProvider interface
│   │   ├── router.go               # Fallback cascade router
│   │   ├── openai.go               # OpenAI provider
│   │   ├── anthropic.go            # Anthropic provider
│   │   ├── gemini.go               # Google Gemini provider
│   │   ├── deepseek.go             # DeepSeek provider
│   │   └── ollama.go               # Local Ollama provider
│   │
│   ├── plugin/                     # WasmLoader Unified Plugin Subsystem
│   │   ├── types.go                # Manifest, capabilities & permissions
│   │   ├── loader.go               # Wazero runtime, module compile cache
│   │   ├── host_api.go             # Host syscalls (HTTP, Vault, Storage, Bus)
│   │   ├── security_gate.go        # Egress domain firewall & RBAC checks
│   │   ├── bridge_tool.go          # ToolRegistry execution bridge
│   │   ├── bridge_channel.go       # ChannelManager dynamic adapter bridge
│   │   ├── bridge_connector.go     # SaaS connector hooks & sync
│   │   └── manager.go              # Plugin lifecycle & hot-reload
│   │
│   ├── tools/                      # Dynamic Tooling Hub
│   │   ├── registry.go             # Centralized tool schema registry
│   │   ├── mcp_client.go           # MCP host (stdio/SSE)
│   │   ├── skill_watcher.go        # fsnotify skill folder watcher
│   │   └── native_tools.go         # HTTP fetch, filesystem, sysinfo
│   │
│   ├── sandbox/                    # Execution Sandboxing
│   │   ├── executor.go             # Sandbox interface
│   │   ├── bwrap_linux.go          # Bubblewrap + Cgroups v2 (bare-metal)
│   │   └── subshell.go             # Fallback runner (Docker)
│   │
│   ├── memory/                     # Storage & Hybrid RAG
│   │   ├── db.go                   # SQLite engine (modernc.org/sqlite)
│   │   ├── fts.go                  # FTS5 lexical search
│   │   ├── vector.go               # Vector store (chromem-go)
│   │   ├── embedding.go            # Durable jobs, chunk activation and search
│   │   ├── embedding_watcher.go    # Recursive workspace mutation watcher
│   │   ├── decay.go                # Ebbinghaus decay algorithm
│   │   ├── hybrid.go               # Calibrated sigmoid fusion
│   │   └── vault.go                # AES-256-GCM vault (Argon2id; not DMI/CPU bound)
│   │
│   ├── system/                     # Hardware Abstraction Layer
│   │   ├── hal.go                  # HAL interface
│   │   ├── baremetal_linux.go      # NetworkManager D-Bus, udev, systemd
│   │   ├── docker_hal.go           # Container mode HAL stub
│   │   ├── metrics.go              # CPU, RAM, temperature readings
│   │   ├── tsnet.go                # Embedded Tailscale
│   │   └── ota.go                  # Atomic update & watchdog
│   │
│   └── server/                     # HTTP Server & APIs
│       ├── router.go               # Chi router, WebSocket hub
│       ├── layered_fs.go           # Override layer for go:embed
│       ├── api_setup.go            # Onboarding & Wi-Fi config API
│       ├── api_agent.go            # Agent CRUD API
│       ├── api_integrations.go     # OAuth & SaaS integration API
│       ├── api_plugins.go          # WASM Plugin management API
│       ├── api_tools.go            # MCP and skills management API
│       ├── api_system.go           # Hardware, Tailscale, OTA API
│       └── static.go              # go:embed static assets
│
├── web/                            # Frontend (React 19 + Tailwind v4)
│   ├── src/
│   │   ├── pages/
│   │   │   ├── SetupWizard/        # Onboarding setup wizard
│   │   │   ├── Chat/               # Streaming chat interface
│   │   │   ├── Agents/             # Agent management & creation
│   │   │   ├── Workspace/          # Sandbox file manager
│   │   │   ├── Integrations/       # OAuth 1-click SaaS setup
│   │   │   ├── ToolHub/            # MCP, Skills, WASM plugin manager
│   │   │   └── Settings/           # API keys, Tailscale, metrics, OTA
│   │   └── App.tsx
│   ├── package.json
│   └── vite.config.ts
│
├── deploy/                         # Deployment Packaging
│   ├── docker/
│   │   ├── Dockerfile              # Multi-stage build (<35 MB image)
│   │   └── docker-compose.yml      # Quick-start template
│   └── live-build/                 # Debian ISO generation
│       ├── auto/
│       ├── config/
│       └── preseed/
│           └── auto-install.cfg    # Automated disk partitioning
│
├── scripts/                        # Build & Development Scripts
│   ├── dev.sh                      # Dev server launcher
│   ├── lint.sh                     # Combined Go + TS linting
│   ├── version-bump.sh             # SemVer version bump
│   ├── changelog-gen.sh            # Changelog generation
│   └── build-iso.sh               # ISO creation
│
├── tests/                          # Integration tests
│   └── integration/
│
├── docs/                           # Documentation
├── Makefile                        # Build pipeline
├── VERSION                         # Source-of-truth version
├── CHANGELOG.md                    # Release history
├── go.mod
└── go.sum
```

---

## Development Workflow

### Daily Development Loop

```bash
# 1. Start development servers (backend + frontend with hot-reload)
make dev

# 2. Make your changes in internal/ or web/src/

# 3. Run linters before committing
make lint

# 4. Run tests
make test

# 5. Commit with conventional commit message
git commit -m "feat(agent): add swarm delegation timeout handling"
```

### Backend-Only Development

```bash
# Build and run the Go binary only (no frontend rebuild)
make build-only
./build/actond --data-dir=./dev-data --log-level=debug

# Or use air for live-reload (install: go install github.com/air-verse/air@latest)
air
```

### Frontend-Only Development

```bash
# Start Vite dev server with HMR
cd web
npm run dev

# The frontend will proxy API calls to localhost:8080 (configure in vite.config.ts)

# Complete local frontend gate
npm run quality
npx tsc --noEmit

# Browser smoke tests, with ActonOS running
npm run test:e2e
```

`npm run quality` runs ESLint, English/Vietnamese locale parity, the strict
hardcoded-visible-text audit, Vitest, production compilation, the entry-bundle
budget, and Playwright browser tests. The browser suite includes an axe scan
that fails on serious or critical accessibility violations. New frontend
behavior should include a colocated `*.test.ts(x)` regression test.
Locale parity validation also fails on common UTF-8 corruption signatures in
Vietnamese text, including mojibake and question marks embedded inside words.
The UI is intentionally emoji-free: use the Lucide component set for icons.
`npm run check:emoji` scans source and locale resources for emoji and common
mojibake fragments.

Frontend production TypeScript follows a zero-explicit-`any` policy and treats
unused variables as lint errors. Shared transport belongs in
`web/src/lib/api/client.ts`; domain-specific API modules belong under
`web/src/lib/api/` and are exposed through the compatibility facade in
`web/src/lib/api.ts`.

---

## Building from Source

### Development Build

```bash
# Quick build (Go binary only, skip web)
make build-only

# Full build (web + Go)
make build
# Output: build/actond

# Frontend assets default to internal/server/dist for go:embed.
# Override when a separate output is required:
VITE_BUILD_DIR=../build/web-dist npm --prefix web run build

# Download pinned multilingual-e5-small ONNX artifacts (~487 MB)
make model-embedding

# Build the CGO helper; ONNX Runtime 1.28.0 is required at runtime
make build-embedding
# Output: build/embeddingd
```

### Production Build

```bash
# Full production pipeline: lint → test → build-web → build-go
make all

# Docker image
make docker
# Output: actonos/actonos:0.1.0

# Bare-metal ISO
make iso
# Output: build/ActonOS-v0.1.0.iso
```

### Build Variables

The binary is built with the following ldflags:

| Variable | Source | Example |
|:---|:---|:---|
| `main.Version` | `VERSION` file | `0.1.0` |
| `main.GitCommit` | `git rev-parse --short HEAD` | `a1b2c3d` |
| `main.BuildTime` | Build timestamp (UTC) | `2026-08-16T12:00:00Z` |

Access in Go code:
```go
var (
    Version   string // set via -ldflags
    GitCommit string
    BuildTime string
)
```

---

## Testing Strategy

### Test Pyramid

```
┌─────────────────────────┐
│    E2E / Smoke Tests    │  ← Few: critical user flows
├─────────────────────────┤
│  Integration Tests      │  ← Moderate: API, DB, cross-package
├─────────────────────────┤
│    Unit Tests           │  ← Many: pure logic, algorithms
└─────────────────────────┘
```

### Running Tests

```bash
# All tests
make test

# Unit tests only (fast, no external deps)
make test-unit

# Integration tests (may need dev-data directory)
make test-integ

# With coverage report
go test -coverprofile=build/coverage.out ./internal/...
go tool cover -html=build/coverage.out -o build/coverage.html
```

Current autonomous-kernel verification baseline:

| Package | Verified coverage |
|:---|---:|
| `internal/agent` | 71.5% |
| `internal/server` | 60.6% |
| `internal/tools` | 60.3% |
| `internal/sandbox` | 87.8% |
| `internal/security` | 92.3% |
| `internal/memory` | 83.3% |

Keep `CGO_ENABLED=0`. On Windows, the Go race detector is unavailable in this
configuration; run the regular suite locally and the race suite in Linux CI.

```bash
# Linux only; requires CGO toolchain
make test-race
```

The `.github/workflows/backend-race.yml` workflow runs this gate on every pull
request and push to `main`.

The race baseline was verified locally on Windows with MSYS2 UCRT64 GCC on
August 18, 2026. Keep the Linux CI gate enabled because it is the canonical,
reproducible environment.

### Writing Tests

**Naming convention:**
```
internal/
├── agent/
│   ├── engine.go
│   ├── engine_test.go          ← Unit tests
│   └── engine_integ_test.go    ← Integration tests (build tag)
```

**Integration test build tag:**
```go
//go:build integration

package agent_test
```

**Table-driven tests (preferred):**
```go
func TestDecayScore(t *testing.T) {
    tests := []struct {
        name     string
        elapsed  time.Duration
        lambda   float64
        expected float64
    }{
        {"recent_memory", 1 * time.Hour, 24.0, 0.959},
        {"old_memory", 72 * time.Hour, 24.0, 0.049},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := decay.Score(tt.elapsed, tt.lambda)
            if math.Abs(got-tt.expected) > 0.01 {
                t.Errorf("Score() = %v, want %v", got, tt.expected)
            }
        })
    }
}
```

---

## Code Style & Linting

### Go Code Style

- Follow the official [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Use `gofmt` / `goimports` for formatting (enforced by linter)
- Keep functions under 60 lines where possible
- Prefer returning errors over panicking
- Use meaningful variable names (no single-letter names except in short loops)
- All exported types, functions, and methods must have doc comments

### TypeScript/React Code Style

- Use TypeScript strict mode
- Prefer functional components with hooks
- Co-locate component styles (CSS modules or Tailwind)
- Use named exports (no default exports)

### Running Linters

```bash
# Run all linters
make lint

# Or directly
bash scripts/lint.sh
```

---

## Debugging Tips

### Backend Debugging

```bash
# Enable debug logging
./build/actond --log-level=debug --data-dir=./dev-data

# Use Delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug ./cmd/actond/ -- --log-level=debug --data-dir=./dev-data

# Inspect SQLite database
sqlite3 dev-data/storage/app.db ".tables"
sqlite3 dev-data/storage/app.db "SELECT * FROM conversations LIMIT 10;"
sqlite3 dev-data/storage/app.db "SELECT status, count(*) FROM embedding_jobs GROUP BY status;"

# Check queue and helper readiness through actond
curl http://127.0.0.1:8080/api/system/embedding

# View audit log
tail -f dev-data/logs/audit.jsonl | jq .
```

### Frontend Debugging

```bash
# Vite dev server with source maps
cd web && npm run dev

# React DevTools: Install the browser extension
# Network tab: Monitor WebSocket frames for streaming chat
```

### Common Issues

| Issue | Solution |
|:---|:---|
| `CGO_ENABLED` errors | Ensure `CGO_ENABLED=0` in build. Use `modernc.org/sqlite` (pure Go) |
| Port 8080 in use | Set `LISTEN_ADDR=:8081` or kill the conflicting process |
| SQLite lock errors | Only one actond process should access the DB at a time |
| WASM plugin crash | Check plugin compiled for `wasm32-wasi` target |
| MCP server timeout | Increase timeout in MCP config, check stdio pipe is open |

---

## Commit Conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Description |
|:---|:---|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Code style (formatting, semicolons, etc.) |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `perf` | Performance improvement |
| `test` | Adding or correcting tests |
| `build` | Build system or external dependency changes |
| `ci` | CI/CD configuration |
| `chore` | Other changes (no production code change) |

### Scopes

| Scope | Package |
|:---|:---|
| `agent` | `internal/agent/` |
| `auth` | `internal/auth/` |
| `bus` | `internal/bus/` |
| `channels` | `internal/channels/` |
| `plugin` | `internal/plugin/` |
| `llm` | `internal/llm/` |
| `tools` | `internal/tools/` |
| `sandbox` | `internal/sandbox/` |
| `memory` | `internal/memory/` |
| `system` | `internal/system/` |
| `server` | `internal/server/` |
| `security` | `internal/security/` |
| `web` | `web/` |
| `deploy` | `deploy/` |

### Examples

```bash
git commit -m "feat(agent): implement multi-agent swarm delegation"
git commit -m "feat(plugin): implement WebSocket gateway syscalls in host_api"
git commit -m "fix(memory): prevent FTS5 index corruption on concurrent writes"
git commit -m "docs: add deployment guide for Docker mode"
git commit -m "refactor(llm): extract common retry logic into shared middleware"
git commit -m "test(auth): add token refresh daemon expiry edge cases"
```

---

## Common Tasks

### Adding a New LLM Provider

1. Create `internal/llm/<provider>.go`
2. Implement the `LLMProvider` interface from `provider.go`
3. Register in `router.go`
4. Add tests in `<provider>_test.go`
5. Update `docs/API.md` with the new provider option

### Adding a New Channel Adapter (via WASM Plugin)

In ActonOS, external chat channels (Telegram, Discord, Slack, WhatsApp, Zalo) are implemented as sandboxed WASM plugins using the **ActonOS Plugin SDK**:

1. Scaffold a channel plugin using the `acton-plugin` CLI or SDK:
   ```bash
   acton-plugin new my-chat-channel --type=channel
   ```
2. Implement `sdk.ChannelAdapter` (`SendMessage`, `PollMessages`, or WebSocket streaming with `ctx.WS()`).
3. Declare `manifest.json` capabilities (`"capabilities": ["channel"]`), permissions (`net_outbound`, `secrets`), and configuration schema.
4. Compile to WebAssembly and package:
   ```bash
   acton-plugin build
   acton-plugin pack -out dist/my-chat-channel.actonpkg
   ```
5. Install via Web UI (`Plugins` page) or upload via `POST /api/plugins/upload`. `WasmChannelBridge` will dynamically register the adapter with `ChannelManager` and route messages to `MessageRouter`.

### Adding a New API Endpoint

1. Add handler in the appropriate `internal/server/api_*.go` file
2. Register the route in `router.go`
3. Add request/response types
4. Write integration tests
5. Update `docs/API.md`

### Modifying the Agent Schema

1. Update `internal/agent/types.go`
2. Add migration logic in `internal/memory/db.go`
3. Update the Web UI agent form
4. Update `docs/ARCHITECTURE.md` with the new schema fields

### Adding or Changing a Tool

1. Register the tool in `internal/tools/`.
2. Add its deterministic risk class in `ToolRiskLevel`.
3. Ensure execution goes through `ToolRegistry.Execute`; never call the tool
   implementation directly from an API, cron, heartbeat, or channel path.
4. Add execution-time authorization, approval, path/network validation, and
   structured failure tests as applicable.
5. If the tool starts a process, require Docker/Bubblewrap and fail closed when
   strong isolation is unavailable.

### Developing Live Operations

1. Start the backend and sign in so the HttpOnly session cookie is present.
2. Open the `Live Operations` sidebar page.
3. Verify `/api/realtime` upgrades to WebSocket and reconnects after a daemon
   restart.
4. Set `ACTONOS_CANVAS_URL` only to a browser/VNC viewer intentionally exposed
   by the sandbox runtime.
5. Run `cd web && npx tsc --noEmit && npm run build`, then the Go server tests.

### Developing WASM Plugins (`internal/plugin/`)

ActonOS supports building plugins in any language compiling to WebAssembly (`wasip1` / `wasm32-wasi` or standard WASM).

#### Plugin Package Bundle (`.actonpkg`)
A production plugin package contains:
- `manifest.json`: Metadata, declared capabilities (`tool`, `channel`, `connector`), permissions, config schemas, and tool definitions.
- `plugin.wasm`: Compiled WebAssembly bytecode.
- `signature.sig`: Optional Ed25519 signature over SHA-256(`manifest.json` || `plugin.wasm`), produced by `acton-plugin sign`. Verified when present. Required only if `ACTONOS_REQUIRE_SIGNED_PLUGINS=1`.
- `README.md`: (Optional) Plugin user guide and documentation.

#### Guest Exports (WASM to Host)
- `acton_alloc(size: u32) -> u32`: Allocates linear memory buffer in guest.
- `acton_free(ptr: u32, size: u32)`: Deallocates linear memory buffer.
- `acton_plugin_init() -> i32`: Initializes plugin instance.
- `acton_tool_execute(name_ptr: u32, name_len: u32, args_ptr: u32, args_len: u32) -> u64`: Executes a declared tool (returns packed `ptr << 32 | len`).
- `acton_channel_send(msg_ptr: u32, msg_len: u32) -> i32`: Sends an outbound message to a recipient.
- `acton_channel_poll() -> u64`: Polls for buffered inbound messages (returns packed `ptr << 32 | len`).
- `acton_connector_handle_webhook(ptr: u32, len: u32) -> u64`: Dispatches incoming webhook payloads.
- `acton_plugin_shutdown() -> i32`: Clean teardown hook.

#### Host Syscalls (Host to WASM)
- **`acton_sys`**:
  - `log(level: i32, ptr: u32, len: u32)`: Emits structured logs.
  - `read_response(destPtr: u32, destLen: u32) -> i32`: Reads buffered host response.
- **`acton_net`**:
  - `http_request(req_ptr: u32, req_len: u32) -> u32`: Sandboxed HTTP requests validated against `permissions.net_outbound`.
- **`acton_ws`**:
  - `ws_connect(urlPtr, urlLen, hPtr, hLen) -> i32`: Establishes tracked WebSocket connection.
  - `ws_send(handleID, msgType, dataPtr, dataLen) -> i32`: Sends text/binary frames.
  - `ws_poll(handleID) -> i32`: Non-blocking poll for incoming WebSocket frames.
  - `ws_close(handleID) -> i32`: Closes active WebSocket connection.
- **`acton_vault`**:
  - `get_secret(key_ptr: u32, key_len: u32) -> u32`: Retrieves authorized secret credentials from the AES-256-GCM vault (not DMI/CPU bound).
- **`acton_storage`**:
  - `kv_get(key_ptr, key_len) -> u32` / `kv_set(k_ptr, k_len, v_ptr, v_len) -> i32`: Isolated SQLite key-value persistence.
- **`acton_bus`**:
  - `emit_event(topic_ptr, topic_len, payload_ptr, payload_len) -> i32`: Publishes system events onto ActonOS Event Bus.

### Frontend UX workflow

Use the shared UI primitives and the comfortable/compact density provider for every new route. Keep navigation grouped by workflow and use nested Agent Studio hashes for detail pages. Validate keyboard access, focus visibility, reduced motion, Vietnamese/English locale parity, and the authenticated browser matrix (390×844, 768×1024, 1440×900) before review.
Agent Studio sections expose dirty-state protection, Settings tabs preserve URL state, and Setup Wizard exposes an accessible progress indicator. New multi-step flows should follow the same patterns.
Authenticated Playwright coverage runs axe on every primary route, reloads hash-query views, and exercises the narrow-layout Chat session drawer. Allow route transitions to settle before evaluating computed contrast.


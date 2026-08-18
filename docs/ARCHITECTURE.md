# ActonOS Architecture

> **Comprehensive Technical Architecture Specification**
> for the ActonOS Extensible AI Agent Operating System Kernel

---

## Table of Contents

- [1. Design Philosophy & Foundational Principles](#1-design-philosophy--foundational-principles)
- [2. Computational Cognition Models](#2-computational-cognition-models)
- [3. Master System Architecture](#3-master-system-architecture)
- [4. Universal Agent Framework](#4-universal-agent-framework)
- [5. Dynamic Tooling Hub](#5-dynamic-tooling-hub)
- [6. Security, Sandboxing & Audit Logging](#6-security-sandboxing--audit-logging)
- [7. Disk Partitioning & Dual-Runtime Model](#7-disk-partitioning--dual-runtime-model)
- [8. Onboarding & Operational Lifecycle](#8-onboarding--operational-lifecycle)
- [9. Self-Healing OTA Update System](#9-self-healing-ota-update-system)

---

## 1. Design Philosophy & Foundational Principles

ActonOS is a **single-purpose appliance operating system** engineered as a customizable, self-governing AI agent kernel running 24/7. It does not hardcode roles or fixed tasks — instead, it provides an absolutely flexible infrastructure allowing users to create, configure, authorize, and extend any AI agent.

### Core Principles

| Principle | Description |
|:---|:---|
| **Single Static Binary** | The entire system — Agent Core, Web Server, Database, RAG Engine, Integration Hub, Web UI — compiles into a single static binary (`actond`) with `CGO_ENABLED=0`. |
| **Minimal Resource Footprint** | Idle RAM consumption of **20–40 MB**, Web UI boot time under **2 seconds** on Intel N-Series (N100/N95) or AMD Ryzen CPUs. |
| **Universal Agent Engine** | Unlimited agent creation. Users define persona, system prompt, tool bindings, delegation scopes, and LLM model per agent via Dashboard or REST API. |
| **Multi-Agent Swarm** | Agent-to-Agent delegation via Goroutines. A primary orchestration agent spawns specialized sub-agents for parallel long-running task chains. |
| **Dual-Runtime Model** | Hardware Abstraction Layer (HAL) auto-detects bare-metal (Wi-Fi, D-Bus, bwrap) vs. Docker (container metrics, jailed exec). |
| **Immutable OS** | Read-only system partition. All user data and agent configs reside in `/data`. Atomic OTA updates with watchdog auto-rollback prevent bricking. |
| **International Standards** | Deep integration with Model Context Protocol (MCP), OAuth 2.1 PKCE (S256), WebAssembly (WASM), and embedded Tailscale (`tsnet`). |

---

## 2. Computational Cognition Models

ActonOS provides a self-adjusting cognitive infrastructure for every user-created agent.

### A. Multi-Layered Memory Architecture

| Layer | Data Type | Storage Mechanism |
|:---|:---|:---|
| **Working Memory** | Scratchpad, current task state, temp variables, tool call results | In-memory (Goroutine context), freed on task completion |
| **User Profile Memory** | User profile, communication style, naming conventions, preferences | Auto-extracted via Async Reflection → Key-Value JSON + SQLite |
| **Procedural Memory** | Error handling history, optimized command sequences (best practices) | Stored as Workflow Patterns, injected into System Prompt on similar tasks |
| **Episodic Memory** | Past conversation/task journals with timestamps | SQLite FTS5 + Chromem-go vector indexing |

### B. Ebbinghaus Forgetting Curve Decay Model

Each memory fragment's retrieval score is computed using:

```
R(m, q, t) = α · D(t) · W(m) + β · CosSim(Embed(q), Embed(m))
```

Where:
- `D(t) = e^(-Δt/λ)` — Temporal decay factor (Δt = time since last retrieval, λ = decay rate)
- `W(m)` — Intrinsic importance weight assigned during extraction
- `CosSim(...)` — Cosine similarity between query and memory embeddings
- `α, β` — Normalized weights satisfying `α + β = 1`

### C. Uncertainty-Gated Decision Branching (POMDP)

Agents operate as adaptive POMDP systems based on decision entropy:

```mermaid
flowchart LR
    INPUT["Input Query"] --> ENTROPY{"Entropy H(p)"}
    ENTROPY -->|"H < θ (High confidence)"| GREEDY["Greedy ReAct\n1-Step Execution\n(Ultra-low latency)"]
    ENTROPY -->|"H ≥ θ (Complex/Ambiguous)"| LATS["Tree-of-Thoughts /\nLATS Search\n(Reward-optimized)"]
    GREEDY --> OUTPUT["Output"]
    LATS --> OUTPUT
```

### D. Calibrated Hybrid Retrieval (Sigmoid-Normalized Fusion)

Merges lexical search (SQLite FTS5) and semantic search (dense vector) scores using sigmoid normalization for optimal retrieval quality.

### E. Deterministic Verification System

| Tier | Method | Behavior |
|:---|:---|:---|
| **Tier 1** | Static Analysis (Pure Go, ~0ms) | AST parsing (Shell/Python/JSON/SQL), path escape detection, schema validation. Blocks immediately on violation. |
| **Tier 2** | Semantic Verification | Content consistency check against user profile and original request. Activated for language-logic tasks. |

---

## 3. Master System Architecture

```mermaid
graph TB
    subgraph "L1 — Connectivity & Interface"
        TSNET["Tailscale tsnet\nE2E Mesh VPN"]
        WEBUI["Web UI SPA\nReact 19 + Tailwind v4\nvia go:embed"]
        EVENTBUS["Event-Driven Bus\nTelegram · Discord\nSlack · MQTT · Webhooks"]
        CAPTIVE["Zero-Config Portal\nCaptive DNS Hijack\n192.168.4.1 / acton.local"]
    end

    subgraph "L2 — Dynamic Agent Framework"
        AGENTCFG["Universal Agent\nConfigurator\n(Unlimited Agents)"]
        SWARM["Swarm Orchestrator\nGoroutine Delegation"]
        ZEROTRUST["Zero-Trust\nScope-based Delegation\n(Grant Once → Autonomous)"]
    end

    subgraph "L3 — Enterprise Auth"
        OAUTH["OAuth 2.1 Provider\nPKCE S256 + DCR"]
        TOKENREFRESH["Token Refresh Daemon\n(Auto-renew 5min before expiry)"]
        SAAS["SaaS Connectors\nGmail · Drive · Calendar\nNotion · GitHub · Slack\nSalesforce · Databases"]
    end

    subgraph "L4 — Dynamic Tooling Hub"
        MCPHOST["MCP Host Engine\nstdio / SSE"]
        WASMRT["WASM Runtime\nwazero (Pure Go)"]
        SKILLDIR["Skill-as-a-Folder\nfsnotify hot-reload"]
    end

    subgraph "L5 — Acton Daemon Core"
        UNIFIED_BUS["Unified Event Bus\nAsync Channel Router"]
        REACT_ENGINE["ReAct Orchestrator\nPlan-and-Solve Loop\nToken Pruner"]
        MODEL_CASCADE["Model Cascade Router\nPrimary → Fallback (429)\n→ Local Ollama"]
        HYBRID_MEM["Hybrid Memory Engine\nFTS5 + Chromem-go\nEbbinghaus Decay + RRF"]
        VAULT["Hardware-bound Vault\nAES-256-GCM\nArgon2id + DMI UUID"]
    end

    subgraph "L6 — Hardware Abstraction Layer"
        BAREMETAL["Bare-metal Mode\nNetworkManager D-Bus\nBubblewrap + Cgroups v2\nHW Stats · OTA Watchdog"]
        DOCKERMODE["Docker Mode\nHost/Bridge Network\nWASM/Jailed Sandbox\nContainer Metrics API"]
    end

    subgraph "L7 — Base OS"
        DEBIAN["Debian 12 Minimal\n(Bare-metal)"]
        ALPINE["Alpine Linux\n(Container)"]
    end

    WEBUI --> AGENTCFG
    EVENTBUS --> UNIFIED_BUS
    CAPTIVE --> WEBUI
    TSNET --> WEBUI

    AGENTCFG --> SWARM
    SWARM --> ZEROTRUST
    ZEROTRUST --> OAUTH

    OAUTH --> TOKENREFRESH
    TOKENREFRESH --> SAAS

    AGENTCFG --> MCPHOST
    AGENTCFG --> WASMRT
    AGENTCFG --> SKILLDIR

    MCPHOST --> UNIFIED_BUS
    WASMRT --> UNIFIED_BUS
    SKILLDIR --> UNIFIED_BUS

    UNIFIED_BUS --> REACT_ENGINE
    REACT_ENGINE --> MODEL_CASCADE
    REACT_ENGINE --> HYBRID_MEM
    MODEL_CASCADE --> VAULT
    HYBRID_MEM --> VAULT

    UNIFIED_BUS --> BAREMETAL
    UNIFIED_BUS --> DOCKERMODE

    BAREMETAL --> DEBIAN
    DOCKERMODE --> ALPINE
```

### Tech Stack Reference

| Subsystem | Technology | Rationale |
|:---|:---|:---|
| Core Daemon | Go (CGO_ENABLED=0) | Single static binary, Goroutine concurrency, instant startup |
| Frontend | React 19 / Tailwind v4 / Vite | Embedded via `go:embed`, compressed bundle <2 MB |
| Remote Access | `tsnet` (Tailscale) | Embedded mesh VPN, E2E encrypted, no port forwarding |
| Tool Protocol | Model Context Protocol (MCP) | Open standard for tool integration via stdio/SSE |
| Plugin Runtime | wazero (WASM) | Pure Go WASM runtime, no CGO, sandboxed |
| Auth | OAuth 2.1 (PKCE S256) | Industry-standard SaaS authentication |
| Sandbox | Bubblewrap + Cgroups v2 | Namespace isolation, resource limits |
| Storage | modernc.org/sqlite + chromem-go | Embedded relational (FTS5) + vector search |
| Vault | AES-256-GCM + Argon2id | Hardware-bound encryption (DMI UUID + CPU serial) |
| Base OS | Debian 12 / Alpine Linux | Driver support (bare-metal) / minimal image (container) |

---

## 4. Universal Agent Framework

### A. Agent Schema Manifest

Each agent is declared via JSON/YAML or the Web Dashboard:

```json
{
  "agent_id": "agent_dev_assistant_01",
  "name": "Senior Software Architect",
  "description": "Expert in architecture analysis, code generation, and automated testing",
  "avatar_icon": "code-bracket",
  "model_config": {
    "primary_model": "anthropic/claude-3-7-sonnet",
    "fallback_model": "google/gemini-2.5-flash",
    "temperature": 0.2
  },
  "system_instructions": "You are a Senior Software Engineer. Always validate code syntax and run unit tests in the sandbox before responding...",
  "authorized_tools": [
    "mcp_github_*",
    "wasm_code_formatter",
    "skill_run_bash",
    "native_file_ops"
  ],
  "delegation_scope": {
    "max_monthly_budget_usd": 100.0,
    "allowed_workspace_paths": ["/data/workspace/project_alpha/"],
    "require_human_approval_level": "High"
  },
  "trigger_rules": [
    { "type": "channel_mention", "channel": "telegram", "filter": "@dev_bot" },
    { "type": "cron_schedule", "expression": "0 8 * * 1-5" }
  ]
}
```

### B. Multi-Agent Swarm Delegation

```mermaid
sequenceDiagram
    participant User
    participant Orchestrator as Orchestration Agent
    participant SubA as Sub-Agent A (Code)
    participant SubB as Sub-Agent B (Data)
    participant SubC as Sub-Agent C (Report)

    User->>Orchestrator: Complex request
    Orchestrator->>Orchestrator: Decompose into sub-tasks
    par Parallel execution
        Orchestrator->>SubA: Code analysis task
        Orchestrator->>SubB: Data processing task
        Orchestrator->>SubC: Report generation task
    end
    SubA-->>Orchestrator: Code results
    SubB-->>Orchestrator: Data results
    SubC-->>Orchestrator: Report draft
    Orchestrator->>Orchestrator: Merge & verify quality
    Orchestrator-->>User: Final consolidated response
```

### C. Autonomous Mission Control & Heartbeat Cognitive Pulse

```mermaid
graph TD
    PULSE["Heartbeat Daemon\n5m Cognitive Pulse\n(or Instant UI Trigger)"] --> READ_MD["Load Standing Directives\n(HEARTBEAT.md) & Backlog\n(TASKS.md / SQLite)"]
    READ_MD --> CHECK_TASK{"Pending / Active\nTasks in Backlog?"}
    
    CHECK_TASK -- "Yes (P0 -> P3)" --> RESUME_SESSION["Resume Dedicated Session\n(conv_task_<id>)\nLoad Working Memory Context"]
    RESUME_SESSION --> REACT_LOOP["Execute ReAct Step\nAuthorized Tools Sandbox"]
    REACT_LOOP --> PERSIST_STEP["Save Message in Session\nUpdate Progress (0-100%)\nSync to TASKS.md"]
    PERSIST_STEP --> PUSH_ALERT{"Task Complete or\nAction Needed?"}
    PUSH_ALERT -- "Yes" --> PROACTIVE_PUSH["Proactive Channel Push\n(Telegram, Discord, WhatsApp)\nAnti-Double-Dispatch Guard"]
    PUSH_ALERT -- "No" --> AUDIT_RECORD["Record Pulse Run in SQLite"]
    
    CHECK_TASK -- "No (Backlog Clean)" --> ZERO_NOISE{"System Nominal?"}
    ZERO_NOISE -- "Yes" --> HB_OK["Return HEARTBEAT_OK\nZero Noise Policy\n(No Channel Spam)"]
    ZERO_NOISE -- "Alert" --> PROACTIVE_PUSH
```

- **Working Memory Continuity**: Automatically preserves dialogue and intermediate thoughts inside SQLite `chat_sessions` per task ID (`conv_task_<id>`), allowing multi-step task execution without losing progress across pulses.
- **Bi-directional Synchronization**: Changes in the Web UI, REST API, or Agent ReAct steps automatically synchronize between SQLite and `data/workspace/TASKS.md` and `data/workspace/HEARTBEAT.md`.
- **Zero-Noise Guarantee**: If all systems are nominal and no task needs human escalation, the kernel records the run in SQLite and remains completely silent without sending spam to external messaging channels.
- **Durable Execution State**: Every engine invocation creates an `agent_runs` record
  and append-only `run_events`. A W3C-sized trace ID correlates LLM attempts, tool
  observations, approval pauses, token totals, and termination reasons.
- **Bounded Self-Healing**: Tool failures are returned as structured observations.
  The ReAct loop can repair and retry, but stops after repeated identical observations,
  three consecutive tool failures, eight iterations, cancellation, budget exhaustion,
  or deterministic verification failure.
- **Verified Completion**: Heartbeat completion markers are advisory. The verifier
  rejects completion claims containing failed/blocked observations or action-oriented
  missions that produced no tool evidence.
- **Unified Delegation Kernel**: Swarm sub-tasks are routed through the same Engine
  whenever available, inheriting durable runs, context budgets, approvals, tools,
  verification, and termination guards.

---

## 5. Dynamic Tooling Hub

```mermaid
graph LR
    subgraph "Tool Registry"
        direction TB
        T1["Tier 1: MCP Host Engine\nstdio (local binary)\nSSE (remote/Internet)"]
        T2["Tier 2: WASM Runtime (wazero)\n/data/plugins/*.wasm\nSandboxed in-memory execution"]
        T3["Tier 3: Skill-as-a-Folder\n/data/skills/<name>/\nskill.json + run.sh/run.py\nfsnotify hot-reload"]
    end

    LLM["LLM Tool Call"] --> T1
    LLM --> T2
    LLM --> T3
```

### OAuth 2.1 & Token Refresh Daemon

All SaaS connections (Gmail, Notion, Figma, GitHub) authenticate via OAuth 2.1 with PKCE (S256). The `token_refresher.go` daemon automatically renews access tokens **5 minutes before expiry** to maintain seamless connectivity.

---

## 6. Security, Sandboxing & Audit Logging

### A. Command Execution Sandbox

When an agent executes shell commands on bare-metal:

Execution is fail-closed:

- Linux bare-metal requires Bubblewrap.
- Docker mode may use the container boundary and a restricted subprocess.
- Other platforms reject `native_exec` unless the operator explicitly enables
  `ACTONOS_ALLOW_INSECURE_EXEC=1` for local development.
- Linux bare-metal executions are attached to a dedicated cgroup v2 with
  `memory.max`, `pids.max`, and `cpu.max`; inability to create or configure the
  cgroup blocks execution.

**Namespace Isolation (Bubblewrap):**
```bash
bwrap \
  --ro-bind /usr /usr \
  --ro-bind /bin /bin \
  --ro-bind /lib /lib \
  --ro-bind /lib64 /lib64 \
  --proc /proc \
  --dev /dev \
  --unshare-all \
  --die-with-parent \
  --cap-drop ALL \
  --bind /data/workspace /workspace \
  --setenv PATH "/usr/bin:/bin:/data/bin" \
  --chdir /workspace \
  bash -c "<agent_command>"
```

File operations use canonical path resolution, `filepath.Rel` containment, and
symlink escape prevention. HTTP fetch validates DNS results and blocks loopback,
private, link-local, multicast, and metadata-network targets on every redirect.

### B. Risk-Based Approval Matrix

| Risk Level | Example Actions | Handling |
|:---|:---|:---|
| **Low** | Workspace read/list/search, system information | Auto-execute when authorized |
| **Medium** | Network fetch/navigation, external notification | Approval when agent threshold is `Medium` or stricter |
| **High** | Command execution, file write/delete, cron mutation, MCP/WASM actions | Durable exact-action approval |

`RequireHumanApproval` is a threshold: `Low` approves every action, `Medium`
auto-runs Low only, and `High` approves High actions only. Authorization and
`AllowedWorkspacePaths` are re-evaluated at execution time, not merely when tool
definitions are sent to the model.

### C. Audit Logging and Run Tracing

All execution history is recorded in structured JSON-lines at `/data/logs/audit.jsonl`:

```json
{
  "timestamp": "2026-08-16T23:55:00Z",
  "trace_id": "9a8b7c6d5e4f3210123456789abcdef0",
  "agent_id": "agent_dev_assistant_01",
  "tool_name": "skill_run_bash",
  "risk_level": "Medium",
  "execution_time_ms": 142,
  "status": "Success"
}
```

SQLite `agent_runs` and `run_events` provide queryable end-to-end traces through
`GET /api/runs` and `GET /api/runs/{id}/events`. Prometheus metrics include total
token cost and `actonos_eventbus_dropped_total` for backpressure visibility.
Audit JSONL entries also form a SHA-256 hash chain through `previous_hash` and
`entry_hash`, allowing tampering to be detected with `AuditLogger.VerifyChain`.

OpenAI-compatible chat providers use real upstream SSE streaming. Deltas are
forwarded immediately while fragmented tool-call arguments are reassembled before
execution.

---

## 7. Disk Partitioning & Dual-Runtime Model

### A. Bare-metal MiniPC Partitioning

The system auto-formats three partitions during USB installation:

```
[Drive: /dev/nvme0n1 or /dev/sda]
├── Partition 1: ESP (512 MB, FAT32) ──► /boot/efi (UEFI Bootloader)
├── Partition 2: System Root (4 GB, Ext4) ──► / (READ-ONLY: Kernel, Base OS, bwrap)
└── Partition 3: User Data (remaining) ──► /data (READ-WRITE, auto-expands)
    ├── bin/           Symlink to active actond build
    ├── releases/      /v1.0.0/actond, /v1.0.1/actond ...
    ├── config/        vault.db (encrypted API keys, user settings)
    ├── agents/        agent_manifests.json (user-created agents)
    ├── tokens/        oauth_tokens.vault (encrypted OAuth tokens)
    ├── storage/       app.db (SQLite chat logs, FTS5, vector index)
    ├── logs/          audit.jsonl (OpenTelemetry structured logs)
    ├── overrides/     Custom Web UI / prompt overrides
    ├── plugins/       WASM plugin files (.wasm)
    ├── skills/        Skill script folders (JSON + Shell/Python)
    ├── mcp-servers/   MCP server configs and binaries
    └── workspace/     Isolated agent read/write environment
```

### B. Docker Container Mode

```bash
docker run -d \
  --name actonos-agent \
  -p 8080:8080 \
  -v /local/acton-data:/data \
  -e RUNTIME_MODE=docker \
  --restart unless-stopped \
  actonos/actonos:latest
```

---

## 8. Onboarding & Operational Lifecycle

```mermaid
flowchart TD
    A["Power On / Container Start"] --> B["actond process starts"]
    B --> C{"Environment Detection"}
    C -->|Docker| D["Open Web UI on port 8080\nAccept config via UI or .env"]
    C -->|Bare-metal| E{"Config exists at\n/data/config/vault.db?"}
    E -->|Yes| H
    E -->|No / Network down >60s| F["Enable Wi-Fi Hotspot:\nActonOS-XXXX\nGateway: 192.168.4.1"]
    F --> G["Captive Portal Setup Wizard:\n1. Select home Wi-Fi\n2. Enter LLM API keys\n3. OAuth 1-click SaaS connect\n4. Tailscale auth key (optional)\n5. Set admin PIN"]
    G --> G2["Save config to encrypted Vault\nDisable Hotspot\nConnect to home Wi-Fi"]
    D --> H
    G2 --> H["Operational State:\n✓ LAN / Ethernet connected\n✓ mDNS: http://acton.local\n✓ Tailscale mesh (tsnet)\n✓ Agent Engine started\n✓ MCP Servers loaded\n✓ Web Dashboard ready"]
```

---

## 9. Self-Healing OTA Update System

```mermaid
sequenceDiagram
    participant OTA as OTA Engine
    participant FS as Filesystem
    participant Service as systemd
    participant WD as Watchdog

    OTA->>FS: Download new build to /data/releases/vX.Y.Z/
    OTA->>OTA: Verify SHA256 checksum + GPG signature
    OTA->>FS: Atomic symlink swap: /data/bin/actond → new build
    OTA->>Service: systemctl restart actond
    Service->>WD: Start health monitoring

    loop Every 5s for 30s
        WD->>Service: GET http://127.0.0.1:8080/api/health
        alt 200 OK
            WD->>FS: Mark new version as stable ✓
        else Crash/Hang (>3 failures)
            WD->>FS: Revert symlink to previous build
            WD->>Service: systemctl restart actond
        end
    end
```

---

## 10. Durable Autonomous Execution

- `Planner.ExecutePlan` validates dependency-aware DAG plans and rejects duplicate nodes, unknown dependencies, and cycles.
- `agent_runs.checkpoint_json` stores messages, aggregate usage, iteration, and the pending tool call whenever approval pauses execution.
- `Engine.ResumeApproved` resumes the same run from its checkpoint without repeating completed actions.
- Context compaction writes provenance-bearing records to `context_snapshots`.
- OpenAI-compatible, Anthropic, and Gemini providers stream live SSE deltas and reconstruct fragmented tool arguments.

## 11. MCP Lifecycle

MCP definitions are persisted in `mcp_servers`; environment values are encrypted in Vault under `mcp.env.<id>`. Enabled servers are restored at startup. Supported transports are isolated `stdio` and remote `http`/`sse` JSON-RPC. Tool-name collisions roll back registration.

All persistent server paths originate from the configured data root. `main`
passes `DataDir`, `WorkspaceDir`, `SkillsDir`, and `WASMDir` into the HTTP
server, so API operations remain scoped correctly when `--data-dir` is changed.

LLM provider credentials are resolved from the encrypted Vault before provider
registration. Only provider metadata is stored on disk. Backup generation uses
SQLite `VACUUM INTO`, producing a consistent snapshot that includes committed
WAL transactions.

## 12. Realtime Frontend Operations

`api_realtime.go` is the read-only realtime aggregation boundary. It upgrades a
same-origin authenticated request to WebSocket and publishes periodic snapshots
of HAL telemetry, Docker state, durable runs, pending approvals, and token
usage. Detailed ordered run events remain sourced from the durable
`run_events` table.

The server-side realtime hub caches one aggregate snapshot for 1.5 seconds so
concurrent browser sessions do not multiply Docker, SQL, and sensor collection.
The React application mounts one `RealtimeProvider`; Header, Operations,
approval interruption, and cost displays consume that shared state. The
provider rejects malformed frames and reconnects with bounded exponential
backoff and jitter.

The Operations UI deliberately separates observation from execution:

- xterm.js renders run events as a read-only terminal.
- sensitive decisions call the durable approval REST endpoints.
- Live Canvas embeds only the URL explicitly published by the sandbox runtime
  through `ACTONOS_CANVAS_URL`. Relative paths, HTTPS URLs, and HTTP loopback
  URLs are accepted; remote plaintext HTTP and protocol-relative URLs are
  rejected. The iframe is sandboxed and sends no referrer.
- interactive commands never bypass `ToolRegistry.Execute` or the sandbox.

Primary UI pages use hash routes and lazy loading. This preserves deep links and
browser history in the embedded `go:embed` deployment while keeping editor and
xterm dependencies outside the application entry chunk.

## References

1. [Model Context Protocol — GitHub](https://github.com/modelcontextprotocol)
2. [MCP Go SDK — pkg.go.dev](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)
3. [Bubblewrap — Debian manpage](https://manpages.debian.org/unstable/bubblewrap/bwrap.1.en.html)
4. [OAuth 2.1 — IETF Draft](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-11)
5. [wazero — Pure Go WASM Runtime](https://github.com/tetratelabs/wazero)
6. [chromem-go — Embeddable Vector Database](https://github.com/philippgille/chromem-go)
7. [modernc.org/sqlite — Pure Go SQLite](https://pkg.go.dev/modernc.org/sqlite)

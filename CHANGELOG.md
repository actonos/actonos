# Changelog

All notable changes to ActonOS will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]


## [1.0.4] - 2026-08-31

### Added

- **B1. Tier-3 Verification: Outcome Assertions & Grounding (`internal/agent/verifier_outcome.go`, `internal/agent/verifier.go`)**:
  - Deterministic multi-type outcome assertion engine supporting 8 verification kinds (`file_exists`, `file_contains`, `json_schema`, `http_status`, `sql_count`, `shell_exit`, `dir_not_empty`, `llm_judge`).
  - Integrated outcome assertions directly into `PlanStep` DAG execution and autonomous task completion validation, eliminating false completion claims.
- **B2. Ebbinghaus Weighted Decay, Importance Tiers & Memory Pinning (`internal/memory/db.go`, `internal/memory/decay.go`, `internal/memory/hybrid.go`, `/api/agents/{agentID}/memories*`)**:
  - SQLite schema migration adding `importance`, `pinned`, `user_pinned`, and `demoted_at` columns.
  - Multi-tier memory retention rules (`critical`, `user_preference`, `high`, `normal`, `low`) ensuring pinned and critical user preferences are immune to deletion or temporal decay.
  - CRUD API for memory listing, permanent pinning, and importance tier assignment.
- **B3. Smart Cascade Router: Task-Kind & Cost/Latency Aware Routing (`internal/llm/router.go`, `/api/llm/health*`, `/api/llm/router/retune`)**:
  - Per-task-kind telemetry tracking (`reasoning`, `coding`, `summarize`, `classify`, `extract`, `general`) with moving average p50/p95 latency and failure statistics.
  - Dynamic cascade re-ordering based on pricing catalog to prioritize cost-effective models on lightweight extraction/classification workloads.
  - Nightly and on-demand self-diagnostic health probe and router retune endpoints.
- **B4. Tool Result Auto-Summarization & Provenance Snapshots (`internal/agent/context.go`, `internal/agent/engine.go`)**:
  - Automatic summarization of oversized tool observations (>8,000 chars) retaining critical facts, paths, and identifiers.
  - Full raw observation payloads persisted in `context_snapshots` with reference token links (`view_full:<run_id>:<snap_id>`).
- **B5. Standardized Eval Suite & CI Regression Detection Gate (`evals/`, `.github/workflows/eval.yml`)**:
  - 30 comprehensive standardized benchmark tasks across coding, planning, tool usage, memory retrieval, safety policy, and Vietnamese language.
  - 3-tier automated grading engine (deterministic assertion checker, outcome verifier, and LLM-as-a-judge rubric).
  - Standalone CLI runner with P50/P95 latency, pass rate, false completion rate, and token cost reporting wired into GitHub Actions.
- **D1. Mobile-First PWA & Responsive Navigation Overhaul (`web/public/manifest.json`, `MobileBottomNav.tsx`, `PWAInstallBanner.tsx`)**:
  - Standalone Web App Manifest with full mobile icons, theme color, display standalone, and cover viewport support.
  - Sticky glassmorphic mobile navigation bar with real-time unread alerts and pending approval badges.
  - Smart PWA installation banner supporting both native `beforeinstallprompt` (Chrome/Android) and iOS Safari "Add to Home Screen" instructions.
- **D2. Mission Timeline & DAG Gantt Trace Inspector (`MissionTimelineView.tsx`, `MissionsPage.tsx`)**:
  - Interactive unified pulse timeline visualizing continuous autonomous heartbeat cycles and agent runs.
  - Visual DAG Gantt step status bar showing completed, in-progress, pending, and failed step execution.
  - Click-through inspection modal with full JSON payloads, W3C trace IDs, and single-click Markdown execution report generation.
- **D3. Agent Templates Marketplace (`internal/agent/templates.go`, `TemplateGalleryModal.tsx`, `AgentStudioPage.tsx`)**:
  - 15+ built-in production agent templates across 5 categories (Development, Operations, Productivity, Security & Compliance, Analysis & Intelligence).
  - Categorized template gallery modal with live search, tool allowlist previews, and one-click clone preload into Agent Studio.
- **D4. Disaster Recovery, Backup & Restore System (`internal/system/backup.go`, `BackupRestoreSection.tsx`, `SettingsPage.tsx`)**:
  - Transactional snapshot backup bundles (`.actonbak` format) leveraging SQLite `VACUUM INTO` and SHA-256 integrity verification.
  - Automated safety pre-restore snapshot generation before applying restored databases or archives.
  - Dual-gate factory reset workflow with confirmation token `RESET-ACTONOS`.
- **D5. Audit Log Explorer & Search Engine (`internal/system/audit.go`, `AuditLogsPage.tsx`)**:
  - Multi-criteria database search engine supporting full-text query, agent ID, risk level, execution status, tool name, and time range.
  - Real-time Live Tail mode with 3s silent auto-polling, quick filter presets (High-Risk Only, Failures, High Latency >1s), and streaming CSV/JSON export.
- **D6. Operator Health Dashboard (`OperatorHealthView.tsx`, `DashboardPage.tsx`)**:
  - High-level 0-100 system health gauge computing real-time vitality across active anomalies, provider circuit breakers, and host resource telemetry.
  - 4-card vitals strip (OpenClaw Heartbeat, LLM Provider Mesh, RAM/CPU load, Agent Fleet).
  - Proactive system anomaly banner with one-click autonomous diagnostic task creation.
- **D7. Smart Notifications & Fatigue Reduction (`internal/system/notifications.go`, `NotificationPreferencesModal.tsx`, `NotificationsPage.tsx`)**:
  - Configurable Quiet Hours schedule with 400+ IANA timezone support, muting non-critical push notifications during focus/sleep hours.
  - Automated 24-hour Daily Executive Digest aggregating task completions, failures, and token expenditure into a single morning briefing.
  - Minimum push notification severity filter (`info`, `warning`, `critical`).

## [1.0.3]

### Added

- **A1. Proactive Anomaly Engine & Operations UI (`internal/agent/proactive.go`, `/api/ops/anomalies*`)**:
  - Continuous 7-probe system diagnostic scanning during idle heartbeat cycles (Disk usage, Certificate/token expiry, Overdue embedding queue, Degraded MCP servers, Stalled tasks, High token consumption >80%, Inbound message queue backlog).
  - Operations tab with interactive Proactive Anomaly card, manual scan triggers, configuration modal (`ProactiveConfigModal.tsx`), and automated one-click mission generation (`AutoTaskPayload`).
- **A2. Risk-Based Governance & Visual Approvals (`internal/tools/risk.go`, `internal/tools/approval.go`)**:
  - Granular 3-tier classification (`RiskTierLow`, `RiskTierMedium`, `RiskTierHigh`) with safety blacklist (exec, delete, restart, ota, vault, cron) guaranteeing dangerous operations never auto-approve.
  - Interactive approval interruptions and approval cards in Chat and Operations with animated countdown indicators, risk level badges, and tool call parameter inspection.
- **A3. Concurrent Burst Pulse for DAG Execution (`internal/agent/planner.go`, `internal/agent/engine.go`)**:
  - Multi-step readiness resolution (`ReadySteps`) and bounded concurrent execution of independent DAG steps in parallel Goroutines.
  - Per-agent burst concurrency slider and badges in Agent Studio and Agent Cards (`max_concurrent_runs`).
- **A4. Structured Standing Directives & Automated Outcome Assertion (`internal/agent/directive_verifier.go`, `internal/agent/tasks.go`)**:
  - Schema-driven directives with deterministic assertion rules (`file_exists`, `file_contains`, `dir_not_empty`, `http_status`) and validation feedback in Agent Studio.
- **A5. ReflectionEngine Self-Review & Insights Hub (`internal/agent/reflection.go`, `/api/agents/{agentID}/insights*`)**:
  - Automated 24h run telemetry and tool reliability evaluation generating self-improvement proposals, persistent SQLite logging (`self_improvement_proposals`), and human-readable insights in `/data/agents/{agent_id}/INSIGHTS.md`.
  - Dedicated Agent Insights modal (`AgentInsightsModal.tsx`) with accept/reject proposal workflow, tool success rates, and direct prompt improvements.

### Fixed

- Vitest created `node_modules` folder outside of the `web` folder
- **Model Output Sanitization & Glitch Suppression**:
  - Automatically strip `functions.` prefixes and map aliases in `NormalizeToolName` (`internal/tools/registry.go` and `internal/llm/sanitize.go`).
  - Extracted `to=functions.<name>` and `{"tool_uses": [...]}` embedded JSON tool call structures from models (Qwen, Hermes, DeepSeek).
  - Suppressed real-time SSE streaming of markup tags, token loop keywords, and raw JSON wrappers during generation.
- **Autonomous Mission Resilience & Plan Deadlock Recovery**:
  - Categorized temporary resource limits (`agent hourly token quota exhausted`, rate limits `429`/`503`, network timeouts, context cancellations) as transient errors via `isTransientExecutionError`, maintaining step status in `StepStatusPending` with notices instead of marking them permanently `failed` or tripping missions into `blocked` state.
  - Operator Task Reset: Updating a task to `progress: 0` or moving status to `pending`/`in_progress` automatically reopens all or failed steps in the persisted `plan_json` (`ReopenAllSteps()`, `ReopenFailedSteps()`), reset `FailCount = 0` and `StalledCycles = 0`, and clears the in-memory stall tracker.
- **Agent Process Run Resilience (Elimination of Stuck 'running' Runs)**:
  - Ensured `finishRun` executes SQLite state updates using an independent context (`context.WithTimeout(context.Background(), 5*time.Second)`), preventing canceled or timed-out turn contexts from dropping terminal status updates.
  - Added guaranteed `defer` run finalizer across `ExecuteStepWithHistory` and `ExecuteStepStreamWithHistory` to ensure no run is left in `RunRunning` upon exit, error, timeout, or panic.
  - Added background stale run reaper (`ReclaimStaleRuns` in `RunStore` & `Engine`) automatically cleaning up runs older than 10 minutes in `running` status during `ListFiltered` API calls and periodic `HeartbeatDaemon.RunPulse` sweeps.

## [1.0.2] - 2026-08-27

### Changed

- Implement agent tool approval workflow with UI support and persistence in Chat

### Fixed

- Kernel permission denied when trying to create a foldẻ in /sys/fs/cgroup
- New chat session missed the first message
- Agent loop sometime doesn't save the message if the run is too long
- Tool call result exceeds the context window

## [1.0.1] - 2026-08-26

### Added
- GitHub-backed Auto Update: the daemon fetches `GET https://api.github.com/repos/actonos/actonos/releases/latest` (JSON, never the HTML `/releases` page), compares Canonical SemVer as-is, and can download `actond` plus `embeddingd` into `{dataDir}/releases/{version}/`. Linux uses symlink activate; native Windows copies into `{dataDir}/bin/` then parent-wait restarts. Docker and Darwin remain check-only. Apply/rollback are High-risk approvals that enqueue so the HTTP request returns immediately. A 24h ticker emits one notification per new `latest_version`.
- Token Ledger & Cost is a System sidebar destination at `#/costs` (including `#/costs?view=transactions`) with a dedicated `costs` locale. `#/settings?view=tokens` still opens the ledger. Live Operations `?view=cost` stays a compact summary with a link to the full ledger.
- Header agent-activity chip on every authenticated page, fed by the existing `/api/realtime` snapshot (running runs unioned with the recent 30). Opening a row goes to `#/operations?view=feed&run={id}` and fetches that run if it is not in the snapshot.
- Tag-triggered GitHub Actions release workflow that publishes `{actond|embeddingd}_v{version}_{x86_64|arm64}[.exe]` plus `SHA256SUMS`, and fails if Windows `embeddingd.exe` is missing.

### Changed
- Settings Maintenance OTA card reports GitHub `error_code` honestly (rate limit is not “up to date”), shows Install only when `canInstall`, and can roll back the previous binary.
- `embeddingd.service` prefers `/var/lib/acton/bin/embeddingd` when present, matching `actond.service`.

### Fixed
- Chat “New Chat” no longer toasts `conversation not found`: draft sessions use `?session_id=new` and do not GET until the server creates the row.
- Markdown images no longer leave an empty `<p>` above the figure (marked wraps `<img>` in a paragraph; TipTap lifts block images).
- Operations deep-link `#/operations?view=feed&run=` keeps a fetched run in the feed instead of showing empty when the run fell out of the realtime snapshot.
- `changelog-gen.sh` no longer dies on `)` in the conventional-commit regex, and no longer drops the last commit (`tformat`).

## [1.0.0] - 2026-08-25

### Security
- Plugin egress now reuses SSRF validation (no loopback/private/metadata, no `*`/`*.com`, redirect re-check). Plugin `signature.sig` is optional (matches `acton-plugin pack`); when present it is verified as Ed25519 over SHA-256(manifest||wasm). Pairing codes are 8-character `/pair`-only with lockout. Admin passwords use Argon2id (min 8) with login lockout; query-string tokens are ignored. Webhook recipient URLs are validated. Plugin logs redact vault secrets. Chat markdown strips `javascript:` links.

### Changed
- Channels UI is the pairing/accounts surface for installed WASM chat plugins (not restored native adapters). Coverage floors are enforced by `scripts/cover-gate.go` from `make test-unit`. Integration Makefile target runs `./internal/...` with the `integration` tag.

### Added
- **Unified WasmLoader Plugin Architecture & Sandboxed Extensions**:
  - Polyglot plugin execution engine (`internal/plugin/`) powered by Wazero (100% pure Go, zero CGO, `CGO_ENABLED=0` static binary compliant).
  - Unified plugin model consolidating **Tools**, **Chat Channels**, and **SaaS Connectors** into portable `.actonpkg` package bundles with `manifest.json`, `plugin.wasm`, and optional Ed25519 `signature.sig` from `acton-plugin sign` (verified when present).
  - Packaged Plugin Upload: Streamlined UI to directly accept `.actonpkg` bundles created with the ActonOS Plugin SDK (`acton-plugin pack`), removing legacy in-browser starter scaffolding.
  - Granular Host Syscall modules: `acton_sys` (structured logging and response streaming), `acton_net` (sandboxed HTTP egress with domain whitelisting), `acton_ws` (real-time WebSocket gateway for streaming channel protocols), `acton_vault` (AES-256-GCM vault brokering, not DMI/CPU bound), `acton_storage` (scoped SQLite key-value persistence), `acton_bus` (event bus publisher), and `acton_host` (legacy compatibility module).
  - Bridges: `WasmToolBridge` for `tools.ToolRegistry` (enforcing Single Execution Boundary), `WasmChannelBridge` for `channels.ChannelManager`, and `WasmConnectorBridge` for SaaS integrations.
  - Lifecycle management: Hot-loading, runtime enable/disable, dynamic uninstallation, log telemetry, configuration update, and vault secret persistence via `/api/plugins/*`.
  - Comprehensive documentation across `ARCHITECTURE.md`, `API.md`, `SECURITY.md`, `DEVELOPMENT.md`, `CONTRIBUTING.md`, and `.agents/rules/source-registry.md`.
  - Complete TypeScript types (`PluginInfo`, `PluginManifest`, `PluginCapability`, etc.) and English / Vietnamese localization namespaces (`plugins.json`).
  - Dedicated `MessageRouter` (`internal/channels/router.go`) decoupled from daemon entrypoint to coordinate multi-account inbound dispatch across Telegram, Discord, and WhatsApp.
  - Inbound message agent mention parsing (`ExtractAgentMention`) supporting `@agent_name` and `/agent <name>` commands in group chats and private DMs.
  - Channel account routing modes: `exclusive` (assigned agent only), `mention` (route by @mention with single-agent fallback), and `fallback` (default to `agent_system_core`).
  - Deterministic agent-aware conversation session IDs (`conv_{channel}_{sender}_{agentID}`) ensuring isolated memory context and chat histories across different agents talking to the same user.
- **Official Available Plugins Registry & 1-Click WASM Installer**:
  - Live remote registry integration (`internal/plugin/registry.go`) fetching official curated pre-built WASM plugins directly from GitHub release downloads (`https://github.com/actonos/plugin-sdk/releases/latest/download/plugin-registry`) with 1-hour TTL caching, concurrent synchronization, and built-in offline fallback catalog.
  - Real-time 1-Click installation pipeline (`POST /api/plugins/install`) downloading `.actonpkg` bundles or `.wasm` binaries, unpacking contents, verifying security sandbox permissions, hot-reloading into Wazero runtime, and emitting `bus.EventPluginProgress` and `bus.EventPluginInstalled` events.
  - Modern Available Plugins UI tab in `PluginsPage.tsx` synchronized with the Skills page UX, featuring category badges, status segmentation (`All`, `Installed`, `Available`), star/recent/name sorting, multi-step progress bar (`useActionProgress`), and `PluginHubDetailModal.tsx` for deep inspection of capabilities, tools, and schemas before installing.

### Removed
- **Legacy Native Channel Implementations & Standalone Pages**:
  - Removed native hardcoded channel adapters (`internal/channels/discord.go`, `internal/channels/telegram.go`, `internal/channels/whatsapp.go`).
  - Removed native connector tools (`internal/tools/connector_tools.go`, `internal/tools/connector_tools_test.go`).
  - Removed standalone Channels and Connectors UI pages (`web/src/pages/Channels/*`, `web/src/pages/Connectors/*`) and deprecated locale files (`channels.json`, `connectors.json`, `integrations.json`).
  - Removed legacy OAuth and WhatsApp webhook handlers from `internal/server/api_integrations.go`.

### Changed
- **Unified Plugins & Extensions Navigation**:
  - Replaced legacy Channels and Connectors pages with the unified, modern **Plugins Hub** (`/plugins`, `PluginsPage.tsx`).
  - Unified configuration, secrets management, live log streaming, and package upload into `PluginDetailModal.tsx`, `PluginUploadModal.tsx`, and `PluginLogsModal.tsx`.
- **Per-Agent Autonomous Heartbeat & Isolated Standing Directives**:
  - Dedicated `AgentHeartbeatConfig` embedded in `AgentManifest` supporting per-agent standing directives, pulse intervals (5m - 24h), target alert channels (`telegram`, `discord`, `whatsapp`, `webhook`, `none`), target account IDs, and active hours windows.
  - Heartbeat daemon evaluation loop (`checkCustomAgentPulses`) executing autonomous cycles for active custom agents with Zero-Noise classification and audit history.
  - New **Heartbeat** configuration tab in Agent Studio (`AgentHeartbeatSection.tsx`) with Markdown standing directives editor, schedule interval picker, alert destination channel, active hours scoping, and status badge.
  - Streamlined `MissionsPage.tsx`: Removed the legacy global directives tab, refocusing the Missions page on Task Backlog, Pulse Audit Ledger, and Human-in-the-loop Governance approvals.
- **Dynamic Community Skills Registry, Requirements Verification & Skill Management**:
  - Live fetching of community skills from official GitHub registry (`https://raw.githubusercontent.com/actonos/actonos-skills/refs/heads/master/registry.json`) with multi-file package downloads and 1-hour TTL caching.
  - Metadata `requires` verification (`env`, `bins`, `os`, `config`) with execution gating and LLM prompt filtering to prevent broken tool invocations.
  - Enable / Disable skill toggle API (`PUT /api/tools/skills/{name}/toggle`) with persistent `.disabled` filesystem state.
  - Fully modernized Community Skills Hub UI with dynamic category pills, installation status filters, popularity/stars sorting, multi-field search, inspect modal, and complete English/Vietnamese localization.
- **Service Worker & Web Push Background Notifications**:
  - Full Web Push API (RFC 8030 / RFC 8291 / RFC 8292) with auto-generated P-256 VAPID keypairs and SQLite subscription persistence.
  - Dedicated Service Worker (`sw.js`) handling background wake-ups, interactive action prompts (Review / Dismiss), vibration patterns, and active tab navigation/focus.
  - Push subscription endpoints (`/api/notifications/push/*`) with immediate test dispatch capabilities and automatic cleanup of expired (HTTP 410/404) subscriptions.
- **Realtime Frontend Operations Center**:
  - Responsive hardware and Docker telemetry, collapsible Thought → Action → Observation feed, Live Canvas, and read-only xterm.js observation terminal.
  - Background task pause/resume/retry/cancel controls and cron pause/resume actions.
  - Global sensitive-action approval interruption with approve or reject-with-feedback workflows.
  - MCP server list/toggles, stdio/HTTP/SSE configuration, and encrypted environment entry from the UI.
  - Daily/monthly/per-model token and cost tracking in the realtime command center.
  - Workspace side-by-side file diff viewer and remote-alert integration through existing Telegram, Discord, WhatsApp, and Slack configuration surfaces.
  - Self-hosted Manrope and Noto Sans variable fonts for modern multilingual rendering.
  - Protected `/api/realtime` WebSocket, Docker container telemetry, and MCP administration endpoints.
- **Secure Autonomous Execution Kernel**:
  - Durable `agent_runs`, `run_events`, and exact-action `approvals` ledgers with correlated trace IDs and termination reasons.
  - Execution-time tool authorization, workspace path scopes, monthly agent budget enforcement, bounded retries, no-progress detection, and deterministic completion verification.
  - Human approval REST workflow (`/api/approvals`) and run tracing APIs (`/api/runs`).
  - Fail-closed Bubblewrap/Docker command isolation, MCP process isolation policy, SSRF protection, symlink-safe workspace resolution, and non-zero exit classification.
  - Context token budgeting, autonomous plan injection, reflection secret redaction, heartbeat execution locking, and EventBus dropped-event metrics.
  - Real OpenAI-compatible SSE streaming with fragmented tool-call reconstruction.
  - Real Anthropic and Gemini SSE streaming with tool-use and usage reconstruction.
  - Linux cgroup v2 CPU, memory, and process enforcement for Bubblewrap executions.
  - SHA-256 chained audit records, shared swarm execution kernel, periodic memory deduplication/retention, and Mission Control approval/run governance UI.
  - Durable checkpoint resume for approval-paused runs and dependency-aware DAG plan execution.
  - Persistent MCP lifecycle with encrypted environments, startup restore, and HTTP/SSE JSON-RPC.
  - Unified exact-action approval for workspace, skill, WASM, Tool Hub, and restart mutations.
  - Provenance-bearing context snapshots and online audit-chain verification.
  - True HTTP SSE chat endpoint with event flushing and conversation persistence.
  - Configured data-root propagation across workspace, provider, integration, audit, storage, backup, skill, and WASM handlers.
  - Integration coverage gates reached for agent, server, tools, sandbox, security, and memory packages.
  - Encrypted Vault-only LLM provider keys with automatic plaintext migration and deletion API.
  - Transactionally consistent SQLite `VACUUM INTO` backups including committed WAL content.
  - Removed the legacy approval “resume next cycle” fallback; checkpointed runs resume directly.
  - Linux GitHub Actions race-detector gate and `make test-race` target.
  - Fixed concurrent profile preference mutation and synchronized mock-provider calls discovered by the race detector.
- **Autonomous Mission Control & Task Backlog Matrix**:
  - Dedicated Sidebar page `Missions` (`web/src/pages/Missions/MissionsPage.tsx`) for visual task management, standing directives editor, and real-time pulse audit ledger.
  - SQLite table `autonomous_tasks` with priority queues (P0 Critical -> P3 Low), progress (0-100%), and automatic bi-directional sync with `data/workspace/TASKS.md` and `data/workspace/HEARTBEAT.md`.
  - Instant on-demand pulse trigger (`POST /api/heartbeat/trigger`) with live UI loading states.
  - REST endpoints `/api/tasks`, `/api/tasks/{id}`, `/api/heartbeat/config`, `/api/heartbeat/trigger`, `/api/heartbeat/runs`.
- **Working Memory Continuity (`chat_sessions` Resume)**:
  - Autonomous task execution automatically resolves and resumes dedicated sessions (`conv_task_<id>`), loading recent history to solve multi-step problems seamlessly across heartbeat pulses.
- **Outbound Notification Precision & Anti-Double-Dispatch**:
  - Anti-double-dispatch safeguards across Cron and Heartbeat to prevent duplicate confirmation messages.
  - Full untruncated content preservation for proactive channel notifications.
  - Automatic paragraph chunking for long articles (>3900 chars) on Telegram.
- **Token Ledger & Cost Analytics Modal**:
  - `internal/memory/tokens.go`: Added `GetHistory` method and REST endpoint `/api/system/token-usage/history`.
  - `web/src/components/modals/TokenLedgerModal.tsx`: Real-time transaction ledger with 14-day daily traffic trend and model distribution charts.
- **Multi-Account Channel Architecture & Dynamic Agent Binding**:
  - `internal/channels/manager.go`: Orchestrates multiple configured accounts per channel (e.g. Support Bot, DevOps Bot, Customer Hotline) with real-time lifecycle start/stop and targeted dispatching.
  - Inbound intelligent routing: inbound messages automatically resolve to the specific bound Agent (or wildcard `*`), executing dedicated sessions with cognitive working memory.
  - Outbound precision targeting: Cron jobs, Heartbeat daemon, and agents (`native_channel_notify`, `native_cron_schedule`) can target specific channel accounts (`target_account_id`) or broadcast (`all`).
  - REST endpoint `/api/integrations/channels/accounts` and dynamic multi-account synchronization in `api_integrations.go`.
  - Frontend Channel Account Modal with Agent Binding selector and Automations Task Modal with Target Account picker.
- **Token Traffic & Cost Analytics Subsystem**:
  - `internal/memory/tokens.go`: Built-in SQLite token usage tracker with catalog pricing per 1M tokens (GPT-4o, Claude 3.5 Sonnet, Gemini 1.5/2.0, DeepSeek V3/R1, Ollama free $0).
  - REST endpoints `/api/system/token-usage` and Prometheus exporter `/api/system/metrics/prometheus`.
  - Frontend Token Consumption & USD Cost Ledger card with real-time model distribution metrics.
- **Autonomous Heartbeat ReAct Engine**:
  - `internal/agent/heartbeat.go`: Autonomous 5-minute cognitive loop evaluating workspace `HEARTBEAT.md` / `TASKS.md` with Zero-Noise policy (`HEARTBEAT_OK`), SQLite persistence in `heartbeat_runs`, and proactive channel push alerts.
  - REST endpoint `/api/system/heartbeat/history`.
- **CRON Execution History Ledger**:
  - `internal/agent/cron_scheduler.go`: Full execution tracking with status, prompt, duration in ms, tokens consumed, and stdout output.
  - REST endpoints `/api/cron/history` and `/api/cron/{id}/history`.
  - Frontend Execution History tab in Automations page with status badges and log inspection.
- **Native Autonomous Tool Suite**:
  - `native_exec`: Sandboxed bash/PowerShell executor with 60s timeout and memory boundaries.
  - `native_file_list`, `native_file_delete`, `native_file_search`: File management and workspace grep searching tools.
  - `native_web_search`: Live web search via DuckDuckGo.
  - `native_channel_notify`: Proactive notification dispatcher to Telegram, WhatsApp, Discord, or Web.
- **Frontend Architecture & Reliability**:
  - React `ErrorBoundary` component styled with Soft Meadow design tokens.
  - Vite Rollup code splitting with `manualChunks` for vendor, tiptap editor, i18n, and icons.
  - Server-Sent Events (SSE) streaming endpoint `/api/agents/{agentID}/chat/stream`.
- **Production Hardening & Deployment**:
  - Security headers middleware (`nosniff`, `DENY`, `X-XSS-Protection`).
  - Safe WAL-checkpoint on shutdown and `VACUUM INTO` backup endpoint.
  - Docker `HEALTHCHECK` and systemd unit `deploy/systemd/actond.service`.

### Added
- **Integration Health Visibility for Chat Channels, Connectors, and MCP Servers**:
  - Previously, a broken Telegram/WhatsApp/Discord account, an expired/failed OAuth connector token, or an
    MCP server that failed to (re)connect or crashed mid-session only ever produced a `slog.Warn` server
    log line — nothing reached the web UI, so users had no way to discover *why* a channel, connector, or
    tool integration silently stopped working.
  - New bus events `channel.adapter_error` / `channel.adapter_recovered` and `mcp.server_error` /
    `mcp.server_recovered` (`internal/bus/messages.go`), published on every state transition (first
    failure after healthy, first success after failing) rather than on every retry, to avoid notification
    spam from a tight backoff/poll loop.
  - `internal/channels/manager.go`: new in-memory `AccountStatus{Connected, LastError, LastErrorAt}` map
    per account, updated both from synchronous adapter-start failures and asynchronously from each
    adapter's own poll/gateway loop (see below); exposed via `GetAccountStatuses()` and surfaced in
    `GET /api/integrations/channels/accounts` responses as an inline `status` field per account.
  - `internal/channels/telegram.go` / `discord.go`: the long-polling `fetchUpdates` loop and the Discord
    Gateway WebSocket dial loop previously swallowed connection/auth failures (e.g. a revoked bot token)
    into an endless `slog.Warn` cycle with zero operator visibility once the adapter had already "started"
    successfully. Both now call a `reportPollHealth`/`reportGatewayHealth` helper that publishes the new
    channel health events on state transitions.
  - `internal/tools/mcp_client.go`: `MCPClient` gained a `deliberate`/`onClose` mechanism — the stdio
    `readLoop()` now distinguishes an operator-initiated `Close()` from an unexpected process crash/stdout
    error and invokes `onClose` exactly once for the latter. `MCPHostEngine` gained `SetEventBus()`,
    `lastErrors` tracking, and now publishes `mcp.server_error`/`mcp.server_recovered` on `RestoreServers()`
    failures, `ConnectServer()` failures, and unexpected mid-session disconnects (previously invisible:
    `ListServers()` kept reporting `Connected: true` for a dead server until full process restart).
    `MCPServerStatus` gained `LastError`/`LastErrorAt` fields.
  - `internal/system/notifications.go`: `NotificationManager.StartBackgroundListener()` now subscribes to
    all 6 new/existing integration events (`EventTokenExpired`, `EventTokenFailed`, the 2 new channel
    events, the 2 new MCP events) and creates persisted, web-visible notifications linking to `/connectors`,
    `/channels`, or `/tools` respectively. A 15-minute per-integration cooldown (`shouldNotifyIntegration`)
    prevents a persistently broken integration from spamming a new notification every retry cycle; a
    recovery event both resets the cooldown and creates a "back online" success notification.
  - `cmd/actond/main.go`: wires `mcpHost.SetEventBus(eventBus)` after `NewMCPHostEngine` construction
    (channels' `ChannelManager` already had the event bus wired for its own bus-event lifecycle).

### Fixed
- **Heartbeat brought into alignment with [OpenClaw's Heartbeat contract](https://docs.openclaw.ai/vi/gateway/heartbeat)**,
  fixing several related bugs reported as spurious cron-approval prompts and duplicate/unrelated web notifications:
  - Idle heartbeats (no active task, no actionable `HEARTBEAT.md` content) no longer invoke the model at all,
    so an unattended pulse can no longer invent a cron schedule to ask approval for.
  - `HEARTBEAT_OK` is now classified per OpenClaw's exact contract (`classifyHeartbeatResponse`): only an
    ack when the token is at the start/end of the reply and the remainder is ≤ `ackMaxChars` (default 300,
    configurable via new `HeartbeatConfig.AckMaxChars`); anything else is delivered as a real alert exactly once.
  - Both mission and routine heartbeat cycles now hard-deny `native_channel_notify`/`channel_notify` and
    `native_cron_schedule` via `tools.WithDeniedTools`, enforced inside `ToolRegistry.Execute` — a real
    execution boundary, not just a prompt instruction.
  - New `tools.WithAllowedTools`/`AllowedTools`/`IsToolAllowedInContext` context-based allowlist primitive
    (intersects across nested calls) alongside the existing denylist.
  - A 15s trigger cooldown coalesces trigger storms (e.g. a task mutation and an approval decision firing
    `TriggerWakeup()` moments apart) into a single cycle; manual "Pulse Now" requests always bypass it.
  - New optional `activeHours` window (`ActiveHoursStart`/`End`/`Timezone`) restricts routine pulses to a
    daily time range, mirroring OpenClaw's `heartbeat.activeHours`.
  - `ApprovalManager.Request()` now reports `IsNew()`; `ToolRegistry` only republishes the
    `approval:required` bus event for a genuinely new approval, not a reused pending one — fixing a
    duplicate web notification for the same approval request.
  - `useWebNotifications()` desktop-notification dedup is now a module-level key shared across every hook
    instance, fixing duplicate desktop notifications when `NotificationBell` and `NotificationsPage` are
    both mounted.
  - Removed default "system"-seeded backlog tasks and the historical generic default `HEARTBEAT.md`
    directive, both of which used to masquerade as real actionable work on every pulse.
  - **New**: weaker/faster models occasionally ignored the directive-or-`HEARTBEAT_OK` contract entirely
    and free-associated a conversational greeting or capability menu instead (e.g. replying "Chào Bieber!
    Tôi đã sẵn sàng, bạn cần hỗ trợ gì?" instead of executing the standing directive). The routine cycle's
    system prompt now includes an explicit "Autonomous Headless Execution Mode" rule block (only injected
    for heartbeat calls) telling the model there is no human present to greet, and a `looksLikeIdleChatter`
    safety net now reclassifies any such off-topic, tool-free reply as nominal ("ok") instead of forwarding
    it as a user-facing alert.
  - **New**: a pending approval for any agent used to freeze the *entire* backlog from launching new
    pending tasks. Approval gating for new task launches is now scoped to the task's own assigned agent —
    an unrelated approval elsewhere no longer blocks unrelated work from proceeding.
  - **New**: a mission task that never advances (model repeats the same `[PROGRESS: X%]` every cycle) used
    to retry silently forever. `trackTaskStall()` now escalates a one-time `[STALL WARNING]` into the task's
    execution log and run summary after 3 consecutive no-progress cycles, so it becomes operator-visible
    instead of quietly burning a model turn every cycle indefinitely.
  - **New**: since the model can never call the notify tool itself, a task whose directive clearly asks to
    "gửi thông báo" / "send to" a channel but whose own `TargetChannel` field is `"none"` would silently
    discard its completed result with no visible trace. `mentionsNotificationIntent()` now logs a one-time
    diagnostic warning for this misconfiguration (the task still executes normally — this is a warning, not
    an auto-corrected delivery target, since the target channel remains an explicit structured field).

### Infrastructure
- `.editorconfig` for consistent coding style
- `.gitignore` for Go + Node.js + IDE artifacts
- Apache 2.0 license

<!-- Release links -->
[Unreleased]: https://github.com/actonos/actonos/compare/v1.0.4...HEAD
[1.0.4]: https://github.com/actonos/actonos/releases/tag/v1.0.4
[1.0.3]: https://github.com/actonos/actonos/releases/tag/v1.0.3
[1.0.3]: https://github.com/actonos/actonos/releases/tag/v1.0.3
[1.0.2]: https://github.com/actonos/actonos/releases/tag/v1.0.2
[1.0.1]: https://github.com/actonos/actonos/releases/tag/v1.0.1
[1.0.0]: https://github.com/actonos/actonos/releases/tag/v1.0.0

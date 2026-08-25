# Changelog

All notable changes to ActonOS will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security
- Plugin egress now reuses SSRF validation (no loopback/private/metadata, no `*`/`*.com`, redirect re-check). Remote plugin installs fail closed without a valid Ed25519 signature. Pairing codes are 8-character `/pair`-only with lockout. Admin passwords use Argon2id (min 8) with login lockout; query-string tokens are ignored. Webhook recipient URLs are validated. Plugin logs redact vault secrets. Chat markdown strips `javascript:` links.

### Changed
- Channels UI is the pairing/accounts surface for installed WASM chat plugins (not restored native adapters). Coverage floors are enforced by `scripts/cover-gate.go` from `make test-unit`. Integration Makefile target runs `./internal/...` with the `integration` tag.

### Added
- **Unified WasmLoader Plugin Architecture & Sandboxed Extensions**:
  - Polyglot plugin execution engine (`internal/plugin/`) powered by Wazero (100% pure Go, zero CGO, `CGO_ENABLED=0` static binary compliant).
  - Unified plugin model consolidating **Tools**, **Chat Channels**, and **SaaS Connectors** into portable `.actonpkg` package bundles with `manifest.json`, `plugin.wasm`, and Ed25519 `signature.sig` (required and verified for remote registry installs; verified when present on operator upload).
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
[Unreleased]: https://github.com/actonos/actonos/commits/main

# Changelog

All notable changes to ActonOS will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

### Infrastructure
- `.editorconfig` for consistent coding style
- `.gitignore` for Go + Node.js + IDE artifacts
- Apache 2.0 license

<!-- Release links -->
[Unreleased]: https://github.com/actonos/actonos/commits/main

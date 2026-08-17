# Changelog

All notable changes to ActonOS will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

# ActonOS REST API Reference

> REST API reference for the ActonOS daemon (`actond`).
> Base URL: `http://localhost:8080/api`

---

## Table of Contents

- [Authentication](#authentication)
- [Health & System](#health--system)
- [Setup & Onboarding](#setup--onboarding)
- [Dashboard](#dashboard)
- [Agent Management & Studio](#agent-management--studio)
- [Cron Automations](#cron-automations)
- [Chat & Conversations](#chat--conversations)
- [Channel Accounts & Device Pairing](#channel-accounts--device-pairing)
- [Tools & Tool Hub](#tools--tool-hub)
- [Plugins (WasmLoader)](#plugins-wasmloader)
- [Vault Secrets](#vault-secrets)
- [Workspace File Manager](#workspace-file-manager)
- [Error Format](#error-format)

---

## Authentication

All API endpoints (except `/api/health`, `/api/models`, `/api/notifications/push/vapid-key`, `/api/auth/status`, `/api/auth/setup`, `/api/auth/login`, `/api/auth/logout`) require authentication when the system is initialized.

```http
Authorization: Bearer <session_token>
```

### `GET /api/auth/status`
Returns initialization status and current authentication state.

**Response:**
```json
{
  "data": {
    "initialized": true,
    "authenticated": false,
    "user_name": "Admin"
  }
}
```

### `POST /api/auth/setup`
Initialize system admin identity and set the master password during onboarding.

**Request:**
```json
{
  "user_name": "Senior Operator",
  "password": "mySecurePassword123"
}
```

### `POST /api/auth/login`
Authenticate using the master password.

**Request:**
```json
{
  "password": "mySecurePassword123"
}
```

**Response:**
```json
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user_name": "Senior Operator",
    "expires_at": "2026-08-18T12:00:00Z"
  }
}
```

### `POST /api/auth/logout`
Revoke current session token.

### `PUT /api/auth/password` (Protected)
Change admin password.

---

## Health & System

### `GET /api/health`
System health check. No authentication required.

**Response:**
```json
{
  "data": {
    "status": "healthy",
    "version": "0.1.0",
    "uptime_seconds": 86400,
    "runtime_mode": "docker",
    "agents_active": 2,
    "tailscale_connected": true,
    "tailscale_ip": "100.64.1.42"
  }
}
```

### `GET /api/models`
Retrieve the canonical catalog of all supported LLM models, provider specs, badges, and pricing tiers. Single source of truth for the entire system.

**Response:**
```json
{
  "data": {
    "models": [
      {
        "id": "anthropic/claude-sonnet-4-6",
        "name": "Claude Sonnet 4.6",
        "provider_id": "anthropic",
        "provider_name": "Anthropic Claude",
        "badge": "Frontier Coding & Multi-Agent Swarm",
        "context_window": "512k",
        "category": "Cloud Frontier",
        "prompt_per_1m": 3.0,
        "completion_per_1m": 15.0,
        "is_default": true,
        "supports_tools": true,
        "supports_vision": true
      }
    ],
    "providers": [
      {
        "id": "anthropic",
        "name": "Anthropic Claude",
        "category": "Cloud Frontier",
        "description": "Frontier coding, hybrid reasoning, and safety-hardened models.",
        "default_base_url": "https://api.anthropic.com/v1",
        "accent_color": "#D97706",
        "model_presets": [...]
      }
    ]
  }
}
```

### `GET /api/system/metrics`
Hardware and container metrics (CPU usage, RAM, disk, uptime).

### `GET /api/system/embedding`
Return the durable semantic-index queue and local embedding helper status.

```json
{
  "data": {
    "pending": 3,
    "running": 0,
    "dead": 0,
    "indexed_sources": 42,
    "active_chunks": 87,
    "oldest_due_at": "2026-08-21T12:01:00Z",
    "model_id": "intfloat/multilingual-e5-small",
    "model_revision": "614241f622f53c4eeff9890bdc4f31cfecc418b3",
    "dimension": 384,
    "service_ready": true
  }
}
```

Chat messages and workspace mutations are debounced for one minute in SQLite
before ONNX inference and Chromem indexing. When `embeddingd` is unavailable,
jobs remain durable and retrieval continues through SQLite FTS5.

### `GET /api/system/identity` | `PUT /api/system/identity`
Retrieve and update user identity & preferences.

### `GET /api/system/keys` | `POST /api/system/keys`
Retrieve masked API keys or store encrypted provider keys (Anthropic, OpenAI, Google, OpenRouter).

Provider keys are encrypted in Vault and never persisted in
`llm_providers.json`. Legacy plaintext key files are migrated automatically.

### `DELETE /api/system/keys/{provider}`

Remove a provider key from Vault and clear its configured status.

### `POST /api/system/keys/test`
Test connectivity for a specific LLM provider API key.

### `GET /api/system/audit`
Retrieve system audit log entries.

### `GET /api/system/token-usage`
Get aggregated token consumption metrics, USD cost estimations, model breakdown, agent breakdown, and 14-day daily trends.

### `GET /api/system/token-usage/history`
Query recent token transaction audit ledger records with optional `agent_id` and `source` filtering.

### `GET /api/system/heartbeat/history`
Get execution logs for autonomous 5-minute heartbeat cycles (Zero-Noise evaluation).

### `GET /api/system/metrics/prometheus`
Export Prometheus-compatible telemetry metrics (`actonos_uptime_seconds`, `actonos_goroutines`, `actonos_memory_alloc_bytes`, `actonos_agents_active`, `actonos_tokens_total`, `actonos_cost_usd_total`).

### `GET /api/system/backup`
Download full system state snapshot (SQLite DB, manifests, profiles, SOUL.md).

### `POST /api/system/ota/check`
Check for software updates.

### `GET /api/system/tailscale`
Tailscale node status and peers.

### `GET /api/system/wifi/scan` | `POST /api/system/wifi/connect`
Scan Wi-Fi access points and connect (Bare-metal mode).

### `POST /api/system/restart`
Restart `actond` daemon service.

---

## Setup & Onboarding

### `GET /api/setup/status`
Check onboarding status.

### `POST /api/setup/wizard`
Submit complete onboarding wizard setup (Wi-Fi, Keys, Identity, Password).

---

## Dashboard

### `GET /api/dashboard/summary`
Get high-level statistics for agents, active cron tasks, memory size, and channel connections.

---

## Agent Management & Studio

### `GET /api/agents`
List all configured agents.

### `POST /api/agents`
Create a new agent manifest.

**Request:**
```json
{
  "name": "Research Assistant",
  "description": "Helps with academic research and web summaries",
  "avatar_icon": "bot",
  "model_config": {
    "primary_model": "openai/gpt-5.4-mini",
    "fallback_model": "openai/gpt-5.4-mini",
    "temperature": 0.3
  },
  "system_instructions": "You are an expert research analyst...",
  "authorized_tools": ["mcp_fetch", "browser_view"],
  "listen_channels": ["*"],
  "heartbeat_config": {
    "enabled": true,
    "directives": "# Research Monitor\n- Review new arxiv preprints\n- Alert Telegram if relevant",
    "interval_minutes": 60,
    "target_channel": "telegram",
    "target_account_id": "tg_support_bot",
    "active_hours_start": "08:00",
    "active_hours_end": "22:00",
    "active_hours_timezone": "Asia/Ho_Chi_Minh"
  },
  "delegation_scope": {
    "max_monthly_budget_usd": 50.0,
    "allowed_workspace_paths": ["/data/workspace/research/"],
    "require_human_approval_level": "Medium"
  }
}
```

### `GET /api/agents/{agentID}`
Retrieve single agent manifest.

### `POST /api/agents/{agentID}/chat/stream`
Send a user prompt over a true Server-Sent Events stream. The server flushes
live `thought`, `token`, `tool_call`, `tool_result`, `audit`, `done`, and
`error` events while preserving the conversation and assistant result.

Request:

```json
{
  "conversation_id": "conv_optional",
  "message": "Inspect and fix the project"
}
```

Response content type: `text/event-stream`.

### `PUT /api/agents/{agentID}`
Update agent manifest.

### `DELETE /api/agents/{agentID}`
Delete agent.

### `POST /api/agents/{agentID}/start` | `POST /api/agents/{agentID}/stop`
Start or stop an agent.

### `GET /api/agents/{agentID}/soul` | `PUT /api/agents/{agentID}/soul`
Get or save raw `SOUL.md` system personality prompt for an agent.

### `GET /api/agents/{agentID}/memory-md` | `DELETE /api/agents/{agentID}/memory-md`
Inspect or clear long-term episodic reflections from `MEMORY.md`.

---

## Autonomous Missions & Task Backlog

### `GET /api/tasks`
List autonomous tasks with optional `status` (`pending`, `in_progress`, `completed`, `blocked`) and `priority` filters.

### `POST /api/tasks`
Create a new autonomous mission / task. Automatically synchronizes with `data/workspace/TASKS.md`.

**Request:**
```json
{
  "title": "Inspect disk storage & database WAL",
  "description": "Examine local disk usage and archive expired temporary log files.",
  "priority": "p1_high",
  "assigned_agent_id": "auto",
  "target_channel": "all",
  "target_account_id": "all"
}
```

### `GET /api/tasks/{id}`
Retrieve a specific task details and execution history.

### `PUT /api/tasks/{id}`
Update task status, progress (0-100%), priority, or execution log.

### `DELETE /api/tasks/{id}`
Delete a task from the database and workspace `TASKS.md`.

---

## Heartbeat Coordinator & Autonomous Pulse

Behavior follows [OpenClaw's Heartbeat contract](https://docs.openclaw.ai/vi/gateway/heartbeat): a periodic
agent turn that reads standing directives strictly, never invents work, and stays silent unless something is
actually worth surfacing. See `docs/ARCHITECTURE.md` §3.C for the full trigger/gating and response-contract
diagrams.

> **Note**: ActonOS supports both **System Core Heartbeat** (managed below for `agent_system_core` and global mission task backlog) and **Per-Agent Autonomous Heartbeats** (configured individually per custom agent via `AgentManifest.heartbeat_config` under `POST /api/agents` and `PUT /api/agents/{agentID}`).

### `GET /api/heartbeat/config`
Get current system standing directives (`HEARTBEAT.md`), pulse interval, target channel, and response-contract
settings. Response body (`HeartbeatConfig`):

| Field | Type | Default | Description |
|:---|:---|:---|:---|
| `enabled` | bool | `true` | Whether the daemon runs at all. |
| `interval_minutes` | int | `5` | Ticker cadence between routine pulses. |
| `directives` | string | `""` | Raw `HEARTBEAT.md` content (legacy default lines are stripped automatically). |
| `target_channel` | string | `"all"` | Delivery channel for alerts; `"none"` runs the cycle but sends nothing. |
| `target_account_id` | string | `"all"` | Multi-account channel id override. |
| `auto_delegate` | bool | `true` | Reserved for future delegation policy. |
| `zero_noise` | bool | `true` | Reserved; ack/alert classification always applies regardless. |
| `ack_max_chars` | int | `300` | Max characters allowed alongside `HEARTBEAT_OK` before a reply counts as a real alert. |
| `active_hours_start` | string | `""` | Daily `HH:MM` window start (routine pulses only); empty = 24/7. |
| `active_hours_end` | string | `""` | Daily `HH:MM` window end; `start == end` always skips. |
| `active_hours_timezone` | string | `""` | IANA timezone for the window; empty = server local time. |

### `PUT /api/heartbeat/config`
Save standing directives and pulse configuration (same `HeartbeatConfig` body as above). Also re-syncs the
running daemon (`HeartbeatDaemon.SyncConfig`) immediately.

### `POST /api/heartbeat/trigger`
Trigger an immediate on-demand cognitive heartbeat pulse. Manual pulses always bypass the trigger cooldown
and `active_hours` window (mirrors OpenClaw's `system event --mode now`).

### `GET /api/heartbeat/runs`
List recent heartbeat cognitive pulses and audit evaluations.

---

## Cron Automations

### `GET /api/agents/cron` (or `/api/cron`)
List all scheduled cron automations.

### `POST /api/agents/cron`
Create or update scheduled cron task.

**Request:**
```json
{
  "id": "cron_morning_brief",
  "agent_id": "agent_system_core",
  "name": "Daily Morning News Brief",
  "cron_expression": "0 8 * * *",
  "prompt": "Summarize top tech news and write to /workspace/daily_brief.md",
  "enabled": true
}
```

### `GET /api/cron/history`
List past execution history across all scheduled cron jobs and autonomous triggers.

### `GET /api/cron/{id}/history`
List past execution history for a specific scheduled task.

### `POST /api/agents/cron/{id}/run`
Manually trigger execution of a cron job immediately.

### `DELETE /api/agents/cron/{id}`
Delete a cron job.

---

## Chat & Conversations

### `GET /api/conversations`
List all conversation threads with metadata and message counts.

### `POST /api/conversations`
Create a new conversation thread.

### `GET /api/conversations/{id}`
Get full conversation history and message items.

### `DELETE /api/conversations/{id}`
Delete conversation thread.

### `POST /api/agents/{agentID}/chat`
Send message to agent. Supports Server-Sent Events (SSE) streaming (`thought`, `token`, `tool_call`, `tool_result`, `audit`, `done`).

---

## Channel Accounts & Device Pairing

### `GET /api/integrations/channels` | `POST /api/integrations/channels`
Get and configure multi-account credentials for Telegram, WhatsApp, and Discord with agent bindings and message routing modes.

**Account Payload Schema (`ChannelAccount`):**
```json
{
  "id": "tg_bot_1",
  "name": "Support Bot",
  "label": "Support Bot",
  "channel": "telegram",
  "token": "123456:ABC-DEF...",
  "phone_id": "",
  "bound_agent_ids": ["support_agent", "triage_agent"],
  "routing_mode": "mention",
  "enabled": true
}
```

- `routing_mode`: `'exclusive'` (only assigned agents), `'mention'` (route by `@agent_name` in group chats), or `'fallback'` (default to `agent_system_core`).

### `GET /api/integrations/channels/accounts`
List all configured channel accounts across all channels with their assigned agent bindings, routing modes, and live health status.

### `POST /api/integrations/pairing/code`
Generate pairing code.

### `POST /api/integrations/pairing/verify`
Verify 6-digit pairing code from external chat channel.

### `GET /api/integrations/authorizations` | `DELETE /api/integrations/authorizations`
List and revoke authorized external chat senders.

---

## Tools & Tool Hub

### `GET /api/tools`
List all registered native tools, MCP servers, and WASM plugins.

### `POST /api/tools/mcp` | `DELETE /api/tools/mcp/{serverID}`
Register or disconnect an MCP server. Connecting is a High-risk operation:

1. Call `POST /api/tools/mcp` with the MCP configuration.
2. The server returns `202` with a durable approval request.
3. Approve through `POST /api/approvals/{id}/approve`; the approved MCP process is started automatically.

MCP stdio processes require Docker, Bubblewrap, or the explicit development-only
`ACTONOS_ALLOW_UNSANDBOXED_MCP=1` override.

### `POST /api/tools/execute`
Execute a tool through the same authorization, risk, approval, audit, and sandbox
boundary used by agents. Medium/High actions may return:

```json
{
  "data": {
    "status": "approval_required",
    "approval": {
      "id": "apr_...",
      "trace_id": "...",
      "tool_name": "native_file_write",
      "risk_level": "High",
      "status": "pending"
    }
  }
}
```

An approved exact action can also be submitted with `approval_id`. Approval hashes
bind the decision to the agent, tool name, and normalized arguments.

### `GET /api/tools/hub/catalog`
Browse online Tool Hub & Skill marketplace catalog.

### `POST /api/tools/hub/install` | `POST /api/tools/hub/uninstall`
Install or remove a skill from the Tool Hub catalog.

---

## Plugins (WasmLoader)

### `GET /api/plugins`
List all installed WASM plugins with their manifest, capabilities (`tool`, `channel`, `connector`), permissions, and running status.

**Response `200 OK`**:
```json
{
  "plugins": [
    {
      "id": "com.actonos.plugin.telegram",
      "name": "Telegram Bot Gateway",
      "version": "1.0.0",
      "author": "ActonOS Team",
      "description": "Telegram chat adapter and tool execution plugin",
      "capabilities": ["channel", "tool"],
      "permissions": {
        "net_outbound": ["api.telegram.org"],
        "storage": true,
        "secrets": ["telegram_bot_token"],
        "bus_events": ["channel:message:inbound"]
      },
      "enabled": true,
      "status": "running"
    }
  ],
  "count": 1
}
```

### `GET /api/plugins/available`
Fetch the available plugin catalog from the remote ActonOS release registry (`https://github.com/actonos/plugin-sdk/releases/latest/download/plugin-registry.json`). Returns official pre-built WASM plugins with author info, ratings, capabilities, permissions, and local installation status.

**Response `200 OK`**:
```json
{
  "catalog": [
    {
      "id": "discord",
      "name": "Discord Channel Adapter",
      "version": "1.0.0",
      "author": "ActonOS Core Team",
      "description": "Bi-directional Discord gateway supporting guild channels, DMs, and slash interactions.",
      "category": "channel",
      "tags": ["discord", "chat", "bot"],
      "stars": 324,
      "capabilities": ["channel"],
      "permissions": {
        "net_outbound": ["discord.com", "gateway.discord.gg"],
        "secrets": ["DISCORD_BOT_TOKEN"],
        "storage": true
      },
      "installed": false
    }
  ],
  "count": 1
}
```

### `POST /api/plugins/install`
Download, unpack, verify, and hot-reload an official WASM plugin package (.actonpkg) from the release registry into the Wazero runtime. Emits real-time `plugin.progress` and `plugin.installed` events over the event bus.

**Request Body**:
```json
{
  "plugin_id": "discord",
  "download_url": "https://github.com/actonos/plugin-sdk/releases/latest/download/discord.actonpkg"
}
```

**Response `200 OK`**:
```json
{
  "status": "installed",
  "plugin": {
    "manifest": {
      "id": "discord",
      "name": "Discord Channel Adapter",
      "version": "1.0.0",
      "capabilities": ["channel"]
    },
    "enabled": true,
    "status": "running"
  }
}
```

### `POST /api/plugins/upload`
Upload an `.actonpkg` package bundle (or compiled `.wasm` binary) via multipart form data (`file`). The server unpacks `manifest.json`, `plugin.wasm`, and `signature.sig` when present. A present signature is always verified as Ed25519 over SHA-256(`manifest.json` || `plugin.wasm`). Unsigned SDK packages (no `signature.sig`) install unless `ACTONOS_REQUIRE_SIGNED_PLUGINS=1`. If administrative approval is configured, returns `202 Accepted` with a pending approval request.

### `POST /api/plugins/{id}/enable` | `POST /api/plugins/{id}/disable`
Enable or disable a specific WASM plugin at runtime without restarting the daemon.

### `DELETE /api/plugins/{id}`
Uninstall a plugin and remove its binary and configuration from `/data/plugins/{id}`.

### `GET /api/plugins/{id}/logs`
Retrieve execution logs and telemetry emitted by the plugin sandbox.

### `POST /api/plugins/{id}/config` | `PUT /api/plugins/{id}/config`
Update plugin configuration values and persist declared vault secrets. Immediately triggers a hot-reload of the plugin instance in the Wazero sandbox runtime.

**Request Body**:
```json
{
  "config": {
    "poll_interval_seconds": 5,
    "accounts": [
      {
        "account_id": "bot_support",
        "default_agent": "agent_customer_care",
        "enable_embeds": true
      }
    ]
  },
  "secrets": {
    "discord_bot_token": "your_bot_token_here"
  }
}
```

---

## Vault Secrets

The vault encrypts credentials at rest using AES-256-GCM and Argon2id key derivation. DMI/CPU hardware binding is not applied. Secrets are brokered to authorized sandboxed WASM plugins and agents.

### `GET /api/vault/secrets`
List metadata for all encrypted secrets stored in the vault.

**Response `200 OK`**:
```json
{
  "secrets": [
    {
      "name": "discord_bot_token",
      "updated_at": "2026-08-24T00:50:00Z",
      "is_provider": false
    }
  ],
  "count": 1
}
```

### `GET /api/vault/secrets/{name}`
Retrieve masked credential metadata for a specific secret key.

**Response `200 OK`**:
```json
{
  "name": "discord_bot_token",
  "configured": true,
  "masked": "MTI••••••••NDU=",
  "length": 32,
  "is_provider": false
}
```

### `POST /api/vault/secrets` | `PUT /api/vault/secrets/{name}`
Encrypt and store a named secret credential in the vault database.

**Request Body**:
```json
{
  "name": "discord_bot_token",
  "value": "your_bot_token_here"
}
```

### `DELETE /api/vault/secrets/{name}`
Permanently remove a named secret from the encrypted vault.

---


## Human Approval & Durable Runs

### `GET /api/approvals?status=pending`
List durable approval requests. Supported filters include `pending`, `approved`,
`rejected`, `expired`, and `all`.

### `POST /api/approvals/{id}/approve`
Approve and execute the exact action recorded by the request. Optional body:

```json
{
  "reason": "Reviewed by the system administrator",
  "dont_ask_again": "task"
}
```

`dont_ask_again` is optional:

- `task` — skip later approvals for this agent and tool while the same
  mission (`task_id`) is running. Rejected if the approval is not tied to a mission.
- `today` — skip later approvals for this agent and tool until the end of the
  current UTC day (at least one hour).

Administrative actions (`admin_*`, MCP connect) cannot receive a waiver. The
current action is still executed immediately after approval.

### `POST /api/approvals/{id}/reject`
Reject a pending action without executing it.

### `GET /api/runs?limit=100`
List durable agent executions with trace ID, source, status, aggregate token usage,
iteration count, and termination reason.

### `GET /api/runs/{id}/events`
Return the ordered LLM/tool/approval event sequence for a run.

Common termination reasons include `goal_completed`, `approval_required`,
`verification_failed`, `no_progress`, `iteration_budget_exhausted`,
and `infrastructure_failure`. Approving a paused autonomous run resumes the same
run from its persisted checkpoint.

### Administrative mutations

Workspace write/upload/delete/mkdir, skill creation, WASM upload, Tool Hub
install/uninstall, and system restart use the same exact-action approval ledger.
The initial request returns `202 Accepted`; approval dispatches only the
normalized action recorded in that request.

### `GET /api/system/audit/verify`

Verify the audit log SHA-256 chain and report corruption caused by deletion,
reordering, or modification.

### `GET /api/system/backup`

Download a transactionally consistent SQLite snapshot created with
`VACUUM INTO`. The snapshot includes committed WAL content and is a standalone
database file.

---

## Workspace File Manager

### `GET /api/workspace/files`
List directory contents in `/workspace`.

### `GET /api/workspace/file`
Read text/code file from workspace.

### `POST /api/workspace/file`
Write/update workspace file.

### `DELETE /api/workspace/file`
Delete workspace file.

### `POST /api/workspace/mkdir`
Create directory in workspace.

### `POST /api/workspace/upload`
Upload file to workspace.

---

## Realtime Operations

### `GET /api/realtime`

Upgrades to a protected, same-origin WebSocket. Authentication uses the
HttpOnly `actonos_token` session cookie. Every two seconds the server emits a
`snapshot` containing:

- `metrics`: CPU, RAM, chip temperature, disk, runtime mode, Docker containers,
  and optional `canvas_url`.
- `runs`: recent durable agent runs.
- `approvals`: pending exact-action approvals.
- `tokens`: current daily/monthly/model token and cost summary.

The socket is observation-only. Mutations continue through their normal REST
approval and authorization boundaries.

## Notification Center

### `GET /api/notifications`
Returns paginated list of system, mission, error, and approval notifications. Query params: `page` (default 1), `limit` (default 20), `type` (optional filter: approval, error, warning, info, success), `unread_only` (bool).

**Response:**
```json
{
  "data": {
    "notifications": [
      {
        "id": "notif_1700000000_abc123",
        "title": "Approval Required: native_exec",
        "message": "Agent 'agent_system_core' requested execution of high-risk tool 'native_exec'.",
        "type": "approval",
        "category": "approval",
        "link": "/missions",
        "is_read": false,
        "created_at": "2026-08-18T14:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 20,
    "unread_count": 1
  }
}
```

### `GET /api/notifications/unread-count`
Returns `{ "data": { "unread_count": 1 } }`.

### `POST /api/notifications/mark-read`
Marks notifications as read. Request body: `{ "id": "notif_..." }` or `{ "all": true }`.

### `DELETE /api/notifications`
Deletes a notification (`?id=notif_...`) or clears entire history (`?all=true`).

### `GET /api/notifications/push/vapid-key`
Returns the server's VAPID public key for browser PushManager subscription.

**Response:**
```json
{
  "data": {
    "public_key": "BN3..."
  }
}
```

### `POST /api/notifications/push/subscribe`
Registers a browser Service Worker push subscription for background alerts.

**Request:**
```json
{
  "endpoint": "https://updates.push.services.mozilla.com/wpush/v2/...",
  "keys": {
    "p256dh": "...",
    "auth": "..."
  },
  "user_agent": "Mozilla/5.0 ..."
}
```

### `POST /api/notifications/push/unsubscribe`
Unsubscribes a push notification endpoint.

**Request:**
```json
{
  "endpoint": "https://updates.push.services.mozilla.com/wpush/v2/..."
}
```

### `POST /api/notifications/push/test`
Dispatches a test push notification to active service worker subscriptions.

---

## Tools & Skills Hub

### `GET /api/tools`
Lists all registered tools (Native, MCP, WASM, Skills) along with their execution parameters and requirements. Query params: `category` (optional: `native`, `mcp`, `wasm`, `skill`).

### `PUT /api/tools/skills/{name}/toggle`
Enables or disables an installed skill tool.

**Request:**
```json
{
  "enabled": false
}
```

### `GET /api/tools/hub/catalog`
Fetches the live Community Skills registry with metadata, stars, categories, and tags.

### `POST /api/tools/hub/install`
Installs a community skill package (downloads all declared files into `/data/skills/<slug>/`).

**Request:**
```json
{
  "skill_id": "tavily"
}
```

### `POST /api/tools/hub/uninstall`
Removes an installed community skill directory.

**Request:**
```json
{
  "skill_id": "tavily"
}
```

---

## Workspace File Manager & Semantic Memory

Workspace schema v2 is metadata-only and is intended for fresh databases. Legacy SQLite BLOB workspace schemas are rejected at startup; no automatic content migration is performed.

### `GET /api/workspace/files`
Lists files and directories in a workspace directory. Query params: `dir` (relative directory path, e.g. `docs` or empty for root). Returns file list with AI indexing status (`ai_indexed`, `ai_state`).

### `GET /api/workspace/file`
Returns metadata for a specific file. Query params: `id` (opaque file ID) or the legacy `path` value. Text, JSON, and CSV responses include UTF-8 `content`; binary and media responses include `raw_url` without embedding a base64 payload.

### `GET /api/workspace/raw`
Streams the original file bytes from `/data/workspace/{FOLDER_UUID}/{FILE_UUID}`. SQLite stores only the relative path and file metadata. Query params: `id` (opaque file ID) or the legacy `path` value. The endpoint supports HTTP byte ranges, returns `Content-Disposition: inline`, and is the canonical source for PDF, image, audio, and video previews and downloads.

### `POST /api/workspace/file`
Creates or updates a file. Enqueues semantic embedding automatically.

**Request:**
```json
{
  "path": "scripts/analysis.py",
  "content": "print('hello world')"
}
```

### `DELETE /api/workspace/file`
Deletes a file or directory. Query params: `path`.

### `POST /api/workspace/rename`
Renames or moves a file/folder within the workspace.

**Request:**
```json
{
  "old_path": "scripts/old.py",
  "new_path": "scripts/new.py"
}
```

### `POST /api/workspace/duplicate`
Duplicates a file with automatic `_copy` suffix naming.

**Request:**
```json
{
  "path": "data/sample.json"
}
```

### `GET /api/workspace/stats`
Returns total workspace storage usage, file/directory counts, semantic memory indexed count, and storage breakdown by category (documents, code, data, media, other).

### `GET /api/workspace/zip`
Downloads a folder or multiple files as a compressed `.zip` archive. Query params: `path` (folder) or `paths` (comma-separated list of relative files/folders).

### `POST /api/workspace/reindex`
Manually triggers embedding re-indexing for a file.

**Request:**
```json
{
  "path": "docs/manual.pdf"
}
```

### `GET /api/workspace/chunks`
Fetches the semantic text chunks and embedding metadata generated for a workspace file. Query params: `path`.

---

## Error Format

All error responses adhere to the standard envelope:

```json
{
  "error": {
    "code": "AGENT_NOT_FOUND",
    "message": "Agent with ID 'xyz' does not exist"
  }
}
```

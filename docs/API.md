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
- [Integrations, Channels & Pairing](#integrations-channels--pairing)
- [Tools & Tool Hub](#tools--tool-hub)
- [Workspace File Manager](#workspace-file-manager)
- [Error Format](#error-format)

---

## Authentication

All API endpoints (except `/api/health`, `/api/auth/status`, `/api/auth/setup`, `/api/auth/login`, `/api/auth/callback`, `/api/webhooks/*`) require authentication when the system is initialized.

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
    "primary_model": "anthropic/claude-sonnet-4-6",
    "fallback_model": "openai/gpt-5-mini",
    "temperature": 0.3
  },
  "system_instructions": "You are an expert research analyst...",
  "authorized_tools": ["mcp_fetch", "browser_view"],
  "listen_channels": ["*"],
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

### `GET /api/agents/{agentID}/memory-md`
Inspect long-term episodic reflections from `MEMORY.md`.

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
agent turn that reads `HEARTBEAT.md` strictly, never invents work, and stays silent unless something is
actually worth surfacing. See `docs/ARCHITECTURE.md` §4.C for the full trigger/gating and response-contract
diagrams.

### `GET /api/heartbeat/config`
Get current standing directives (`HEARTBEAT.md`), pulse interval, target channel, and response-contract
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

## Integrations, Channels & Pairing

### `GET /api/integrations`
List SaaS integrations (Google, Notion, GitHub) and connection status.

### `POST /api/integrations/{provider}/auth-url`
Generate OAuth 2.1 PKCE authorization URL.

### `POST /api/integrations/{provider}/token`
Save direct API token for SaaS connector.

### `GET /api/integrations/channels` | `POST /api/integrations/channels`
Get and configure multi-account credentials for Telegram, WhatsApp, and Discord with agent bindings.

### `GET /api/integrations/channels/accounts`
List all configured channel accounts across all channels with their assigned agent bindings.

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

## Human Approval & Durable Runs

### `GET /api/approvals?status=pending`
List durable approval requests. Supported filters include `pending`, `approved`,
`rejected`, `expired`, and `all`.

### `POST /api/approvals/{id}/approve`
Approve and execute the exact action recorded by the request. Optional body:

```json
{ "reason": "Reviewed by the system administrator" }
```

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

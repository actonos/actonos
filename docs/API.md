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

### `GET /api/system/metrics`
Hardware and container metrics (CPU usage, RAM, disk, uptime).

### `GET /api/system/identity` | `PUT /api/system/identity`
Retrieve and update user identity & preferences.

### `GET /api/system/keys` | `POST /api/system/keys`
Retrieve masked API keys or store encrypted provider keys (Anthropic, OpenAI, Google, OpenRouter).

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
    "primary_model": "anthropic/claude-3-7-sonnet",
    "fallback_model": "google/gemini-2.5-flash",
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
Send user prompt and stream real-time Server-Sent Events (SSE) token stream with tool invocation badges.

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
Register or disconnect an MCP server.

### `POST /api/tools/execute`
Directly execute a tool.

### `GET /api/tools/hub/catalog`
Browse online Tool Hub & Skill marketplace catalog.

### `POST /api/tools/hub/install` | `POST /api/tools/hub/uninstall`
Install or remove a skill from the Tool Hub catalog.

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

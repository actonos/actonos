# ActonOS REST API Reference

> API endpoints for the ActonOS daemon (`actond`).
> Base URL: `http://localhost:8080/api`

---

## Table of Contents

- [Authentication](#authentication)
- [Health & System](#health--system)
- [Setup & Onboarding](#setup--onboarding)
- [Agent Management](#agent-management)
- [Chat & Conversations](#chat--conversations)
- [Integrations & OAuth](#integrations--oauth)
- [Tools & Plugins](#tools--plugins)
- [Workspace & Files](#workspace--files)

---

## Authentication

All API endpoints (except `/api/health` and `/api/setup/*`) require authentication via the admin PIN set during onboarding.

```
Authorization: Bearer <session_token>
```

### `POST /api/auth/login`

Authenticate with admin PIN.

**Request:**
```json
{
  "pin": "123456"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2026-08-17T12:00:00Z"
}
```

---

## Health & System

### `GET /api/health`

Returns system health status. No authentication required.

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "git_commit": "a1b2c3d",
  "uptime_seconds": 86400,
  "runtime_mode": "baremetal",
  "agents_active": 3,
  "memory_usage_mb": 28.5,
  "cpu_percent": 2.1,
  "cpu_temp_celsius": 45.2,
  "disk_usage_percent": 15.3,
  "tailscale_connected": true,
  "tailscale_ip": "100.64.1.42"
}
```

### `GET /api/system/metrics`

Returns detailed hardware metrics.

**Response:**
```json
{
  "cpu": {
    "model": "Intel N100",
    "cores": 4,
    "usage_percent": 2.1,
    "temperature_celsius": 45.2
  },
  "memory": {
    "total_mb": 8192,
    "used_mb": 1024,
    "actond_mb": 28.5
  },
  "disk": {
    "total_gb": 64,
    "used_gb": 9.8,
    "data_dir_gb": 5.2
  }
}
```

### `GET /api/system/tailscale`

Returns Tailscale connection status and peer information.

### `POST /api/system/update/check`

Check for available OTA updates.

### `POST /api/system/update/apply`

Apply a pending OTA update. Triggers atomic symlink swap and restart.

### `POST /api/system/restart`

Restart the actond service.

---

## Setup & Onboarding

### `GET /api/setup/status`

Returns the current onboarding state.

**Response:**
```json
{
  "is_configured": false,
  "step": "wifi_select",
  "available_networks": [
    { "ssid": "HomeNetwork", "signal": -45, "security": "WPA2" }
  ]
}
```

### `POST /api/setup/wifi`

Connect to a Wi-Fi network.

**Request:**
```json
{
  "ssid": "HomeNetwork",
  "password": "wifi_password"
}
```

### `POST /api/setup/api-keys`

Save LLM API keys to the encrypted vault.

**Request:**
```json
{
  "keys": [
    { "provider": "anthropic", "api_key": "sk-ant-..." },
    { "provider": "openai", "api_key": "sk-..." },
    { "provider": "google", "api_key": "AIza..." }
  ]
}
```

### `POST /api/setup/pin`

Set the admin PIN.

**Request:**
```json
{
  "pin": "123456"
}
```

### `POST /api/setup/complete`

Finalize the onboarding process.

---

## Agent Management

### `GET /api/agents`

List all configured agents.

**Response:**
```json
{
  "agents": [
    {
      "agent_id": "agent_dev_assistant_01",
      "name": "Senior Software Architect",
      "description": "Expert in architecture analysis and code generation",
      "avatar_icon": "code-bracket",
      "status": "active",
      "model_config": {
        "primary_model": "anthropic/claude-3-7-sonnet",
        "fallback_model": "google/gemini-2.5-flash",
        "temperature": 0.2
      },
      "created_at": "2026-08-16T10:00:00Z",
      "last_active_at": "2026-08-16T23:55:00Z"
    }
  ]
}
```

### `POST /api/agents`

Create a new agent.

**Request:**
```json
{
  "name": "Research Assistant",
  "description": "Helps with academic research and paper analysis",
  "avatar_icon": "book-open",
  "model_config": {
    "primary_model": "anthropic/claude-3-7-sonnet",
    "fallback_model": "google/gemini-2.5-flash",
    "temperature": 0.3
  },
  "system_instructions": "You are a meticulous research assistant...",
  "authorized_tools": ["mcp_web_fetch", "native_file_ops"],
  "delegation_scope": {
    "max_monthly_budget_usd": 50.0,
    "allowed_workspace_paths": ["/data/workspace/research/"],
    "require_human_approval_level": "Medium"
  },
  "trigger_rules": [
    { "type": "channel_mention", "channel": "telegram", "filter": "@research_bot" }
  ]
}
```

### `GET /api/agents/:id`

Get a specific agent's configuration.

### `PUT /api/agents/:id`

Update an agent's configuration.

### `DELETE /api/agents/:id`

Delete an agent.

### `POST /api/agents/:id/start`

Start/activate an agent.

### `POST /api/agents/:id/stop`

Stop/deactivate an agent.

---

## Chat & Conversations

### `GET /api/agents/:id/conversations`

List conversations for an agent.

### `POST /api/agents/:id/chat`

Send a message to an agent. Supports streaming via Server-Sent Events.

**Request:**
```json
{
  "message": "Analyze the architecture of the ActonOS codebase",
  "conversation_id": "conv_abc123",
  "stream": true
}
```

**Streaming Response (SSE):**
```
event: token
data: {"content": "I'll analyze"}

event: token
data: {"content": " the architecture..."}

event: tool_call
data: {"tool": "skill_run_bash", "args": {"command": "find . -name '*.go' | head -20"}}

event: tool_result
data: {"tool": "skill_run_bash", "result": "..."}

event: done
data: {"conversation_id": "conv_abc123", "tokens_used": 1234}
```

### `GET /api/agents/:id/conversations/:conv_id`

Get full conversation history.

### `DELETE /api/agents/:id/conversations/:conv_id`

Delete a conversation.

---

## Integrations & OAuth

### `GET /api/integrations`

List all available and connected integrations.

**Response:**
```json
{
  "integrations": [
    {
      "id": "google_workspace",
      "name": "Google Workspace",
      "services": ["Gmail", "Drive", "Calendar"],
      "status": "connected",
      "connected_at": "2026-08-16T10:00:00Z",
      "token_expires_at": "2026-08-16T11:00:00Z"
    },
    {
      "id": "notion",
      "name": "Notion",
      "status": "disconnected"
    }
  ]
}
```

### `POST /api/integrations/:id/connect`

Initiate OAuth 2.1 PKCE flow for a service. Returns the authorization URL.

**Response:**
```json
{
  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "state": "random_state_value"
}
```

### `GET /api/integrations/callback`

OAuth callback endpoint (handles authorization code exchange).

### `DELETE /api/integrations/:id/disconnect`

Revoke tokens and disconnect an integration.

---

## Tools & Plugins

### `GET /api/tools`

List all registered tools (MCP servers, WASM plugins, skills).

**Response:**
```json
{
  "tools": [
    {
      "id": "mcp_github",
      "type": "mcp",
      "name": "GitHub MCP Server",
      "transport": "stdio",
      "status": "running",
      "tools_provided": ["create_issue", "list_repos", "read_file"]
    },
    {
      "id": "wasm_code_formatter",
      "type": "wasm",
      "name": "Code Formatter",
      "file": "code_formatter.wasm",
      "status": "loaded"
    },
    {
      "id": "skill_run_bash",
      "type": "skill",
      "name": "Bash Runner",
      "path": "/data/skills/run_bash/",
      "status": "active"
    }
  ]
}
```

### `POST /api/tools/mcp`

Register a new MCP server.

**Request:**
```json
{
  "name": "PostgreSQL MCP",
  "command": "/data/mcp-servers/postgres-mcp",
  "args": ["--connection-string", "postgres://..."],
  "transport": "stdio"
}
```

### `DELETE /api/tools/:id`

Remove a tool registration.

### `POST /api/tools/:id/restart`

Restart a tool (MCP server or WASM plugin).

---

## Workspace & Files

### `GET /api/workspace`

List files in the agent workspace.

**Query Parameters:**
- `path` — subdirectory path (default: `/`)
- `recursive` — include subdirectories (default: `false`)

### `GET /api/workspace/file`

Read a file from the workspace.

**Query Parameters:**
- `path` — file path relative to workspace root

### `POST /api/workspace/file`

Create or update a file in the workspace.

### `DELETE /api/workspace/file`

Delete a file from the workspace.

---

## Error Format

All errors follow a consistent format:

```json
{
  "error": {
    "code": "AGENT_NOT_FOUND",
    "message": "Agent with ID 'agent_xyz' does not exist",
    "details": {}
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|:---|:---|:---|
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource does not exist |
| `VALIDATION_ERROR` | 422 | Invalid request body |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Unexpected server error |

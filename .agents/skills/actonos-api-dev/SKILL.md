---
name: actonos-api-dev
description: "Skill for developing REST API endpoints in internal/server/. Covers routing, request validation, WebSocket streaming, and API conventions."
---

# ActonOS API Development Skill

Use this skill when creating or modifying REST API endpoints in the `internal/server/` package.

---

## 1. Package Overview

```
internal/server/
├── router.go             # Chi router setup, middlewares, the whole route tree (the place to register routes)
├── api_<subsystem>.go    # Handlers grouped by subsystem; `ls internal/server/api_*.go` is authoritative
│   ├── api_auth.go, api_setup.go, api_vault.go, api_providers.go
│   ├── api_agent*.go (agent CRUD, chat, cognition, cron, soul/memory files), api_tasks.go, api_runs.go, api_approvals.go
│   ├── api_conversations.go, api_dashboard.go, api_realtime.go, api_anomalies.go, api_audit.go
│   ├── api_plugins.go, api_tools.go, api_integrations.go (channels, pairing), api_notifications.go
│   ├── api_workspace_db.go (DB-backed workspace), api_embedding.go, api_terminal.go
│   ├── api_system.go, api_backup.go, api_ota_storage.go (metrics, keys, HAL, backups, OTA)
│   └── api_mailcal*.go (mail and calendar: accounts, OAuth, rules, proposals)
├── health.go             # /api/health, /api/ready
├── layered_fs.go, static.go  # Embedded SPA (/data/overrides/ layered over go:embed)
└── server_test.go        # newTestServer(t) and the endpoint test suites
```

Every route must also appear in `.agents/rules/source-registry.md`; `TestSourceRegistryListsEveryRoute` fails when the two drift.

---

## 2. HTTP Framework & Conventions

### Base URL & Versioning
- **Current Base URL**: `/api` (all routes are prefixed with `/api`)
- Do not use `/api/v1/` prefix until v1.0.0 is officially released.

### Router Engine
ActonOS uses [Chi v5](https://github.com/go-chi/chi) with standard library `net/http`:
```go
import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)
```

### Standard Response Envelope

**Success (`s.respondJSON(w, http.StatusOK, data)`):**
```json
{
  "data": { ... }
}
```

**Error (`s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "description")`):**
```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Human-readable explanation of error"
  }
}
```

### Common Helper Methods in `Server`

```go
// 1. Respond with JSON data wrapped in {"data": ...}
func (s *Server) respondJSON(w http.ResponseWriter, status int, data any)

// 2. Respond with standard error envelope wrapped in {"error": {"code": ..., "message": ...}}
func (s *Server) respondError(w http.ResponseWriter, status int, code, message string)

// 3. Decode request body with 1MB safety limit
func (s *Server) decodeJSON(r *http.Request, v any) error
```

---

## 3. Route Organization & Authentication

### Public Routes
Registered at the top of `setupRoutes()` in `router.go`: `/api/health`, `/api/ready`, `/api/models`, `/api/notifications/push/vapid-key`, `/api/auth/{status,setup,login,logout}`, and `/api/mailcal/oauth/callback` (the provider redirects the browser here without the SameSite=Strict session cookie; the handler is protected by single-use state and a nonce cookie). Keep this list as short as possible; a new public route needs a written reason in the handler.

### Protected Subsystems
Every other route is nested inside the group that uses `s.RequireAuthMiddleware`. The browser authenticates with the HttpOnly `actonos_token` cookie; API clients may send `Authorization: Bearer <token>` (`api_auth.go`).

---

## 4. Implementing a New API Endpoint

### Step 1: Pick the closest existing handler

Find the neighbour in `internal/server/api_<subsystem>.go` (for example `handleSaveSoul` in `api_agent_files.go`, or a `handleMailcal*` handler in `api_mailcal_rules.go`) and copy its shape: define request/response types next to it, decode with `s.decodeJSON`, validate, call the subsystem, answer with `s.respondJSON` or `s.respondError(w, status, "UPPER_SNAKE_CODE", message)`. Nil-check optional subsystems (`if s.profileMgr == nil { ... 501 }`). Handlers contain no business logic; that belongs in the `internal/<subsystem>` package so it can be tested without HTTP.

### Step 2: Test it

Use `newTestServer(t)` (`server_test.go`) and `httptest` against `srv.Router()`; mailcal examples: `api_mailcal_rules_test.go`.

### Step 3: Register the route in `internal/server/router.go`

Add it inside the existing `r.Route(...)` block of its subsystem (for example `r.Put("/soul", s.handleSaveSoul)` under `/agents/{agentID}`). Aliases are separate routes and each needs its registry row.

### Step 4: Update Documentation and TypeScript Types
1. Add endpoint entry to `docs/API.md`
2. Add route and handler to `.agents/rules/source-registry.md`
3. Add the client method (`web/src/lib/api.ts` or a domain client under `web/src/lib/api/`) and interfaces in `web/src/lib/types.ts`
4. `go test ./internal/server -run TestSourceRegistryListsEveryRoute` must pass

---

## 5. Streaming Endpoints (SSE / Server-Sent Events)

For streaming LLM tokens, reasoning thoughts, and tool call progress:

```go
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        s.respondError(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming not supported")
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    eventChan := make(chan agent.AgentStreamEvent, 64)
    go func() {
        _, _ = s.engine.ExecuteStepStreamWithHistory(
            r.Context(), agentID, msg, history, eventChan,
        )
    }()

    for ev := range eventChan {
        data, _ := json.Marshal(ev)
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
        flusher.Flush()
    }
}
```

The stream endpoint must flush live `thought`, `token`, `tool_call`,
`tool_result`, `audit`, `done`, and `error` events. It must not proxy to the
non-streaming JSON handler. Conversation messages are persisted before and
after the stream.

### Realtime Operations WebSocket

- `GET /api/realtime` is a protected, same-origin WebSocket.
- Browser authentication uses the HttpOnly `actonos_token` cookie set by setup/login; never put bearer tokens in WebSocket query strings.
- Snapshots include hardware/Docker metrics, durable runs, pending approvals and token summaries.
- The stream is observation-only and must not expose an interactive shell or mutation channel.

---

## 6. Verification Checklist for API Changes

- [ ] Route is registered inside `router.go`
- [ ] Authentication middleware requirement is verified
- [ ] Error responses use standard `s.respondError` with appropriate HTTP status
- [ ] TypeScript types in `web/src/lib/types.ts` and `api.ts` are synced
- [ ] `docs/API.md` and `.agents/rules/source-registry.md` are updated
- [ ] `go test ./internal/server/...` passes

---

## 7. Approval and Run APIs

- `api_approvals.go` owns durable exact-action approval decisions.
- `api_runs.go` exposes durable run summaries and ordered execution events.
- Authentication is not tool authorization. Execution handlers MUST call
  `ToolRegistry.Execute` and return HTTP 202 for `ApprovalRequiredError`.
- MCP connection is a High-risk administrative action and requires approval.
- Preserve trace IDs through handler, engine, registry, approval, and audit calls.
- Workspace, skill, WASM, Hub, and restart mutations must use
  `requestAdminAction` and exact dispatch (`executeAdminAction`) in `admin_actions.go`.
- `GET /api/system/audit/verify` is the canonical audit-chain integrity check.
- Checkpointed approvals resume the same durable run; do not create a replacement.
- All server filesystem paths must derive from `Config.DataDir`,
  `Config.WorkspaceDir`, `Config.SkillsDir`, or `Config.WASMDir`; handlers must
  not assume the process working directory is the data root.
- Provider credentials must use `Server.vault`; never write API keys into JSON
  or `.key` files. Preserve automatic legacy migration and fail closed without
  Vault.
- SQLite backups must use `VACUUM INTO` through the live database connection;
  never copy the main database file while WAL mode is active.

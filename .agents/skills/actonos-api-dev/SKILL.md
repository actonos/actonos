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
├── router.go             # Chi router setup, global & auth middlewares, route tree
├── api_auth.go           # Setup, login, logout, password change, auth status
├── api_dashboard.go      # Dashboard aggregate metrics & summaries
├── api_agent.go          # Agent CRUD, start/stop, chat, soul, memory-md, cron
├── api_tasks.go          # Autonomous Task matrix CRUD, Heartbeat config & manual pulse triggers
├── api_conversations.go  # Chat conversations and message history
├── api_integrations.go   # OAuth, WhatsApp/Telegram/Discord channels, pairing
├── api_tools.go          # MCP servers, WASM plugins, skills, hub marketplace
├── api_workspace.go      # Workspace file browser, read/write/mkdir/upload
├── api_system.go         # Metrics, token usage ledger history, keys, identity, HAL
├── api_setup.go          # Legacy/standalone setup endpoints
├── layered_fs.go         # Layered filesystem (/data/overrides/ → go:embed fallback)
├── static.go             # Embedded static asset server
└── server_test.go        # Comprehensive endpoint test suite
```

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
- `GET /api/health`
- `GET /api/auth/status`
- `POST /api/auth/setup`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/callback`
- `GET/POST /api/webhooks/whatsapp`

### Protected Subsystems
All other routes are nested inside `r.Group` with `r.Use(s.RequireAuthMiddleware)`. Requests must include `Authorization: Bearer <token>` in the HTTP headers when authentication is initialized.

---

## 4. Implementing a New API Endpoint

### Step 1: Add Request & Response Structs

```go
type UpdateSoulRequest struct {
    SoulContent string `json:"soul_content"`
}

type SoulResponse struct {
    AgentID   string `json:"agent_id"`
    Content   string `json:"content"`
    UpdatedAt string `json:"updated_at"`
}
```

### Step 2: Implement Handler Method on `*Server`

```go
func (s *Server) handleSaveSoul(w http.ResponseWriter, r *http.Request) {
    agentID := chi.URLParam(r, "agentID")
    if agentID == "" {
        agentID = agent.DefaultSystemAgentID
    }

    var req UpdateSoulRequest
    if err := s.decodeJSON(r, &req); err != nil {
        s.respondError(w, http.StatusBadRequest, "INVALID_BODY", "failed to decode json body")
        return
    }

    if err := s.profileMgr.SaveSoul(r.Context(), agentID, req.SoulContent); err != nil {
        s.respondError(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error())
        return
    }

    s.respondJSON(w, http.StatusOK, SoulResponse{
        AgentID:   agentID,
        Content:   req.SoulContent,
        UpdatedAt: time.Now().UTC().Format(time.RFC3339),
    })
}
```

### Step 3: Register Route in `internal/server/router.go`

```go
r.Route("/agents", func(r chi.Router) {
    // ...
    r.Route("/{agentID}", func(r chi.Router) {
        // ...
        r.Put("/soul", s.handleSaveSoul)
    })
})
```

### Step 4: Update Documentation and TypeScript Types
1. Add endpoint entry to `docs/API.md`
2. Add route and handler to `.agents/rules/source-registry.md`
3. Add client method to `web/src/lib/api.ts` and interfaces to `web/src/lib/types.ts`

---

## 5. Streaming Endpoints (SSE / Server-Sent Events)

For streaming LLM tokens, reasoning thoughts, and tool call progress:

```go
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        s.respondError(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming not supported")
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    eventChan, err := s.engine.StreamChat(r.Context(), agentID, msg)
    if err != nil {
        s.respondError(w, http.StatusInternalServerError, "STREAM_FAILED", err.Error())
        return
    }

    for ev := range eventChan {
        data, _ := json.Marshal(ev)
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
        flusher.Flush()
    }
}
```

---

## 6. Verification Checklist for API Changes

- [ ] Route is registered inside `router.go`
- [ ] Authentication middleware requirement is verified
- [ ] Error responses use standard `s.respondError` with appropriate HTTP status
- [ ] TypeScript types in `web/src/lib/types.ts` and `api.ts` are synced
- [ ] `docs/API.md` and `.agents/rules/source-registry.md` are updated
- [ ] `go test ./internal/server/...` passes

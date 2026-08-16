---
name: actonos-api-dev
description: "Skill for developing REST API endpoints in internal/server/. Covers routing, request validation, WebSocket streaming, and API conventions."
---

# ActonOS API Development Skill

Use this skill when creating or modifying REST API endpoints in the `internal/server/` package.

## Package Overview

```
internal/server/
├── router.go             # Chi router setup, middleware stack, WebSocket hub
├── layered_fs.go         # Override layer: /data/overrides/ → go:embed fallback
├── api_setup.go          # Onboarding & Wi-Fi configuration endpoints
├── api_agent.go          # Agent CRUD endpoints
├── api_integrations.go   # OAuth & SaaS integration endpoints
├── api_tools.go          # MCP, Skills, WASM plugin management endpoints
├── api_system.go         # Hardware metrics, Tailscale, OTA update endpoints
└── static.go             # go:embed static asset server
```

## HTTP Framework

ActonOS uses [chi](https://github.com/go-chi/chi) as the HTTP router:

```go
import "github.com/go-chi/chi/v5"
import "github.com/go-chi/chi/v5/middleware"
```

## API Conventions

### URL Structure

```
/api/v1/<resource>           # Collection
/api/v1/<resource>/:id       # Individual resource
/api/v1/<resource>/:id/action  # Resource action
```

**Current (pre-v1):** Use `/api/<resource>` without version prefix until API stabilizes at v1.0.0.

### HTTP Methods

| Method | Semantics | Example |
|:---|:---|:---|
| `GET` | Read (list or single) | `GET /api/agents`, `GET /api/agents/:id` |
| `POST` | Create or action | `POST /api/agents`, `POST /api/agents/:id/start` |
| `PUT` | Full update | `PUT /api/agents/:id` |
| `PATCH` | Partial update | `PATCH /api/agents/:id` |
| `DELETE` | Delete | `DELETE /api/agents/:id` |

### Response Format

**Success:**
```json
{
  "data": { ... }
}
```

**Error:**
```json
{
  "error": {
    "code": "AGENT_NOT_FOUND",
    "message": "Agent with ID 'xyz' does not exist"
  }
}
```

### Status Codes

| Code | Usage |
|:---|:---|
| `200 OK` | Successful GET, PUT, PATCH |
| `201 Created` | Successful POST (resource creation) |
| `204 No Content` | Successful DELETE |
| `400 Bad Request` | Malformed request |
| `401 Unauthorized` | Missing/invalid auth |
| `403 Forbidden` | Insufficient permissions |
| `404 Not Found` | Resource doesn't exist |
| `422 Unprocessable Entity` | Validation error |
| `429 Too Many Requests` | Rate limited |
| `500 Internal Server Error` | Unexpected error |

## Adding a New Endpoint

### Step 1: Define Request/Response Types

```go
// internal/server/api_agent.go

type CreateAgentRequest struct {
    Name               string         `json:"name" validate:"required,min=1,max=100"`
    Description        string         `json:"description"`
    ModelConfig        ModelConfig    `json:"model_config" validate:"required"`
    SystemInstructions string         `json:"system_instructions"`
    AuthorizedTools    []string       `json:"authorized_tools"`
    DelegationScope    DelegationScope `json:"delegation_scope"`
}

type AgentResponse struct {
    AgentID     string    `json:"agent_id"`
    Name        string    `json:"name"`
    Status      string    `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    // ...
}
```

### Step 2: Implement the Handler

```go
func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // 1. Decode and validate request
    var req CreateAgentRequest
    if err := s.decodeJSON(r, &req); err != nil {
        s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }

    if err := s.validate(req); err != nil {
        s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
        return
    }

    // 2. Execute business logic
    agent, err := s.agentManager.Create(ctx, req.toManifest())
    if err != nil {
        slog.Error("failed to create agent", "error", err)
        s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create agent")
        return
    }

    // 3. Respond
    s.respondJSON(w, http.StatusCreated, AgentResponse{
        AgentID:   agent.ID,
        Name:      agent.Name,
        Status:    "active",
        CreatedAt: agent.CreatedAt,
    })
}
```

### Step 3: Register the Route

```go
// internal/server/router.go

func (s *Server) setupRoutes() chi.Router {
    r := chi.NewRouter()

    // Middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(30 * time.Second))

    // Public routes (no auth)
    r.Get("/api/health", s.handleHealth)
    r.Route("/api/setup", func(r chi.Router) {
        r.Get("/status", s.handleSetupStatus)
        r.Post("/wifi", s.handleSetupWifi)
        r.Post("/api-keys", s.handleSetupAPIKeys)
        r.Post("/pin", s.handleSetupPin)
        r.Post("/complete", s.handleSetupComplete)
    })

    // Authenticated routes
    r.Group(func(r chi.Router) {
        r.Use(s.authMiddleware)

        r.Route("/api/agents", func(r chi.Router) {
            r.Get("/", s.handleListAgents)
            r.Post("/", s.handleCreateAgent)       // ← New endpoint
            r.Route("/{agentID}", func(r chi.Router) {
                r.Get("/", s.handleGetAgent)
                r.Put("/", s.handleUpdateAgent)
                r.Delete("/", s.handleDeleteAgent)
                r.Post("/start", s.handleStartAgent)
                r.Post("/stop", s.handleStopAgent)
                r.Post("/chat", s.handleChat)
            })
        })
    })

    return r
}
```

### Step 4: Helper Methods

```go
// JSON response helper
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// Error response helper
func (s *Server) respondError(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error": map[string]string{
            "code":    code,
            "message": message,
        },
    })
}

// JSON decoder with max body size
func (s *Server) decodeJSON(r *http.Request, v interface{}) error {
    r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB limit
    return json.NewDecoder(r.Body).Decode(v)
}
```

## WebSocket Streaming (Chat)

Chat responses use Server-Sent Events (SSE) for streaming:

```go
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
    agentID := chi.URLParam(r, "agentID")
    flusher, ok := w.(http.Flusher)
    if !ok {
        s.respondError(w, 500, "STREAMING_ERROR", "streaming not supported")
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Stream tokens
    for token := range s.engine.StreamResponse(r.Context(), agentID, msg) {
        fmt.Fprintf(w, "event: token\ndata: %s\n\n", token)
        flusher.Flush()
    }

    fmt.Fprintf(w, "event: done\ndata: {}\n\n")
    flusher.Flush()
}
```

## Testing API Endpoints

```go
func TestHandleCreateAgent(t *testing.T) {
    srv := NewTestServer(t)

    body := `{"name":"Test Agent","model_config":{"primary_model":"test/model"}}`
    req := httptest.NewRequest("POST", "/api/agents", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer test-token")

    w := httptest.NewRecorder()
    srv.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
    }

    var resp map[string]AgentResponse
    json.NewDecoder(w.Body).Decode(&resp)
    if resp["data"].Name != "Test Agent" {
        t.Errorf("unexpected name: %s", resp["data"].Name)
    }
}
```

## Reference Files

- [docs/API.md](../../../docs/API.md) — Full API reference
- [docs/ARCHITECTURE.md](../../../docs/ARCHITECTURE.md) — System architecture

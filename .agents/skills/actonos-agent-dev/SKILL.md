---
name: actonos-agent-dev
description: "Skill for developing AI agent engine components in internal/agent/. Covers the ReAct loop, swarm delegation, planning, verification, and memory reflection."
---

# ActonOS Agent Development Skill

Use this skill when developing components in the `internal/agent/` package — the core AI engine that powers all user-created agents.

## Package Overview

```
internal/agent/
├── engine.go       # POMDP & ReAct state machine (main orchestration loop)
├── manager.go      # Agent CRUD — create, read, update, delete agent manifests
├── swarm.go        # Multi-agent swarm delegation via Goroutines
├── planner.go      # Task decomposition and planning (Tree-of-Thoughts, LATS)
├── verifier.go     # Deterministic static analysis (AST, invariant, schema)
├── reflection.go   # Background fact extraction and learning (async goroutine)
├── profile.go      # User persona management and dynamic preferences
├── heartbeat.go    # Proactive task daemon (cron-scheduled agent actions)
├── context.go      # Context window management and token pruning
└── types.go        # Shared types, interfaces, and constants
```

## Core Concepts

### Agent Manifest (types.go)

Every agent is defined by a manifest:

```go
type AgentManifest struct {
    AgentID           string          `json:"agent_id"`
    Name              string          `json:"name"`
    Description       string          `json:"description"`
    AvatarIcon        string          `json:"avatar_icon"`
    ModelConfig       ModelConfig     `json:"model_config"`
    SystemInstructions string         `json:"system_instructions"`
    AuthorizedTools   []string        `json:"authorized_tools"`
    DelegationScope   DelegationScope `json:"delegation_scope"`
    TriggerRules      []TriggerRule   `json:"trigger_rules"`
}

type ModelConfig struct {
    PrimaryModel  string  `json:"primary_model"`
    FallbackModel string  `json:"fallback_model"`
    Temperature   float64 `json:"temperature"`
}

type DelegationScope struct {
    MaxMonthlyBudgetUSD     float64  `json:"max_monthly_budget_usd"`
    AllowedWorkspacePaths   []string `json:"allowed_workspace_paths"`
    RequireHumanApproval    string   `json:"require_human_approval_level"` // Low, Medium, High
}
```

### ReAct Loop (engine.go)

The main agent execution loop follows the ReAct (Reasoning + Acting) pattern:

```
User Message → Entropy Check → [Greedy 1-Step | Tree Search] → Tool Calls → Verification → Response
```

Key decision point (Uncertainty-Gated):
- **Low entropy (H < θ)**: Greedy ReAct — single-step execution for ultra-low latency
- **High entropy (H ≥ θ)**: Tree-of-Thoughts / LATS — multi-path search with reward function

### Swarm Delegation (swarm.go)

Orchestration agents decompose complex tasks and dispatch to sub-agents:

```go
type SwarmManager struct {
    agentManager *AgentManager
    bus          *bus.EventBus
}

// SpawnSubAgent creates a sub-agent goroutine for a specific sub-task
func (s *SwarmManager) SpawnSubAgent(ctx context.Context, parentID string, task SubTask) (<-chan SubTaskResult, error) {
    // 1. Find or create the specialized sub-agent
    // 2. Launch in a new goroutine
    // 3. Return result channel
}
```

### Verification (verifier.go)

Two-tier verification system:

**Tier 1 — Static Analysis (Pure Go, ~0ms):**
- AST parsing for Shell, Python, JSON, SQL
- Path escape detection (must stay within `/workspace`)
- JSON schema validation
- Blocks immediately on violation

**Tier 2 — Semantic Verification:**
- Content consistency against user profile
- Activated for language-logic tasks

### Memory Reflection (reflection.go)

Background goroutine that:
1. Analyzes completed conversations
2. Extracts facts, preferences, and patterns
3. Updates User Profile Memory and Procedural Memory
4. Runs asynchronously to avoid blocking the main loop

## Development Guidelines

### Adding a New Agent Capability

1. Define the interface in `types.go`
2. Implement in a new or existing file
3. Wire it into `engine.go`'s ReAct loop
4. Add unit tests with table-driven patterns
5. Update `docs/ARCHITECTURE.md`

### Working with the Event Bus

Agents communicate through `internal/bus/`:

```go
// Publishing an event
s.bus.Publish(bus.Event{
    Type:    bus.EventToolResult,
    AgentID: agentID,
    Payload: result,
})

// Subscribing to events
ch := s.bus.Subscribe(bus.EventUserMessage)
for event := range ch {
    // Handle incoming message
}
```

### Error Handling Pattern

```go
func (e *Engine) ExecuteStep(ctx context.Context, agentID string, msg Message) (*Response, error) {
    agent, err := e.manager.Get(ctx, agentID)
    if err != nil {
        return nil, fmt.Errorf("getting agent %s: %w", agentID, err)
    }

    // ... execute step ...

    if err := e.verifier.Check(ctx, response); err != nil {
        slog.Warn("verification failed, retrying",
            "agent_id", agentID,
            "error", err,
        )
        return e.retry(ctx, agent, msg, err)
    }

    return response, nil
}
```

### Testing Agent Components

```go
func TestEngine_ExecuteStep(t *testing.T) {
    tests := []struct {
        name        string
        message     Message
        mockLLM     *MockLLMProvider
        wantErr     bool
        wantTools   []string
    }{
        {
            name:    "simple_greeting",
            message: Message{Content: "Hello"},
            mockLLM: NewMockLLM(Response{Content: "Hi there!"}),
        },
        {
            name:    "tool_call_triggered",
            message: Message{Content: "Search for ActonOS docs"},
            mockLLM: NewMockLLM(Response{ToolCalls: []ToolCall{{Name: "web_fetch"}}}),
            wantTools: []string{"web_fetch"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            engine := NewEngine(tt.mockLLM, NewTestVerifier(), NewTestMemory())
            resp, err := engine.ExecuteStep(context.Background(), "test-agent", tt.message)
            if (err != nil) != tt.wantErr {
                t.Fatalf("ExecuteStep() error = %v, wantErr %v", err, tt.wantErr)
            }
            if resp != nil && len(tt.wantTools) > 0 {
                // verify tool calls
            }
        })
    }
}
```

## Key Dependencies

| Package | Import Path | Purpose |
|:---|:---|:---|
| Event Bus | `internal/bus` | Inter-component messaging |
| LLM Router | `internal/llm` | LLM provider abstraction |
| Tools | `internal/tools` | Tool registry and execution |
| Memory | `internal/memory` | Hybrid RAG and vault |
| Sandbox | `internal/sandbox` | Command execution isolation |

## Reference Files

- [docs/ARCHITECTURE.md](../../../docs/ARCHITECTURE.md) — Full architecture spec
- [internal/bus/eventbus.go](../../../internal/bus/eventbus.go) — Event bus implementation
- [internal/llm/provider.go](../../../internal/llm/provider.go) — LLM provider interface

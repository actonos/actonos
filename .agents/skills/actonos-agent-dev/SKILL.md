---
name: actonos-agent-dev
description: "Skill for developing AI agent engine components in internal/agent/. Covers the ReAct loop, swarm delegation, planning, verification, memory reflection, and cron automation."
---

# ActonOS Agent Development Skill

Use this skill when developing components in the `internal/agent/` package — the core AI engine powering autonomous execution, reasoning, multi-agent swarms, and scheduled automations.

---

## 1. Package Overview

```
internal/agent/
├── engine.go            # POMDP & ReAct execution loop with streaming SSE events
├── manager.go           # Agent CRUD & persistence (AgentManifest stored in SQLite/JSON)
├── cron_scheduler.go    # Scheduled autonomous task engine (cron expressions & triggers)
├── swarm.go             # Multi-agent swarm delegation via Goroutines & channels
├── planner.go           # Dynamic task decomposition & multi-path tree search (LATS)
├── verifier.go          # Two-tier deterministic static analysis & semantic verification
├── reflection.go        # Async background learning, fact extraction, and memory update
├── profile.go           # User persona, profile management, dynamic SOUL.md & MEMORY.md
├── heartbeat.go         # Proactive heartbeat daemon triggering periodic checks
├── context.go           # Context window management, message sliding, and token pruning
├── types.go             # Manifests, delegation scopes, stream events, audit log types
└── *_test.go            # Unit & integration test suites for all agent capabilities
```

---

## 2. Core Structs & Data Contracts (`types.go`)

### Agent Manifest
```go
type AgentManifest struct {
    AgentID             string           `json:"agent_id"`
    Name                string           `json:"name"`
    Description         string           `json:"description"`
    AvatarIcon          string           `json:"avatar_icon"`
    Status              AgentStatus      `json:"status"` // "active", "stopped", "error"
    IsSystem            bool             `json:"is_system,omitempty"`
    ModelConfig         llm.ModelConfig  `json:"model_config"`
    SystemInstructions string           `json:"system_instructions"`
    AuthorizedTools     []string         `json:"authorized_tools"`
    ListenChannels      []string         `json:"listen_channels"` // ["*"] for all, or specific ["telegram"]
    DelegationScope     DelegationScope  `json:"delegation_scope"`
    TriggerRules        []TriggerRule    `json:"trigger_rules"`
    CreatedAt           time.Time        `json:"created_at"`
    UpdatedAt           time.Time        `json:"updated_at"`
}
```

### Delegation Scope & Approval Levels
```go
type DelegationScope struct {
    MaxMonthlyBudgetUSD     float64       `json:"max_monthly_budget_usd"`
    AllowedWorkspacePaths   []string      `json:"allowed_workspace_paths"`
    RequireHumanApproval    ApprovalLevel `json:"require_human_approval_level"` // "Low", "Medium", "High"
}
```

### Streaming Events (`AgentStreamEvent`)
The agent engine emits structured stream events:
- `thought`: Internal reasoning chain (ReAct)
- `token`: Streamed text content chunks
- `tool_call`: Invocation of an authorized tool
- `tool_result`: Output payload returned by tool execution
- `audit`: Security/governance log entry
- `done`: Completion of execution step
- `error`: Execution failure details

---

## 3. Cognitive Subsystems

### A. ReAct Reasoning Loop (`engine.go`)
1. **Entropy Check**: Evaluates uncertainty $H(p)$. Low entropy $\rightarrow$ 1-step Greedy ReAct; High entropy $\rightarrow$ Tree-of-Thoughts exploration.
2. **Tool Invocation**: Validates tool against `agent.AuthorizedTools` and executes inside isolated sandbox or MCP host.
3. **Verification**: Passes candidate output through `verifier.go` (AST checks + policy guards) before returning to user.

### B. Dynamic Soul & Memory Files (`profile.go`)
- **`SOUL.md`**: Dedicated per-agent identity and instructions.
- **`MEMORY.md`**: Persisted long-term reflections and episodic summaries, auto-updated by `reflection.go`.

### C. Swarm Delegation (`swarm.go`)
Parent agents can spawn asynchronous sub-agents with constrained budgets and scoped tool access:
```go
func (s *SwarmManager) SpawnSubAgent(ctx context.Context, parentID string, task SubTask) (<-chan SubTaskResult, error)
```

### D. Scheduled Automations (`cron_scheduler.go`)
Manages cron expressions (e.g. `0 9 * * *`), dispatches autonomous prompt triggers, and records run logs.

---

## 4. Key Dependencies

| Subsystem | Package | Integration Purpose |
|:---|:---|:---|
| **Event Bus** | `internal/bus` | Decoupled event publishing (`bus.EventAgentMessage`, etc.) |
| **LLM Router** | `internal/llm` | Multi-model cascade and failover provider routing |
| **Memory Engine** | `internal/memory` | Hybrid RAG, vector retrieval, and FTS5 decay search |
| **Channels** | `internal/channels` | Multi-channel ingress and response dispatch |
| **Tools Hub** | `internal/tools` | Native tools, MCP clients, and WASM runner |
| **Auth** | `internal/auth` | User profile encryption and token lifecycle |

---

## 5. Development & Testing Rules

1. Always add table-driven tests in `*_test.go`.
2. Wrap all error returns with context: `fmt.Errorf("agent %s step failed: %w", agentID, err)`.
3. Keep Goroutines governed by `context.Context` cancellation.
4. When modifying `AgentManifest` or `DelegationScope`, update `web/src/lib/types.ts` immediately.

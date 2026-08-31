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
├── tasks.go             # Autonomous Task Backlog Manager with SQLite & bi-directional TASKS.md sync
├── cron_scheduler.go    # Scheduled autonomous task engine (cron expressions & anti-double-dispatch)
├── heartbeat.go         # Autonomous cognitive heartbeat pulse with session resume & zero-noise policy
├── proactive.go         # 7-probe background anomaly detection & proactive health triaging
├── swarm.go             # Multi-agent swarm delegation via Goroutines & channels
├── planner.go           # Dynamic DAG task decomposition, concurrent burst pulse & step assertions
├── verifier.go          # Multi-tier verification coordinator
├── verifier_outcome.go  # Tier-3 empirical outcome assertion grounding engine (8 assertion kinds)
├── directive_verifier.go# Structured standing directive validation
├── reflection.go        # Async background learning, self-review proposals, and memory retention
├── profile.go           # User persona, profile management, dynamic SOUL.md & MEMORY.md
├── context.go           # Context window manager, token pruning & AutoSummarizeObservation (Context Shield)
├── types.go             # Manifests, delegation scopes, stream events, audit log types
└── *_test.go            # Unit & integration test suites for all agent capabilities
```

---

## 2. Core Structs & Data Contracts (`types.go`, `tasks.go`)

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

### Autonomous Task Model (`tasks.go`)
```go
type AutonomousTask struct {
    ID              string     `json:"id"`
    Title           string     `json:"title"`
    Description     string     `json:"description"`
    Status          string     `json:"status"`            // "pending", "in_progress", "completed", "blocked", "cancelled"
    Priority        string     `json:"priority"`          // "p0_critical", "p1_high", "p2_normal", "p3_low"
    AssignedAgentID string     `json:"assigned_agent_id"` // "auto", "agent_system_core", or specific ID
    TargetChannel   string     `json:"target_channel,omitempty"`
    TargetAccountID string     `json:"target_account_id,omitempty"`
    Progress        int        `json:"progress"` // 0 to 100%
    ExecutionLog    string     `json:"execution_log,omitempty"`
    SessionID       string     `json:"session_id,omitempty"`
    CreatedBy       string     `json:"created_by"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
    CompletedAt     *time.Time `json:"completed_at,omitempty"`
}
```

---

## 3. Cognitive Subsystems

### A. ReAct Reasoning Loop (`engine.go`)
1. **Entropy Check**: Evaluates uncertainty $H(p)$. Low entropy $\rightarrow$ 1-step Greedy ReAct; High entropy $\rightarrow$ Tree-of-Thoughts exploration.
2. **Tool Invocation**: Validates tool against `agent.AuthorizedTools` and executes inside isolated sandbox or MCP host.
3. **Verification**: Passes candidate output through `verifier.go` (AST checks + policy guards) before returning to user.

### B. Autonomous Task Matrix & Mission Backlog (`tasks.go`)
- Maintains persistent queue in SQLite `autonomous_tasks`.
- Bi-directional synchronization with `data/workspace/TASKS.md` ensures CLI, file tools, and LLMs see identical task structures.

### C. Heartbeat Cognitive Pulse & Working Memory Continuity (`heartbeat.go`)
Follows [OpenClaw's Heartbeat contract](https://docs.openclaw.ai/vi/gateway/heartbeat) — see
`docs/ARCHITECTURE.md` §4.C for full diagrams.
- **Gated triggers**: scheduled ticks and event wakeups (`TriggerWakeup()`, e.g. task/approval mutations)
  pass through `checkCycle(ctx, manual=false)`, which enforces a 15s trigger cooldown (coalesces trigger
  storms), an optional daily `activeHours` window, and an idle guard (`hasActionableHeartbeatDirectives()`)
  that skips the model entirely when there's no active task and no actionable `HEARTBEAT.md` content. Manual
  "Pulse Now" calls (`manual=true`) bypass all three gates.
- **Session Resume**: For each mission, resumes its dedicated `chat_sessions` (`conv_task_<id>`) and loads previous step history (`LoadRecentHistory`), enabling multi-pulse problem solving without losing context.
- **Hard tool boundary**: both mission and routine cycles run inside a context that hard-denies
  `native_channel_notify`/`channel_notify` (delivery is the daemon's job) and `native_cron_schedule`
  (recurring automations always need an explicit operator request) via `tools.WithDeniedTools`, enforced
  inside `ToolRegistry.Execute` itself — not just a prompt instruction.
- **Response contract**: `classifyHeartbeatResponse()` only treats `HEARTBEAT_OK` as silent when the token
  is at the start/end of the reply and the remainder is ≤ `ackMaxChars` (default 300, configurable). Anything
  else — including off-directive hallucinated chatter — is delivered as a real alert exactly once.
- **Anti-Double-Dispatch**: Suppresses duplicate notifications if the agent already pushed a message via tools, and `tools.ApprovalRequest.IsNew()` prevents re-publishing `approval:required` for an already-pending approval.

### D. Scheduled Automations (`cron_scheduler.go`)
Manages cron expressions (e.g. `0 9 * * *`), dispatches autonomous prompt triggers, and records SQLite execution runs.

---

## 4. Key Dependencies

| Subsystem | Package | Integration Purpose |
|:---|:---|:---|
| LLM | `internal/llm/` | ModelCascadeRouter, OpenAI-compatible streaming completion |
| Memory | `internal/memory/` | HybridEngine (Chroma vector + SQLite FTS5) + TokenTracker |
| Channels | `internal/channels/` | ChannelSessionManager for multi-step task working memory |
| Tools | `internal/tools/` | ToolRegistry, native tools, MCP client, WASM runner |
| Event Bus | `internal/bus/` | Async decoupling, stream event broadcasting |
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

---

## 6. Secure Durable Execution

- `runs.go` persists `agent_runs` and append-only `run_events`.
- Autonomous prompts receive planner decomposition on their first step.
- `ContextManager` budgets messages before every LLM attempt.
- All tools MUST execute through `ToolRegistry.Execute`.
- The engine stops after 20 iterations, 5 consecutive tool failures, repeated
  equivalent observations, cancellation, budget exhaustion, or verification failure.
- `[TASK_COMPLETED]` is advisory until `Verifier.VerifyTaskCompletion` accepts it.
- Aggregate token usage covers every LLM attempt in the ReAct loop.
- Swarm sub-tasks must use the shared Engine when configured; direct LLM fallback
  exists only for isolated unit construction.
- Periodic reflection deduplicates episodic memories and removes stale,
  never-accessed low-importance entries after six months.
- Use `Planner.ExecutePlan` for dependency-aware DAG execution; reject duplicate
  IDs, unknown dependencies, and cycles.
- Approval pauses must persist `RunCheckpoint` and resume through
  `Engine.ResumeApproved`; never restart the original goal.
- Context pruning must use `PruneAndSnapshot` to persist compaction provenance.

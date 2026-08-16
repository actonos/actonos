# ActonOS Coding Standards

> Enforced coding standards for all ActonOS contributions.

## Go Standards

### Naming Conventions

- **Packages**: short, lowercase, single word (e.g., `agent`, `memory`, `bus`)
- **Interfaces**: verb-based or capability-based (e.g., `LLMProvider`, `ChannelAdapter`, `Executor`)
- **Structs**: noun-based (e.g., `AgentManager`, `TokenRefresher`, `VaultStore`)
- **Functions**: verb-based (e.g., `CreateAgent`, `RefreshToken`, `DecryptKey`)
- **Constants**: `CamelCase` for exported, `camelCase` for unexported
- **Error variables**: `Err` prefix (e.g., `ErrAgentNotFound`, `ErrTokenExpired`)

### Error Handling

```go
// ✅ Good: wrap errors with context
if err := db.Save(agent); err != nil {
    return fmt.Errorf("saving agent %s: %w", agent.ID, err)
}

// ❌ Bad: raw error propagation
if err := db.Save(agent); err != nil {
    return err
}

// ❌ Bad: uppercase error messages
return fmt.Errorf("Failed to save agent: %w", err)
```

### Logging

Use `log/slog` for structured logging:

```go
// ✅ Good
slog.Info("agent started",
    "agent_id", agent.ID,
    "model", agent.ModelConfig.PrimaryModel,
)

slog.Error("tool execution failed",
    "agent_id", agent.ID,
    "tool", toolName,
    "error", err,
)

// ❌ Bad: fmt.Printf or log.Printf
log.Printf("Agent %s started", agent.ID)
```

### Context Usage

```go
// ✅ Good: context as first parameter
func (m *AgentManager) CreateAgent(ctx context.Context, manifest AgentManifest) (*Agent, error) {
    // ...
}

// ❌ Bad: no context
func (m *AgentManager) CreateAgent(manifest AgentManifest) (*Agent, error) {
    // ...
}
```

### Function Length

- Keep functions under **60 lines** where possible
- Extract helper functions for complex logic
- Each function should do **one thing**

### Testing

- Use **table-driven tests** for parameterized scenarios
- Name test cases descriptively: `TestDecayScore/recent_memory_high_score`
- Use `testify` assertions sparingly; prefer stdlib `testing` package
- Integration tests must have `//go:build integration` build tag

## TypeScript/React Standards

### Component Structure

```tsx
// ✅ Good: named export, functional component
export function AgentCard({ agent }: AgentCardProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  return (
    <div className="agent-card">
      {/* ... */}
    </div>
  );
}

// ❌ Bad: default export
export default function AgentCard() { /* ... */ }
```

### Type Safety

```tsx
// ✅ Good: explicit types
interface AgentCardProps {
  agent: Agent;
  onSelect: (id: string) => void;
}

// ❌ Bad: any
function AgentCard(props: any) { /* ... */ }
```

### State Management

- Use React hooks for local state
- Use Context API for shared state across a page
- Avoid prop drilling beyond 2 levels — use context instead

## Git Conventions

### Branch Names

```
feat/swarm-delegation-timeout
fix/fts5-concurrent-write
docs/api-reference-oauth
refactor/llm-retry-middleware
```

### Commit Messages

```
feat(agent): implement swarm delegation with configurable timeout
fix(memory): prevent concurrent FTS5 write corruption
docs: update API reference with new agent endpoints
test(auth): add edge case tests for expired token refresh
```

### PR Size

- Target **under 400 lines** of changes per PR
- Split large features into incremental PRs
- Each PR should be independently reviewable and testable

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

### Design System Compliance (`DESIGN.md`)

- **Color Palette:** Strictly follow the semantic color tokens:
  - Base canvas: `#f9fbf2`
  - Card surfaces & nav: `#eff2e5`
  - Primary text / headings / dark button: `#130e30` (Deep Ink)
  - Primary CTA button: `#ffe228` (Hi-Yellow, max 1 per viewport)
  - Body / helper text: `#5f5c6e` (Slate)
  - Logo / fine strokes: `#000000` (Onyx)
  - Background blobs only: `#59e25d` (Moss Green), `#e261e5` (Fuchsia) — **NEVER** use in UI controls or badges.
- **Typography:**
  - `Hedvig Letters Serif` exclusively for headings and display titles ($\ge 22\text{px}$).
  - `Inter` for all UI controls, body text, buttons, and captions ($< 22\text{px}$).
- **Geometry:**
  - Signature `1440px` (or `rounded-full`) pill radius for **all** buttons, inputs, tags, badges, and nav containers.
  - `24px` (`rounded-[24px]`) radius for cards.
  - Zero sharp corners ($< 16\text{px}$) on interactive controls.
- **Elevation:** Zero drop shadows. Cards rely on surface contrast (`#eff2e5` on `#f9fbf2`).

### Component Decomposition & Reusability

- Decompose UI into clean, reusable layers:
  - `src/components/ui/` — Atomic primitives (`Button`, `Input`, `Card`, `Badge`, `Modal`, `BlobBackdrop`, `LanguageSwitcher`, etc.)
  - `src/components/layout/` — Shell structures (`Navbar`, `Sidebar`, `PageContainer`, `SectionHeader`)
  - `src/components/features/` — Domain-specific reusable modules (`AgentCard`, `ChatBubble`, `ToolCallCard`, etc.)
  - `src/pages/` — Composed route views delegating to feature & UI components.
- Co-locate subcomponents when they are only used within a single parent feature.
- Extract common logic into custom hooks under `src/hooks/`.

### Mandatory Internationalization (i18n) — Zero Hardcoded Text

- **NO HARDCODED STRINGS:** Every user-facing text, label, placeholder, aria-label, and error message must be retrieved via `react-i18next` (`useTranslation()` or `<Trans>`).
- Maintain locale files under `src/locales/{lang}/{namespace}.json` (`en/`, `vi/`, etc.).
- Group keys hierarchically within namespaces (`common`, `nav`, `setup`, `chat`, `agents`, `tools`, `settings`).

### Component Structure

```tsx
// ✅ Good: named export, functional component, typed props, i18n
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';

interface AgentCardProps {
  agent: Agent;
  onSelect: (id: string) => void;
  isActive?: boolean;
}

export function AgentCard({ agent, onSelect, isActive = false }: AgentCardProps) {
  const { t } = useTranslation('agents');

  return (
    <Card hoverable onClick={() => onSelect(agent.agent_id)}>
      <h3 className="font-serif text-heading-sm text-deep-ink">{agent.name}</h3>
      <p className="font-sans text-body-sm text-slate">{agent.description}</p>
      <Button variant="secondary" size="sm" onClick={() => onSelect(agent.agent_id)}>
        {t('actions.select')}
      </Button>
    </Card>
  );
}

// ❌ Bad: default export, hardcoded text, untyped props, sharp corners
export default function AgentCard(props: any) {
  return (
    <div className="rounded border shadow p-4">
      <h3>{props.agent.name}</h3>
      <button className="rounded bg-blue-500 text-white">Select Agent</button>
    </div>
  );
}
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

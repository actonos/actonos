# ActonOS Coding Standards

> Enforced coding standards for all ActonOS contributions.

---

## 1. Go Standards

### Naming Conventions
- **Packages**: short, lowercase, single word (e.g., `agent`, `memory`, `bus`, `channels`, `tools`)
- **Interfaces**: verb-based or capability-based (e.g., `LLMProvider`, `ChannelAdapter`, `Executor`, `HAL`)
- **Structs**: noun-based (e.g., `AgentManager`, `TokenRefresher`, `VaultStore`, `CronScheduler`)
- **Functions**: verb-based (e.g., `CreateAgent`, `RefreshToken`, `DecryptKey`, `ScheduleTask`)
- **Constants**: `CamelCase` for exported, `camelCase` for unexported
- **Error variables**: `Err` prefix (e.g., `ErrAgentNotFound`, `ErrTokenExpired`)

### Error Handling
```go
// ✅ Good: wrap errors with context
if err := db.Save(agent); err != nil {
    return fmt.Errorf("saving agent %s: %w", agent.ID, err)
}

// ❌ Bad: raw error propagation or capitalized messages
if err := db.Save(agent); err != nil {
    return err // Missing context
}
return fmt.Errorf("Failed to save: %w", err) // Capitalized
```

### Logging
Use `log/slog` for structured logging:
```go
// ✅ Good
slog.Info("agent started", "agent_id", agent.AgentID, "model", agent.ModelConfig.PrimaryModel)
slog.Error("tool execution failed", "agent_id", agent.AgentID, "tool", toolName, "error", err)

// ❌ Bad: fmt.Printf or standard log.Printf
fmt.Println("Agent started: " + agent.AgentID)
```

### Context Usage
Always pass `context.Context` as the first argument to functions performing I/O, database access, or blocking operations:
```go
func (m *AgentManager) CreateAgent(ctx context.Context, manifest AgentManifest) (*Agent, error)
```

---

## 2. TypeScript / React Standards

### Design System Compliance (`docs/DESIGN.md`)
- **Color Tokens**:
  - Base canvas: `#f9fbf2` (Canvas)
  - Surfaces / cards / sidebar: `#eff2e5` (Soft Meadow)
  - Primary text / headings / dark pills: `#130e30` (Deep Ink)
  - Primary CTA button: `#ffe228` (Hi-Yellow, max 1 per viewport)
  - Secondary text / muted icons: `#5f5c6e` (Slate)
  - Logo / high-contrast borders: `#000000` (Onyx)
  - Decorative backdrop blobs only: `#59e25d` (Moss Green), `#e261e5` (Fuchsia) — **NEVER** in UI controls.
- **Geometry**:
  - `rounded-full` (1440px pill) on all buttons, inputs, badges, and language switchers.
  - `rounded-[24px]` for cards.
  - Zero sharp corners on interactive elements.
- **Scrollbars**:
  - Use custom 6px slim minimalist pill scrollbars defined in `index.css`.
- **Elevation**:
  - Zero drop shadows. Surface separation relies strictly on color contrast (`#eff2e5` on `#f9fbf2`).

### Mandatory Internationalization (i18n)
- **Zero hardcoded text**: Every string in UI must use `useTranslation()` or `<Trans>`.
- Maintain keys across all **14 namespaces** in `web/src/locales/` (`en/` and `vi/`):
  `common`, `nav`, `setup`, `chat`, `agents`, `tools`, `skills`, `automations`, `channels`, `connectors`, `dashboard`, `integrations`, `workspace`, `settings`.

### Component Structure & Exports
```tsx
// ✅ Good: Named export, typed props, i18n
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import type { AgentManifest } from '@/lib/types';

interface AgentRowProps {
  agent: AgentManifest;
  onEdit: (id: string) => void;
}

export function AgentRow({ agent, onEdit }: AgentRowProps) {
  const { t } = useTranslation('agents');
  return (
    <tr className="hover:bg-soft-meadow transition-colors">
      <td className="font-medium text-deep-ink">{agent.name}</td>
      <td>
        <Button variant="ghost" size="sm" onClick={() => onEdit(agent.agent_id)}>
          {t('actions.edit')}
        </Button>
      </td>
    </tr>
  );
}

// ❌ Bad: Default export, untyped props, hardcoded text
export default function AgentRow(props: any) {
  return <tr><td>{props.agent.name}</td><button>Edit</button></tr>;
}
```

---

## 3. Mandatory Pre-Flight Verification

Before concluding any development step, verify according to `.agents/rules/verification-checklist.md`:
1. `go vet ./...` passes without errors.
2. `cd web && npx tsc --noEmit` passes with zero type errors.
3. Every new route or endpoint is recorded in `docs/API.md` and `.agents/rules/source-registry.md`.

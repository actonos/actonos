---
name: actonos-frontend-dev
description: "Skill for developing the ActonOS React 19 + Tailwind v4 frontend adhering to the official DESIGN.md specification. Covers design tokens, component decomposition, zero-hardcoded-text i18n, and go:embed delivery."
---

# ActonOS Frontend Development Skill

Use this skill when developing the Web UI in the `web/` directory. All frontend code must strictly follow the **ActonOS Design System** (`docs/DESIGN.md`), implement **clean reusable component decomposition**, and enforce **mandatory internationalization (i18n)** with zero hardcoded text.

---

## 1. Tech Stack & Foundations

| Technology | Version | Role & Configuration |
|:---|:---|:---|
| **React** | 19 | Functional components with hooks, strict mode, named exports |
| **Tailwind CSS** | v4 | Modern `@theme` CSS configuration with custom design tokens in `index.css` |
| **Vite** | Latest | Fast build tool, dev server, HMR, proxy to `:8080` |
| **TypeScript** | 5.x | Strict mode, zero `any`, exhaustive typing for props & i18n |
| **i18next / react-i18next** | Latest | Mandatory localization for all UI strings across 16 namespaces |
| **xterm.js** | Latest | Read-only observability terminal; never bypass tool authorization with an interactive shell |
| **Lucide React** | Latest | Minimal line-style iconography (monochrome `#130e30` or `#5f5c6e`) |

---

## 2. Design System & Visual Language (`docs/DESIGN.md`)

ActonOS uses a **sunlit wildflower compliance atelier** aesthetic:
- **Canvas & Surfaces:** Warm cream canvas (`#f9fbf2`) with Soft Meadow (`#eff2e5`) card surfaces (slight organic green warmth).
- **Ink & Contrast:** Deep navy-violet (`#130e30`) carries all text, headings, and borders; pure black (`#000000`) for logo mark and fine stroke details; Slate (`#5f5c6e`) for body/muted copy.
- **Primary Action (Highlighter):** Vivid Hi-Yellow (`#ffe228`) with Deep Ink text. One per viewport max, paired with a dark pill (`#130e30`).
- **Decorative Atmosphere:** Organic blob shapes in Moss Green (`#59e25d`), Fuchsia (`#e261e5`), Yellow (`#ffe228`), and Deep Ink (`#130e30`) blooming behind hero/product cards (`BlobBackdrop.tsx`).
- **Geometry:** Signature **1440px (full pill)** radius on all buttons, inputs, tags, badges, nav capsules, and icon containers. **24px** radius on cards (`rounded-[24px]`).
- **Elevation:** Zero drop shadows. Surface separation is achieved purely via contrast between Canvas (`#f9fbf2`) and Soft Meadow (`#eff2e5`).
- **Scrollbars:** Ultra-clean 6px slim minimalist pill thumb scrollbars.

### Color Tokens

| Name | Hex | CSS Token | Usage Role |
|:---|:---|:---|:---|
| **Deep Ink** | `#130e30` | `--color-deep-ink` | Headings, primary text, card borders, dark button fills |
| **Hi-Yellow** | `#ffe228` | `--color-hi-yellow` | Primary CTA button fill, active pagination pill, highlight accents |
| **Moss Green** | `#59e25d` | `--color-moss-green` | **Decorative backdrop blobs only** (never in UI controls/badges) |
| **Fuchsia** | `#e261e5` | `--color-fuchsia` | **Decorative backdrop blobs only** (never in UI controls/badges) |
| **Slate** | `#5f5c6e` | `--color-slate` | Secondary body text, placeholder text, muted icons |
| **Canvas** | `#f9fbf2` | `--color-canvas` | Base page background, lightest surface (warm near-white) |
| **Soft Meadow** | `#eff2e5` | `--color-soft-meadow` | Card surfaces, nav bar background, sidebar background |
| **Charcoal** | `#222222` | `--color-charcoal` | Secondary dark button text/borders, nav dividers |
| **Onyx** | `#000000` | `--color-onyx` | Logo mark, input borders, highest-contrast fine details |

### Typography Tokens

- **Hedvig Letters Serif** (`--font-serif`): Exclusively for headings and display titles ($\ge 22\text{px}$).
- **Inter** (`--font-sans`): All functional UI text, navigation, buttons, inputs, tables, and body copy ($< 22\text{px}$).

---

## 3. Mandatory Internationalization (i18n) — Zero Hardcoded Text

## 3a. Current application architecture

- `DensityProvider` owns the comfortable/compact preference and applies `data-density` to `<html>`.
- `PageHeader`, `EmptyState`, `IconButton`, and `SegmentedControl` are the required primitives for new route work.
- Sidebar navigation is grouped by workflow: Overview, Build, Connections, Capabilities, and System.
- Agent Studio detail routes use `#/agents/new` and `#/agents/:id`; preserve these nested routes when adding links.
- The global command palette is opened with `Ctrl/Cmd+K`, supports navigation/entity search, and must never execute sensitive mutations without the existing approval flow.
- Authenticated Playwright coverage and axe checks run at 390×844, 768×1024, and 1440×900. Update visual snapshots only when the UI change is intentional.
- Product UI is emoji-free and must pass `npm run check:emoji`; all visible copy belongs in locale namespaces and must pass the hardcoded-text audit.
- Reusable operational primitives include `MetricCard`, `AsyncState`, and `FreshnessBadge`; use them for health, loading/error, and realtime freshness states instead of page-local variants.
- Important tabs and filters should be represented in hash query state through `web/src/lib/url-state.ts` so refresh and browser navigation preserve context.
- Hash route parsing must strip the query before resolving the primary route. A deep link such as `#/operations?view=runtime` must not fall back to Dashboard.
- Approval interruption dialogs must autofocus feedback, trap keyboard focus, require rejection feedback, and expose risk/action context before allowing a decision.
- Chat route composition uses feature components such as `ChatHeader`, `ChatSessionRail`, `ChatEmptyState`, `ChatComposer`, `MessageTimeline`, `MessageBubble`, and `TraceDisclosure`; keep message, trace, session, and composer concerns separated when extending chat behavior.
- Agent Studio section navigation lives in `AgentStudioNav`; expose section readiness/counts there and keep editor sections focused on their own form state.
- Agent Studio finishes with a localized Review & save section. Structural validation blocks save and summarizes identity, models, tools, channels, approval, and budget.
- Chat sessions use one responsive rail: persistent on desktop and an explicit dismissible drawer on narrow layouts.
- Operations uses `overview`, `feed`, `runtime`, and `cost` views, with non-default views persisted in the hash query.
- Agent identity fields and lifecycle controls live in `AgentIdentitySection`; do not duplicate editable identity controls in the route component.
- Budget, approval threshold, and workspace scope controls live in `AgentGovernanceSection`; keep governance warnings and validation close to these fields.
- Tool authorization selection lives in `AgentToolsSection`; use pressed-state buttons, localized status labels, and an explicit empty state.
- Channel listener configuration lives in `AgentChannelsSection`; channel names/descriptions must come from i18n and selection controls use pressed-state semantics.

### Strict Rule
**Every user-facing string in JSX/TSX must be loaded via `useTranslation()` or `<Trans>` components.**
Hardcoded strings in components violate build verification.

### Active 16 Locale Namespaces (`web/src/locales/{en,vi}/`)

| Namespace | Usage |
|:---|:---|
| `common.json` | Buttons, badges, modal actions, validation messages, generic labels |
| `nav.json` | Sidebar navigation tabs, header titles, breadcrumbs |
| `missions.json` | Mission control, autonomous backlog, task modal, standing directives |
| `setup.json` | Setup wizard, initial admin identity, password setup |
| `chat.json` | Chat interface, streaming thoughts, tool invocation outputs |
| `agents.json` | Agent management, agent table, manifest editor, memory inspector |
| `tools.json` | Tool Hub, MCP servers, tool status |
| `skills.json` | Skills registry, skill cards, marketplace actions |
| `plugins.json` | Sandboxed WASM plugins hub, upload, detail modal, logs, configuration & secrets |
| `automations.json` | Cron scheduler, automated periodic tasks, heartbeat |
| `dashboard.json` | System metrics, quick actions, agent status cards |
| `workspace.json` | File manager, file preview, workspace browser |
| `audit.json` | Audit logs ledger, filters, cryptographic hash verification, detail modal |
| `settings.json` | System configuration, token ledger, backup snapshots, OTA updates, Tailscale |
| `notifications.json` | Notification center, browser push, history page |
| `operations.json` | Live telemetry, Docker, execution feed, canvas, terminal, queue, approvals, costs |

### Usage in Components

```tsx
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Plus } from 'lucide-react';

export function AgentsHeader({ onNewAgent }: { onNewAgent: () => void }) {
  const { t } = useTranslation('agents');

  return (
    <div className="flex items-center justify-between">
      <div>
        <h1 className="font-serif text-heading text-deep-ink">{t('title')}</h1>
        <p className="font-sans text-body-sm text-slate">{t('subtitle')}</p>
      </div>
      <Button variant="primary" icon={<Plus className="w-4 h-4" />} onClick={onNewAgent}>
        {t('actions.createAgent')}
      </Button>
    </div>
  );
}
```

---

## 4. Actual Component Architecture (`web/src/`)

```
web/src/
├── components/
│   ├── ui/                         # Atomic Primitives (Design System)
│   │   ├── Button.tsx              # Primary yellow pill, dark pill, ghost pill
│   │   ├── Input.tsx               # Pill capsule input with label/error
│   │   ├── Card.tsx                # Soft Meadow 24px surface card
│   │   ├── Badge.tsx               # Pill status badges
│   │   ├── Modal.tsx               # Accessible dialog container
│   │   ├── ConfirmModal.tsx        # Standard confirmation dialog
│   │   ├── PromptModal.tsx         # User input modal prompt
│   │   ├── Toast.tsx               # ToastProvider & useToast notifications
│   │   ├── BlobBackdrop.tsx        # Organic SVG decorative backdrop shapes
│   │   ├── LanguageSwitcher.tsx    # Compact / standard language switcher trigger
│   │   └── LanguageSelectModal.tsx # Sidebar-scoped & standalone language modal
│   │
│   ├── layout/                     # Structural Layouts
│   │   ├── Sidebar.tsx             # Collapsible left navigation bar with compact mode
│   │   ├── Header.tsx              # Sticky top bar with breadcrumbs and user actions
│   │   ├── Navbar.tsx              # Standalone navigation bar
│   │   └── PageContainer.tsx       # Standard constrained page wrapper
│   │
│   ├── chat/                       # Chat Feature Components
│   │   └── MarkdownContent.tsx     # Rich markdown renderer for thoughts and responses
│   │
│   └── features/                   # Domain Specific Modules
│       ├── agents/
│       │   ├── AgentCard.tsx       # Summary card for an agent
│       │   ├── AgentFormModal.tsx  # Create/edit manifest modal
│       │   ├── CronJobModal.tsx    # Automation cron configuration modal
│       │   └── SoulEditorModal.tsx # SOUL.md system instruction modal
│       ├── onboarding/             # Setup wizard subcomponents
│       └── tools/                  # Tool detail cards
│
├── pages/                          # Composed Route Views (NavTabs)
│   ├── Dashboard/DashboardPage.tsx       # 'dashboard'
│   ├── Agents/AgentsPage.tsx             # 'agents' (Rich table)
│   ├── Agents/AgentStudioPage.tsx        # 'agent-studio' (Config, Soul, Memory.md)
│   ├── Chat/ChatPage.tsx                 # 'chat'
│   ├── Automations/AutomationsPage.tsx   # 'automations'
│   ├── Missions/MissionsPage.tsx         # 'missions'
│   ├── Operations/OperationsPage.tsx     # 'operations'
│   ├── Plugins/
│   │   ├── PluginsPage.tsx               # 'plugins' (WASM Plugin Hub)
│   │   ├── PluginDetailModal.tsx         # Plugin manifest & config modal
│   │   ├── PluginUploadModal.tsx         # .actonpkg bundle uploader
│   │   └── PluginLogsModal.tsx           # Realtime sandbox log viewer
│   ├── ToolHub/ToolHubPage.tsx           # 'tools'
│   ├── Skills/SkillsPage.tsx             # 'skills'
│   ├── Workspace/WorkspacePage.tsx       # 'workspace'
│   ├── Terminal/TerminalPage.tsx         # 'terminal'
│   ├── Notifications/NotificationsPage.tsx # 'notifications'
│   ├── AuditLogs/AuditLogsPage.tsx       # 'audit-logs'
│   ├── Settings/SettingsPage.tsx         # 'settings'
│   └── Auth/
│       ├── SetupWizardPage.tsx           # First-run onboarding
│       └── LoginPage.tsx                 # Password authentication
│
├── lib/
│   ├── api.ts                      # Full type-safe REST API client
│   ├── i18n.ts                     # i18next configuration & resource loader
│   ├── models.ts                   # Up-to-date LLM model catalog
│   └── types.ts                    # TypeScript interface contracts
│
├── App.tsx                         # State root, auth gating, sidebar & page router
├── index.css                       # Tailwind v4 @theme, custom scrollbars, typography
└── main.tsx                        # React 19 application mount point
```

---

## 5. Verification & Common Pitfalls

### Common Pitfalls to Avoid:
1. **Never use generic card layouts for Agent list**: The agents page uses a rich, responsive table with search and filtering.
2. **Never hardcode language switcher as a toggle**: Always open `LanguageSelectModal` or sidebar overlay.
3. **Never forget the second locale**: Any translation key added to `en/xyz.json` MUST exist in `vi/xyz.json`.
4. **Never introduce Sharp Corners**: Interactive elements (buttons, inputs, badges) MUST use `rounded-full`. Cards use `rounded-[24px]`.
5. **Never import non-existent files**: Consult `.agents/rules/source-registry.md` before importing or referencing components.

### Verification Checklist
```bash
# Complete frontend quality gate
cd web
npm run quality
npx tsc --noEmit

# Browser smoke tests require a running backend and installed browser
npm run test:e2e
```

The quality gate must include `audit:i18n -- --fail`; a report-only hardcoded
text scan is not sufficient. Playwright includes `@axe-core/playwright` and
must fail on serious or critical accessibility violations.
`check:i18n` also rejects mojibake, Unicode replacement characters, repeated
question marks, and question marks embedded inside Vietnamese words.
ActonOS UI text is emoji-free. Use Lucide icons for visual meaning and
`check:emoji` to reject emoji, regional flags, or their common mojibake forms.

Keep the REST facade small by placing shared fetch/session behavior in
`lib/api/client.ts` and domain clients in `lib/api/`. Reusable feature panels
belong in their feature module rather than being duplicated by pages. Chat
message contracts and formatting helpers live outside the route component so
the page remains focused on orchestration.

### Governance UI

Mission Control owns the human approval queue and durable run ledger. Approval
cards must preview exact arguments, show risk and agent identity, and expose
explicit approve/reject actions. All labels belong to the `missions` namespace
in both English and Vietnamese.

### Live Operations UI

- Mount exactly one `RealtimeProvider` at the authenticated application root.
  Header, Operations, approvals, and cost widgets consume its shared snapshot;
  feature components must not open duplicate realtime sockets.
- Consume `/api/realtime` with the HttpOnly session cookie and reconnect with
  exponential bounded backoff plus jitter. Invalid frames must close the socket.
- Keep xterm.js read-only. Shell execution must continue through the authorized Tool Registry and sandbox.
- Render run events as collapsible Thought/Action/Observation cards and fetch ordered detail from `/api/runs/{id}/events`.
- Live Canvas embeds only a sanitized `SystemMetrics.canvas_url`, uses a sandboxed
  iframe without clipboard permissions, and shows a waiting state when unavailable.
- Never invent telemetry fallback values when sensors or Docker access are unavailable.

### Browser Authentication and Mutations

- Browser authentication is cookie-only. Never persist bearer tokens in
  `localStorage`, session storage, URLs, or WebSocket query strings.
- Every mutation that can return `202 Accepted` must use `MutationResult<T>` and
  check `isApprovalRequired()` before showing success or changing optimistic state.
- `fetchJSON` emits `actonos:approval-required`; the global
  `ApprovalInterruption` renders the exact action and arguments immediately.

### Routing and Bundle Discipline

- Primary pages use hash routes (`#/dashboard`, `#/operations`, and so on) so
  embedded deployments support reload, deep links, and browser history.
- All primary pages remain lazy-loaded. Heavy editor/xterm dependencies must not
  return to the entry chunk.
- `npm run check:bundle` enforces the current entry-JavaScript budget.
- Supported language choices must match complete locale resources. Do not expose
  placeholder languages or add user-visible hardcoded strings.
- Production TypeScript has zero explicit `any`, and unused variables are lint
  errors. Technical identifiers may remain untranslated only when the strict
  audit explicitly recognizes them as immutable protocol values.

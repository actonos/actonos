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
| **i18next / react-i18next** | Latest | Mandatory localization for all UI strings across 14 namespaces |
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

### Strict Rule
**Every user-facing string in JSX/TSX must be loaded via `useTranslation()` or `<Trans>` components.**
Hardcoded strings in components violate build verification.

### Active 15 Locale Namespaces (`web/src/locales/{en,vi}/`)

| Namespace | Usage |
|:---|:---|
| `common.json` | Buttons, badges, modal actions, validation messages, generic labels |
| `nav.json` | Sidebar navigation tabs, header titles, breadcrumbs |
| `missions.json` | Mission control, autonomous backlog, task modal, standing directives |
| `setup.json` | Setup wizard, initial admin identity, password setup |
| `chat.json` | Chat interface, streaming thoughts, tool invocation outputs |
| `agents.json` | Agent management, agent table, manifest editor, memory inspector |
| `tools.json` | Tool Hub, MCP servers, WASM plugins, tool status |
| `skills.json` | Skills registry, skill cards, marketplace actions |
| `automations.json` | Cron scheduler, automated periodic tasks, heartbeat |
| `channels.json` | Messaging channels (Telegram, WhatsApp, Discord), account credentials |
| `connectors.json` | SaaS integrations & OAuth connectors (Google, Notion, GitHub) |
| `dashboard.json` | System metrics, quick actions, agent status cards |
| `integrations.json` | Integration settings, API keys, pairing verification |
| `workspace.json` | File manager, file preview, workspace browser |
| `settings.json` | System configuration, token ledger, backup snapshots, OTA updates, Tailscale |

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
│   ├── Channels/ChannelsPage.tsx         # 'channels'
│   ├── Connectors/ConnectorsPage.tsx     # 'connectors'
│   ├── ToolHub/ToolHubPage.tsx           # 'tools'
│   ├── Skills/SkillsPage.tsx             # 'skills'
│   ├── Workspace/WorkspacePage.tsx       # 'workspace'
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
# 1. Type Check
cd web && npx tsc --noEmit

# 2. Build Check
cd web && npm run build
```

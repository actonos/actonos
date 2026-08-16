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
| **Tailwind CSS** | v4 | Modern `@theme` CSS configuration with custom design tokens |
| **Vite** | Latest | Fast build tool, dev server, HMR, proxy to `:8080` |
| **TypeScript** | 5.x | Strict mode, zero `any`, exhaustive typing for props & i18n |
| **i18next / react-i18next** | Latest | Mandatory localization for all UI strings across namespaces |
| **Lucide React** | Latest | Minimal line-style iconography (monochrome `#130e30` or `#5f5c6e`) |

---

## 2. Design System & Visual Language (`DESIGN.md`)

ActonOS uses a **sunlit wildflower compliance atelier** aesthetic:
- **Canvas & Surfaces:** Warm cream canvas (`#f9fbf2`) with Soft Meadow (`#eff2e5`) card surfaces (slight organic green warmth).
- **Ink & Contrast:** Deep navy-violet (`#130e30`) carries all text, headings, and borders; pure black (`#000000`) for logo mark and fine stroke details; Slate (`#5f5c6e`) for body/muted copy.
- **Primary Action (Highlighter):** Vivid Hi-Yellow (`#ffe228`) with Deep Ink text. One per viewport max, paired with a dark pill (`#130e30`).
- **Decorative Atmosphere:** Organic blob shapes in Moss Green (`#59e25d`), Fuchsia (`#e261e5`), Yellow (`#ffe228`), and Deep Ink (`#130e30`) blooming behind hero/product cards.
- **Geometry:** Signature **1440px (full pill)** radius on all buttons, inputs, tags, badges, nav capsules, and icon containers. **24px** radius on cards.
- **Elevation:** Zero drop shadows. Surface separation is achieved purely via contrast between Canvas (`#f9fbf2`) and Soft Meadow (`#eff2e5`).

### Color Tokens

| Name | Hex | CSS Token | Usage Role |
|:---|:---|:---|:---|
| **Deep Ink** | `#130e30` | `--color-deep-ink` | Headings, primary text, card borders, dark button fills |
| **Hi-Yellow** | `#ffe228` | `--color-hi-yellow` | Primary CTA button fill, active pagination pill, highlight accents |
| **Moss Green** | `#59e25d` | `--color-moss-green` | **Decorative backdrop blobs only** (never in UI controls/badges) |
| **Fuchsia** | `#e261e5` | `--color-fuchsia` | **Decorative backdrop blobs only** (never in UI controls/badges) |
| **Slate** | `#5f5c6e` | `--color-slate` | Secondary body text, placeholder text, muted icons, case study links |
| **Canvas** | `#f9fbf2` | `--color-canvas` | Base page background, lightest surface (warm near-white) |
| **Soft Meadow** | `#eff2e5` | `--color-soft-meadow` | Card surfaces, nav bar background, hero backdrop panels |
| **Charcoal** | `#222222` | `--color-charcoal` | Secondary dark button text/borders, nav dividers |
| **Onyx** | `#000000` | `--color-onyx` | Logo mark, input borders, highest-contrast fine details |

### Typography Tokens

- **Hedvig Letters Serif** (`--font-hedvig-letters-serif`): Exclusively for headings and display titles ($\ge 22\text{px}$). Weights: `400`, `700`. Letter-spacing: `-0.01em` to `-0.02em`. Line-height: `1.0–1.25`.
- **Inter** (`--font-inter`): All functional UI text, navigation, buttons, inputs, and body copy ($< 22\text{px}$). Weights: `400`, `500`, `600`. OpenType: `"ss01" on, "cv11" on`.

| Role | Font Family | Size | Leading | Tracking | Token Class |
|:---|:---|:---|:---|:---|:---|
| **Display** | Hedvig Letters Serif | 64px | 1.0 | -0.64px | `text-display font-serif` |
| **Heading LG** | Hedvig Letters Serif | 48px | 1.1 | -0.48px | `text-heading-lg font-serif` |
| **Heading** | Hedvig Letters Serif | 32px | 1.15 | -0.32px | `text-heading font-serif` |
| **Heading SM** | Hedvig Letters Serif | 22px | 1.25 | -0.22px | `text-heading-sm font-serif` |
| **Subheading** | Inter | 18px | 1.5 | -0.18px | `text-subheading font-sans` |
| **Body** | Inter | 16px | 1.5 | -0.16px | `text-body font-sans` |
| **Body SM** | Inter | 14px | 1.5 | -0.14px | `text-body-sm font-sans` |
| **Caption / Small Caps** | Inter (500) | 10px | 1.2 | -0.2px | `text-caption uppercase tracking-wider font-sans` |

### Strict Design Do's & Don'ts

```
✅ DO:
- Use 1440px / rounded-full pill radius on every button, input, tag, and nav item.
- Pair filled Hi-Yellow (#ffe228) CTA with Dark Pill (#130e30) for button hierarchy.
- Use Hedvig Letters Serif for headings >= 22px and Inter for all body/UI text.
- Use Soft Meadow (#eff2e5) for card backgrounds on Canvas (#f9fbf2).
- Use decorative organic blobs (#59e25d, #e261e5, #ffe228, #130e30) strictly as backdrop atmosphere.

❌ DON'T:
- NEVER use sharp corners (<16px) on interactive controls or inputs.
- NEVER use Moss Green (#59e25d) or Fuchsia (#e261e5) on buttons, badges, or icons.
- NEVER add drop shadows to cards or buttons (surface color contrast defines boundaries).
- NEVER place two yellow primary buttons in the same viewport.
- NEVER hardcode user-facing strings (always use i18n).
```

---

## 3. Mandatory Internationalization (i18n) — Zero Hardcoded Text

### Strict Rule
**Every user-facing string in JSX/TSX must be loaded via `useTranslation()` or `<Trans>` components.**
Hardcoded strings in components are considered a build and lint violation.

### Locale Structure (`web/src/locales/`)

```
web/src/locales/
├── en/
│   ├── common.json         # Buttons, badges, validation, generic labels
│   ├── nav.json            # Navigation items, footer links, language labels
│   ├── setup.json          # Setup wizard, Wi-Fi, vault keys, admin PIN
│   ├── chat.json           # Chat input, stream states, tool execution logs
│   ├── agents.json         # Agent cards, create/edit modal, swarm settings
│   ├── tools.json          # MCP servers, WASM plugins, skills explorer
│   ├── workspace.json      # File manager, code preview, terminal output
│   ├── integrations.json   # OAuth connectors (Google, Notion, GitHub, etc.)
│   └── settings.json       # System metrics, Tailscale status, OTA updates
└── vi/
    ├── common.json
    ├── nav.json
    └── ... (mirrored namespace structure)
```

### i18n Configuration (`web/src/lib/i18n.ts`)

```ts
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

// English translations
import enCommon from '../locales/en/common.json';
import enNav from '../locales/en/nav.json';
import enSetup from '../locales/en/setup.json';
import enChat from '../locales/en/chat.json';
import enAgents from '../locales/en/agents.json';
import enTools from '../locales/en/tools.json';
import enSettings from '../locales/en/settings.json';

// Vietnamese translations
import viCommon from '../locales/vi/common.json';
import viNav from '../locales/vi/nav.json';
import viSetup from '../locales/vi/setup.json';
import viChat from '../locales/vi/chat.json';
import viAgents from '../locales/vi/agents.json';
import viTools from '../locales/vi/tools.json';
import viSettings from '../locales/vi/settings.json';

export const defaultNS = 'common';
export const resources = {
  en: {
    common: enCommon,
    nav: enNav,
    setup: enSetup,
    chat: enChat,
    agents: enAgents,
    tools: enTools,
    settings: enSettings,
  },
  vi: {
    common: viCommon,
    nav: viNav,
    setup: viSetup,
    chat: viChat,
    agents: viAgents,
    tools: viTools,
    settings: viSettings,
  },
} as const;

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    defaultNS,
    ns: ['common', 'nav', 'setup', 'chat', 'agents', 'tools', 'settings'],
    resources,
    interpolation: {
      escapeValue: false, // React already escapes values
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
  });

export default i18n;
```

### Usage in Components

```tsx
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';

export function AgentHeader() {
  const { t } = useTranslation('agents');

  return (
    <div className="flex items-center justify-between">
      <div>
        <span className="text-caption uppercase tracking-wider text-slate">
          {t('eyebrow')}
        </span>
        <h1 className="font-serif text-heading-lg text-deep-ink">
          {t('title')}
        </h1>
        <p className="font-sans text-body text-slate mt-2">
          {t('subtitle')}
        </p>
      </div>
      <Button variant="primary">
        {t('actions.createNew')}
      </Button>
    </div>
  );
}
```

---

## 4. Reusable Component Architecture

The frontend is strictly structured into atomic, reusable layers to eliminate code duplication and maintain visual consistency:

```
web/src/
├── components/
│   ├── ui/                         # Atomic Primitives (Design System)
│   │   ├── Button.tsx              # Primary yellow pill, dark pill, ghost pill
│   │   ├── Input.tsx               # Pill email / text input capsule
│   │   ├── Card.tsx                # Soft Meadow 24px surface card
│   │   ├── Badge.tsx               # Pill status badges (Inter 500)
│   │   ├── Modal.tsx               # Accessible dialog container
│   │   ├── BlobBackdrop.tsx        # Organic SVG decorative background shapes
│   │   ├── Dropdown.tsx            # Pill dropdown selector
│   │   ├── PaginationDot.tsx       # Carousel position indicators
│   │   ├── SmallCapsLabel.tsx      # Uppercase taxonomic section labels
│   │   └── LanguageSwitcher.tsx    # Globe icon + language selector
│   │
│   ├── layout/                     # Structural Layouts
│   │   ├── Navbar.tsx              # Top bar with logo, links, lang, CTA pair
│   │   ├── Sidebar.tsx             # Collapsible appliance side menu
│   │   ├── PageContainer.tsx       # 1200px max-width centered container
│   │   └── SectionHeader.tsx       # Serif headline + small caps eyebrow
│   │
│   └── features/                   # Domain-Specific Reusable Modules
│       ├── agents/
│       │   ├── AgentCard.tsx       # Card displaying agent persona & status
│       │   ├── AgentGrid.tsx       # 3-column responsive card layout
│       │   └── AgentFormModal.tsx  # Create/edit manifest dialog
│       ├── chat/
│       │   ├── ChatMessageList.tsx # Message stream container
│       │   ├── ChatBubble.tsx      # User/Agent message bubbles
│       │   ├── ChatInputBar.tsx    # Capsule input with send button
│       │   └── ToolCallCard.tsx    # Collapsible tool execution card
│       ├── setup/
│       │   ├── WifiSelector.tsx    # Hotspot / network scan picker
│       │   └── VaultKeyForm.tsx    # API key input table
│       └── tools/
│           ├── ToolCard.tsx        # MCP / WASM / Skill card
│           └── ToolRegistryTable.tsx
│
├── pages/                          # Composed Route Views
│   ├── SetupWizard/SetupWizardPage.tsx
│   ├── Chat/ChatPage.tsx
│   ├── Agents/AgentsPage.tsx
│   ├── Workspace/WorkspacePage.tsx
│   ├── Integrations/IntegrationsPage.tsx
│   ├── ToolHub/ToolHubPage.tsx
│   └── Settings/SettingsPage.tsx
│
├── hooks/                          # Custom Hooks
│   ├── useApi.ts                   # Type-safe REST client
│   ├── useSSE.ts                   # Streaming chat SSE consumer
│   ├── useAuth.ts                  # PIN authentication context
│   └── useLanguage.ts              # i18n language toggle helper
│
├── lib/
│   ├── api.ts                      # Fetch wrapper with interceptors
│   ├── i18n.ts                     # i18next initialization
│   └── types.ts                    # Global TypeScript contracts
│
└── index.css                       # Tailwind v4 theme & font declarations
```

---

## 5. Reference Component Implementations

### A. Global CSS & Tailwind v4 Theme (`web/src/index.css`)

```css
@import "tailwindcss";

@theme {
  /* Colors */
  --color-deep-ink: #130e30;
  --color-hi-yellow: #ffe228;
  --color-moss-green: #59e25d;
  --color-fuchsia: #e261e5;
  --color-slate: #5f5c6e;
  --color-canvas: #f9fbf2;
  --color-soft-meadow: #eff2e5;
  --color-charcoal: #222222;
  --color-onyx: #000000;

  /* Typography */
  --font-serif: "Hedvig Letters Serif", ui-serif, Georgia, serif;
  --font-sans: "Inter", ui-sans-serif, system-ui, -apple-system, sans-serif;

  /* Type Scale */
  --text-caption: 10px;
  --text-body-sm: 14px;
  --text-body: 16px;
  --text-subheading: 18px;
  --text-heading-sm: 22px;
  --text-heading: 32px;
  --text-heading-lg: 48px;
  --text-display: 64px;

  /* Border Radii */
  --radius-card: 24px;
  --radius-pill: 1440px;
}

/* Base resets & typography */
body {
  background-color: var(--color-canvas);
  color: var(--color-deep-ink);
  font-family: var(--font-sans);
  font-feature-settings: "ss01" on, "cv11" on;
  -webkit-font-smoothing: antialiased;
}

h1, h2, h3, .font-serif {
  font-family: var(--font-serif);
  letter-spacing: -0.01em;
}
```

### B. Atomic Button (`web/src/components/ui/Button.tsx`)

```tsx
import React from 'react';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  icon?: React.ReactNode;
  children: React.ReactNode;
}

export function Button({
  variant = 'primary',
  size = 'md',
  icon,
  children,
  className = '',
  disabled,
  ...props
}: ButtonProps) {
  const baseStyles = 'inline-flex items-center justify-center font-sans font-medium transition-all duration-150 rounded-full select-none cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed';

  const sizeStyles = {
    sm: 'px-4 py-2 text-body-sm gap-1.5',
    md: 'px-6 py-3 text-body gap-2',
    lg: 'px-8 py-3.5 text-subheading gap-2.5',
  };

  const variantStyles = {
    // Primary: Highlighter yellow with Deep Ink text (max 1 per viewport)
    primary: 'bg-hi-yellow text-deep-ink hover:brightness-95 active:brightness-90',
    // Secondary: Deep Ink dark pill with white text
    secondary: 'bg-deep-ink text-white hover:bg-opacity-90 active:bg-opacity-95',
    // Ghost: Transparent with deep ink border
    ghost: 'bg-transparent text-deep-ink border border-deep-ink hover:bg-soft-meadow',
  };

  return (
    <button
      className={`${baseStyles} ${sizeStyles[size]} ${variantStyles[variant]} ${className}`}
      disabled={disabled}
      {...props}
    >
      {icon && <span className="inline-flex shrink-0">{icon}</span>}
      <span>{children}</span>
    </button>
  );
}
```

### C. Atomic Capsule Input (`web/src/components/ui/Input.tsx`)

```tsx
import React from 'react';

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  actionButton?: React.ReactNode;
}

export function Input({
  label,
  error,
  actionButton,
  className = '',
  id,
  ...props
}: InputProps) {
  return (
    <div className="w-full flex flex-col gap-1.5">
      {label && (
        <label htmlFor={id} className="text-caption uppercase tracking-wider text-slate font-medium">
          {label}
        </label>
      )}
      <div className="relative flex items-center">
        <input
          id={id}
          className={`w-full bg-white text-deep-ink placeholder-slate font-sans text-body px-5 py-3 rounded-full border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink transition-all ${
            actionButton ? 'pr-36' : ''
          } ${error ? 'border-red-500' : ''} ${className}`}
          {...props}
        />
        {actionButton && (
          <div className="absolute right-1.5">
            {actionButton}
          </div>
        )}
      </div>
      {error && <span className="text-body-sm text-red-600 px-3">{error}</span>}
    </div>
  );
}
```

### D. Reusable Surface Card (`web/src/components/ui/Card.tsx`)

```tsx
import React from 'react';

export interface CardProps {
  children: React.ReactNode;
  className?: string;
  onClick?: () => void;
  hoverable?: boolean;
}

export function Card({
  children,
  className = '',
  onClick,
  hoverable = false,
}: CardProps) {
  return (
    <div
      onClick={onClick}
      className={`bg-soft-meadow rounded-[24px] p-6 md:p-8 transition-all ${
        hoverable ? 'hover:scale-[1.01] cursor-pointer' : ''
      } ${className}`}
    >
      {children}
    </div>
  );
}
```

### E. Decorative Organic Blobs (`web/src/components/ui/BlobBackdrop.tsx`)

```tsx
export function BlobBackdrop() {
  return (
    <div className="absolute inset-0 overflow-hidden pointer-events-none -z-10" aria-hidden="true">
      <svg className="w-full h-full" viewBox="0 0 800 600" fill="none" xmlns="http://www.w3.org/2000/svg">
        {/* Moss Green Blob */}
        <path
          d="M120 180C220 120 320 160 380 260C440 360 380 460 280 480C180 500 80 440 40 340C0 240 20 240 120 180Z"
          fill="#59e25d"
          className="opacity-70 mix-blend-multiply"
        />
        {/* Fuchsia Blob */}
        <path
          d="M520 140C620 80 720 140 760 240C800 340 740 420 640 460C540 500 460 420 440 320C420 220 420 200 520 140Z"
          fill="#e261e5"
          className="opacity-60 mix-blend-multiply"
        />
        {/* Hi-Yellow Accent Blob */}
        <path
          d="M320 280C400 240 480 260 520 340C560 420 500 480 420 500C340 520 280 460 260 380C240 300 240 320 320 280Z"
          fill="#ffe228"
          className="opacity-80 mix-blend-multiply"
        />
        {/* Deep Ink Deepening Blob */}
        <path
          d="M600 320C680 280 740 320 760 380C780 440 740 500 660 520C580 540 540 480 540 420C540 360 520 360 600 320Z"
          fill="#130e30"
          className="opacity-20 mix-blend-multiply"
        />
      </svg>
    </div>
  );
}
```

### F. Language Switcher (`web/src/components/ui/LanguageSwitcher.tsx`)

```tsx
import { useTranslation } from 'react-i18next';
import { Globe } from 'lucide-react';

export function LanguageSwitcher() {
  const { i18n, t } = useTranslation('common');

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  return (
    <button
      onClick={toggleLanguage}
      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-soft-meadow text-deep-ink hover:bg-canvas transition-colors text-body-sm font-sans font-medium"
      title={t('language.toggle')}
    >
      <Globe className="w-4 h-4 text-slate" />
      <span className="uppercase">{i18n.language.slice(0, 2)}</span>
    </button>
  );
}
```

### G. Reusable Agent Card (`web/src/components/features/agents/AgentCard.tsx`)

```tsx
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Bot, ArrowRight } from 'lucide-react';
import type { AgentManifest } from '@/lib/types';

export interface AgentCardProps {
  agent: AgentManifest;
  onSelect: (id: string) => void;
}

export function AgentCard({ agent, onSelect }: AgentCardProps) {
  const { t } = useTranslation('agents');

  return (
    <Card hoverable onClick={() => onSelect(agent.agent_id)} className="flex flex-col justify-between h-full">
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="w-10 h-10 rounded-full bg-canvas flex items-center justify-center text-deep-ink border border-onyx">
            <Bot className="w-5 h-5" />
          </div>
          <span className="text-caption uppercase tracking-wider text-slate font-medium">
            {agent.model_config.primary_model}
          </span>
        </div>

        <h3 className="font-serif text-heading-sm text-deep-ink mb-2">
          {agent.name}
        </h3>

        <p className="font-sans text-body-sm text-slate line-clamp-3">
          {agent.description}
        </p>
      </div>

      <div className="mt-6 pt-4 border-t border-canvas flex items-center justify-between">
        <span className="text-caption uppercase text-slate">
          {t('card.toolsCount', { count: agent.authorized_tools?.length || 0 })}
        </span>
        <Button variant="ghost" size="sm" icon={<ArrowRight className="w-4 h-4" />}>
          {t('card.openChat')}
        </Button>
      </div>
    </Card>
  );
}
```

---

## 6. Frontend Testing & Verification

1. **Type Checking:** Ensure 100% strict TypeScript typing without `any`.
   ```bash
   cd web && npx tsc --noEmit
   ```
2. **Linting:** Validate React conventions and code quality.
   ```bash
   cd web && npx eslint src/
   ```
3. **i18n Completeness:** Verify all translation keys exist in both `en/` and `vi/` namespaces before submitting code.

---

## 7. Reference Files

- [docs/DESIGN.md](../../docs/DESIGN.md) — Comprehensive style reference and design tokens
- [docs/API.md](../../docs/API.md) — REST API reference for frontend integration
- [web/src/index.css](../../web/src/index.css) — Tailwind v4 theme definitions
- [web/src/lib/i18n.ts](../../web/src/lib/i18n.ts) — i18n configuration

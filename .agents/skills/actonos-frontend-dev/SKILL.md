---
name: actonos-frontend-dev
description: "Skill for developing the ActonOS React 19 + Tailwind v4 frontend. Covers page structure, component patterns, API integration, and go:embed delivery."
---

# ActonOS Frontend Development Skill

Use this skill when developing the Web UI in the `web/` directory.

## Tech Stack

| Technology | Version | Purpose |
|:---|:---|:---|
| React | 19 | UI framework |
| Tailwind CSS | v4 | Utility-first styling |
| Vite | Latest | Build tool and dev server |
| TypeScript | 5.x (strict) | Type safety |

## Directory Structure

```
web/
├── src/
│   ├── App.tsx                     # Root component with routing
│   ├── main.tsx                    # Entry point
│   ├── index.css                   # Global styles + Tailwind directives
│   ├── pages/
│   │   ├── SetupWizard/            # Onboarding flow (Wi-Fi, API keys, OAuth)
│   │   │   ├── SetupWizard.tsx
│   │   │   ├── WifiStep.tsx
│   │   │   ├── ApiKeysStep.tsx
│   │   │   └── OAuthStep.tsx
│   │   ├── Chat/                   # Streaming chat interface
│   │   │   ├── ChatPage.tsx
│   │   │   ├── MessageList.tsx
│   │   │   ├── MessageInput.tsx
│   │   │   └── ToolCallCard.tsx
│   │   ├── Agents/                 # Agent management
│   │   │   ├── AgentListPage.tsx
│   │   │   ├── AgentCard.tsx
│   │   │   └── AgentFormModal.tsx
│   │   ├── Workspace/              # Sandbox file manager
│   │   ├── Integrations/           # OAuth 1-click SaaS setup
│   │   ├── ToolHub/                # MCP, Skills, WASM management
│   │   └── Settings/               # API keys, Tailscale, metrics, OTA
│   ├── components/                 # Shared UI components
│   │   ├── Button.tsx
│   │   ├── Modal.tsx
│   │   ├── Sidebar.tsx
│   │   └── StatusBadge.tsx
│   ├── hooks/                      # Custom React hooks
│   │   ├── useApi.ts
│   │   ├── useSSE.ts
│   │   └── useAuth.ts
│   ├── lib/                        # Utilities
│   │   ├── api.ts                  # API client
│   │   └── types.ts                # Shared TypeScript types
│   └── assets/                     # Static assets (icons, images)
├── public/
├── package.json
├── tsconfig.json
├── vite.config.ts
└── tailwind.config.ts
```

## Component Conventions

### Named Exports Only

```tsx
// ✅ Good
export function AgentCard({ agent }: AgentCardProps) { ... }

// ❌ Bad
export default function AgentCard() { ... }
```

### Props Interface Pattern

```tsx
interface AgentCardProps {
  agent: Agent;
  onSelect: (id: string) => void;
  isActive?: boolean;
}

export function AgentCard({ agent, onSelect, isActive = false }: AgentCardProps) {
  return (
    <div
      className={`rounded-xl border p-4 cursor-pointer transition-all ${
        isActive ? 'border-blue-500 bg-blue-50' : 'border-gray-200 hover:border-gray-300'
      }`}
      onClick={() => onSelect(agent.agent_id)}
    >
      <h3 className="font-semibold text-lg">{agent.name}</h3>
      <p className="text-gray-500 text-sm">{agent.description}</p>
    </div>
  );
}
```

### API Integration Hook

```tsx
// hooks/useApi.ts
export function useApi<T>(endpoint: string) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch(`/api${endpoint}`, {
      headers: { 'Authorization': `Bearer ${getToken()}` },
    })
      .then(res => res.json())
      .then(json => setData(json.data))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, [endpoint]);

  return { data, loading, error };
}
```

### SSE Streaming Hook

```tsx
// hooks/useSSE.ts
export function useSSE(url: string, onToken: (content: string) => void) {
  useEffect(() => {
    const eventSource = new EventSource(url);

    eventSource.addEventListener('token', (e) => {
      const data = JSON.parse(e.data);
      onToken(data.content);
    });

    eventSource.addEventListener('done', () => {
      eventSource.close();
    });

    return () => eventSource.close();
  }, [url]);
}
```

## Vite Configuration

```ts
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    // Optimize for go:embed
    rollupOptions: {
      output: {
        manualChunks: undefined,
      },
    },
  },
});
```

## go:embed Integration

The built frontend is embedded into the Go binary:

```go
// internal/server/static.go
//go:embed all:../../web/dist
var embeddedAssets embed.FS
```

The `layered_fs.go` module serves assets with this priority:
1. `/data/overrides/` (runtime customization)
2. Embedded `web/dist/` (compiled into binary)

## Pages Overview

| Page | Route | Purpose |
|:---|:---|:---|
| Setup Wizard | `/setup` | First-time onboarding (Wi-Fi, API keys, OAuth) |
| Chat | `/chat/:agentId` | Streaming chat with an agent |
| Agents | `/agents` | Create, manage, and configure agents |
| Workspace | `/workspace` | Browse and manage sandbox files |
| Integrations | `/integrations` | Connect SaaS services via OAuth |
| Tool Hub | `/tools` | Manage MCP servers, skills, WASM plugins |
| Settings | `/settings` | System settings, metrics, updates |

## Development Workflow

```bash
# Start frontend dev server with HMR
cd web
npm run dev
# → http://localhost:5173

# API calls proxy to backend at localhost:8080
# (configure proxy in vite.config.ts)

# Build for production
npm run build
# → web/dist/

# Type-check without building
npx tsc --noEmit

# Lint
npx eslint src/
```

## Reference Files

- [docs/API.md](../../docs/API.md) — Backend API reference
- [web/vite.config.ts](../../web/vite.config.ts) — Vite configuration
- [internal/server/static.go](../../internal/server/static.go) — go:embed setup

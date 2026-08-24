# ActonOS Agent Rules

> Top-level development context for AI coding assistants working on the ActonOS project.

## Project Overview

ActonOS is an Extensible AI Agent Operating System Kernel written in **Go** with a **React 19** frontend. It compiles into a single static binary (`actond`) that runs on bare-metal MiniPCs or Docker containers.

## Key Architecture Decisions

1. **Single static binary**: `CGO_ENABLED=0` always. No CGO dependencies. Use `modernc.org/sqlite` (not `mattn/go-sqlite3`).
2. **Monorepo**: All code lives in one repository. Backend in `internal/`, frontend in `web/`, deployment in `deploy/`.
3. **Internal packages**: All Go packages are under `internal/` — nothing is exported as a library.
4. **Interface-driven design**: Core abstractions (`LLMProvider`, `ChannelAdapter`, `Sandbox`, `HAL`) are defined as Go interfaces.
5. **Event-driven**: Components communicate through the `internal/bus/` event bus using Go channels.
6. **Dual-runtime**: The HAL layer abstracts bare-metal vs Docker differences. Use build tags for platform-specific code (`_linux.go`).
7. **Single execution boundary**: All agent/API/cron/channel tool calls MUST go
   through `tools.ToolRegistry.Execute` so authorization, approvals, tracing,
   sandboxing, and audit behavior cannot be bypassed.

## Canonical Source Registry

**Before creating, modifying, or referencing any file**, consult the [Source Registry](.agents/rules/source-registry.md) for:
- All 70+ backend API routes and their handler files
- All frontend pages with NavTab IDs
- All UI components (actual files, not aspirational)
- All 14 locale namespaces
- Go ↔ TypeScript type mapping

## Mandatory Verification

**Every code change** must pass the checks in [Verification Checklist](.agents/rules/verification-checklist.md). This includes:
- Type sync (Go structs ↔ TypeScript types)
- i18n completeness (keys in BOTH `en/` and `vi/`)
- API doc sync (`docs/API.md`)
- Route registration (`router.go`)
- Build verification (`go vet` + `tsc --noEmit`)

## Change Impact Matrix

| If you change... | Also update... |
|:---|:---|
| Go struct fields (JSON-exposed) | `web/src/lib/types.ts` |
| API endpoint (add/modify) | `docs/API.md`, `router.go`, source-registry |
| UI text / labels | `locales/en/*.json` AND `locales/vi/*.json` |
| New page / tab | `App.tsx`, `Sidebar.tsx`, `nav.json` (both locales), source-registry |
| Design tokens / colors | `web/src/index.css`, `docs/DESIGN.md` |
| Agent manifest fields | `internal/agent/types.go`, `web/src/lib/types.ts`, `AgentFormModal.tsx` |
| LLM model catalog | `web/src/lib/models.ts` |
| New locale namespace | `web/src/lib/i18n.ts`, both `en/` and `vi/` directories |

## Coding Standards

### Go
- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Use `gofmt` and `goimports` for formatting
- All exported types, functions, and methods must have doc comments
- Prefer table-driven tests
- Error messages should be lowercase, no trailing punctuation
- Use `context.Context` as the first parameter for functions that may block
- Use `slog` for structured logging

### TypeScript/React
- Use TypeScript strict mode
- Prefer functional components with hooks
- Use named exports (no default exports)
- Co-locate component tests with their components

### Commits
- Use [Conventional Commits](https://www.conventionalcommits.org/) format
- Scope by package name: `feat(agent)`, `fix(memory)`, `docs`, etc.

## File Organization

When creating new files:
- Go source files go in the appropriate `internal/<package>/` directory
- Build tags go on the first line: `//go:build linux` or `//go:build integration`
- Test files are named `*_test.go` and live alongside the code they test
- Frontend pages go in `web/src/pages/<PageName>/`
- API endpoints go in `internal/server/api_<subsystem>.go`

## Documentation

- Update `docs/ARCHITECTURE.md` when changing core design
- Update `docs/API.md` when adding/changing endpoints
- Update `CHANGELOG.md` under `[Unreleased]` for user-facing changes

## Available Skills

Use these skills for specialized development workflows:
- `actonos-setup` — Project setup and environment verification
- `actonos-build` — Building the project (web → Go → Docker → ISO)
- `actonos-agent-dev` — Developing `internal/agent/` components
- `actonos-api-dev` — Developing REST API endpoints
- `actonos-frontend-dev` — Developing the React frontend
- `actonos-channels-dev` — Developing messaging channel adapters via WASM Plugins and `internal/channels/` routing/pairing
- `actonos-testing` — Writing and running tests
- `actonos-release` — Version bumping and release management

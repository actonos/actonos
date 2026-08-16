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
- `actonos-testing` — Writing and running tests
- `actonos-release` — Version bumping and release management

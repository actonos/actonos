# Mandatory Verification Checklist

> **This rule is MANDATORY.** Every code change must pass these checks before being submitted.

## Pre-Flight: Before Submitting Any Code Change

### Go Backend Changes

- [ ] **Error Wrapping**: All errors wrapped with `fmt.Errorf("context: %w", err)` — never raw `return err`
- [ ] **Handler Registration**: If you added a handler function, verify it's registered in [`router.go`](../../internal/server/router.go) `setupRoutes()`
- [ ] **API Doc Sync**: If you added/changed API endpoints, update [`docs/API.md`](../../docs/API.md)
- [ ] **Struct Field Sync**: If you modified Go struct fields exposed via JSON, update [`web/src/lib/types.ts`](../../web/src/lib/types.ts) to match
- [ ] **Import Cleanup**: No unused imports (`go vet ./...` catches this)
- [ ] **Logging**: Use `slog.Info/Warn/Error` — never `fmt.Printf` or `log.Printf`
- [ ] **Context**: Functions that may block take `ctx context.Context` as first parameter

### Frontend Changes

- [ ] **i18n Completeness**: Every user-facing string uses `t('key')` — zero hardcoded text
- [ ] **Locale Parity**: If you added keys to `en/*.json`, add the same keys to `vi/*.json` (and vice versa)
- [ ] **Named Exports**: Use `export function X()` — never `export default`
- [ ] **No emoji** in UI copy (`npm run check:emoji`)
- [ ] **Type Safety**: No `any` types. All props have TypeScript interfaces
- [ ] **Design System**: Follow `docs/DESIGN.md` — pill radius, correct colors, no drop shadows, correct fonts

### After Creating New Files

| New File Type | Mandatory Actions |
|:---|:---|
| **Go source file** | Verify package name matches directory; add build tags if platform-specific |
| **React component** | Named export, typed props interface, `useTranslation()` for all text |
| **API endpoint** | Register route in `router.go`, add to `docs/API.md`, add request/response types |
| **New page** | Add to `App.tsx` routing, add Sidebar nav entry, add locale keys for page title in `nav.json` |
| **Locale namespace** | Add to BOTH `en/` and `vi/`, register in `web/src/lib/i18n.ts` |

## Post-Flight: After Completing a Task

1. **Build Check (Go)**: `go vet ./...` must pass
2. **Build Check (Frontend)**: `cd web && npx tsc --noEmit` must pass
3. **CHANGELOG**: Add entry under `[Unreleased]` in `CHANGELOG.md` for user-facing changes
4. **Documentation**: If you changed core design, update `docs/ARCHITECTURE.md`

## Change Impact Matrix

The matrix lives in the root `AGENTS.md` ("Change impact"); keep it only there.

## Commands by scope

| Scope | Narrow (while working) | Final gate |
|:---|:---|:---|
| Go | `make check-fast PKG=./internal/<pkg>/...` (or `go test ./internal/<pkg> -run Name -count=1`) | `make test-unit` (includes the coverage floors), `make test-race` for concurrency changes |
| Web | `make check-web` (or `cd web && npx vitest run <file>`) | `make check-full` (includes `npm run quality`; run it alone, parallel load makes Playwright time out) |
| Mail and calendar | `go test ./internal/mailcal/... -count=1` | add `make conformance`; sending/gate changes also need `-race` |
| Commits | Conventional Commits scoped by package; no `Co-Authored-By` or "Generated with" trailer in commits or PR text | |

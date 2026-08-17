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

Use this matrix to identify all files that must be updated when making changes:

| If you change... | Also update... |
|:---|:---|
| Go struct fields (JSON-exposed) | `web/src/lib/types.ts` |
| API endpoint (add/modify) | `docs/API.md`, `router.go` route registration |
| UI text / labels | `locales/en/*.json` AND `locales/vi/*.json` |
| New page / tab | `App.tsx`, `Sidebar.tsx`, `nav.json` (both locales) |
| Design tokens / colors | `web/src/index.css`, `docs/DESIGN.md` |
| Agent manifest fields | `internal/agent/types.go`, `web/src/lib/types.ts`, `AgentFormModal.tsx` |
| LLM model catalog | `web/src/lib/models.ts` |
| New locale namespace | `web/src/lib/i18n.ts`, both `en/` and `vi/` directories |
| New internal package | `AGENTS.md` or relevant skill file |

---
name: actonos-mailcal-dev
description: "Skill for developing the mail and calendar subsystem (internal/mailcal): provider plugins, the send gate and rule engine, OAuth sign-in, conformance, and the Mail and Calendar screens."
---

# ActonOS Mail and Calendar Development Skill

Read `internal/mailcal/AGENTS.md` first (map and invariants). This skill lists the recipes.

## Add a provider plugin

1. Copy `internal/mailcal/plugins/echomail/` (read-only) or the closest shipped provider (`caldav` for password calendars, `gmail` for OAuth, `imapsmtp` for TLS sockets).
2. Implement `provkit.Backend` (embed `provkit.Base`); list exactly the implemented exports in `Capabilities().Ops`; every error carries an ABI code.
3. Manifest: smallest permissions; no bare `*`; no secrets/storage/workspace/bus; `oauth` block and `permissions.oauth` agree.
4. Tests: plain Go tests with `netio.Stdlib` and fake servers, then `go test ./internal/mailcal -run EveryShipped`.
5. `Makefile` target modeled on `plugin-caldav`, then `make conformance`. No core file may change (`git status` shows only the plugin directory and the Makefile).

## Change sending, rules or approvals

- Touches `internal/tools/gate.go`, `internal/mailcal/policy`, `writeops.go`: add a failing test first (see `write_test.go`, `triage_cron_test.go`, `policy_test.go`), run with `-race`, and update the threat table in `docs/SECURITY.md`.
- Never widen what `needs_approval` can skip; never add a send path that accepts free text.

## Add an API route or screen

- Handler in `internal/server/api_mailcal*.go`, route in `router.go`, `docs/API.md`, source-registry, TS type and client in `web/src/lib/api/mailcal.ts`, locale keys in both languages, vitest for the component.

## Verify

```bash
go test ./internal/mailcal/... -count=1        # add -race for gate/ledger changes
make conformance                                # builds and certifies the shipped plugins
cd web && npx vitest run src/pages/Mailcal && npx tsc --noEmit
scripts/eval-office.sh <provider/model>         # live eval; run by the user, keys from the environment
```

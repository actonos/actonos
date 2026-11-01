# internal/mailcal

Mail and calendar. **The core decides, plugins execute, and plugins are always untrusted.** Full design: `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md` (long; read only the section you need). Plugin authors: `docs/MAILCAL-PLUGIN-GUIDE.md`.

## Map

| Path | Role |
|:---|:---|
| `abi/` | Wire contract with plugins (`mail@1`, `calendar@1`). Additive-only; `TestGoldenV1` guards it |
| `policy/` | Rule engine: deny wins, exact recipients/domains, limits, fail closed. Fuzzed |
| `service.go`, `tools_read.go`, `tools_write.go`, `writeops.go` | Providers, tools, the send/delete/calendar write path |
| `drafts.go`, `ledger.go`, `proposals.go`, `rules.go`, `decisions.go`, `settings.go` | Immutable drafts, one-send-per-draft ledger, rule proposals, audit, master switch |
| `oauth.go`, `tokens.go`, `credentials.go`, `clients.go` | Sign-in run by the core, token and app-password brokers |
| `provkit/` | Toolkit plugins are built from (`Backend`/`Base`, `netio`, `mimekit`, `icskit`, `wasmguest`) |
| `plugins/{gmail,graph,caldav,imapsmtp,echomail}` | Shipped providers; `echomail` is the minimal example |
| `conformance/`, `certificate/` | Suite every provider passes; hash-bound certificate (information only) |
| `refplugin/`, `refstore/` | Reference provider and dataset used by tests and evals |

## Invariants (never weaken; each has a test)

- External-effect tools (send/delete mail, calendar writes) pass the mandatory `ToolGate` inside `ToolRegistry.Execute` (`internal/tools/gate.go`): always High risk, no grants, no gate means no run, `needs_approval` cannot be bypassed by threshold or cron (`WithBypassApproval`).
- `native_mail_send` takes only a `draft_id`; drafts are immutable; an approval binds to the draft; a draft is sent at most once (ledger).
- The agent can only propose rules; only an authenticated person approves. Master switch is off by default.
- Secrets live in the vault; never in account config, API responses, logs or certificates.
- Mail text is untrusted data (wrapped by `untrusted.go`); never let it become an instruction.
- Adding a provider must not change any core file. If it seems to, fix the abstraction instead.

## Working here

- Test: `go test ./internal/mailcal/... -count=1` (the package alone takes about 2 minutes; use `-run` while iterating). Add `-race` for send/ledger/gate changes.
- `allplugins_test.go` gates every directory under `plugins/` automatically.
- New plugin: copy `plugins/echomail`, add a `Makefile` target like `plugin-caldav`, run `make conformance`.
- API/UI changes: `internal/server/api_mailcal*.go`, `web/src/pages/Mailcal/`, `web/src/lib/api/mailcal.ts`, locale `mailcal.json` (en and vi), `docs/API.md`, source-registry.
- Live services (Google, Microsoft, CalDAV, IMAP) and live evals are run by the user; never put keys in files.

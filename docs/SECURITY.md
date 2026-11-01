# Security Policy

## Supported Versions

| Version | Supported |
|:---|:---|
| Latest release | ✅ Full support |
| Previous minor | ✅ Security patches only |
| Older versions | ❌ No support |

---

## Reporting a Vulnerability

**Please do NOT file public GitHub issues for security vulnerabilities.**

### Responsible Disclosure

If you discover a security vulnerability in ActonOS, please report it responsibly:

1. **Email**: Send a detailed report to `security@actonos.io`
2. **Subject line**: `[SECURITY] Brief description of the vulnerability`
3. **Include**:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact assessment
   - Suggested fix (if any)
   - Your name/handle for credit (optional)

### Response Timeline

| Phase | Timeframe |
|:---|:---|
| Acknowledgment | Within **48 hours** |
| Initial Assessment | Within **5 business days** |
| Fix Development | Within **30 days** (critical: 7 days) |
| Public Disclosure | After fix is released |

### Recognition

We gratefully acknowledge security researchers who practice responsible disclosure. With your permission, we will credit you in the security advisory and CHANGELOG.

---

## Security Architecture

### Sandboxing Model

ActonOS isolates agent-executed commands using a multi-layered sandbox:

#### Bare-metal Mode

| Control | Configuration |
|:---|:---|
| **Filesystem** | Read-only bind mounts for `/usr`, `/bin`, `/lib`; writable only within `/workspace` |
| **Capabilities** | All capabilities dropped (`--cap-drop ALL`) |
| **Namespaces** | Full unsharing (`--unshare-all`) with a new session |
| **Lifecycle** | `--die-with-parent` (killed when parent terminates) |

Bubblewrap is mandatory on Linux bare-metal. ActonOS does not silently fall back
to a host subshell. Memory/process fields remain in the sandbox interface for
deployment-specific cgroup enforcement.
Each bare-metal command receives a dedicated cgroup v2 enforcing 512 MB memory,
30 processes, and 50% of one CPU by default. Failure to attach the process to the
cgroup terminates the process and rejects the action.

#### Docker Mode

- Shell commands execute inside the existing container boundary
- No host filesystem access beyond the `/data` volume

On Windows/macOS development hosts, command execution is disabled by default.
`ACTONOS_ALLOW_INSECURE_EXEC=1` is an explicit unsafe development override and
must not be enabled in production.

### WASM Plugin Sandboxing & Capability Model

WASM plugins execute in **Wazero** within an isolated linear memory sandbox. They possess zero ambient authority:

1. **Capability-Based Permissions**: Plugins declare requested permissions in `manifest.json` (`net_outbound`, `secrets`, `storage`, `bus_events`). All undeclared actions are denied by default.
2. **Egress Network Firewall**: Outbound HTTP/WebSocket traffic is proxied through host syscalls (`acton_net.http_request`, `acton_ws.ws_connect`). Destinations must match `net_outbound` (exact host or `*.example.com`; a bare `*` or `*.com` is rejected) and pass `security.ValidateOutboundURL` (no loopback, private, link-local, metadata, or non-HTTP schemes). Redirect hops are re-validated. Direct raw TCP/UDP sockets are impossible.
3. **Vault brokering**: Plugins cannot read host files or database tables. Scoped secrets requested via `acton_vault.get_secret` are validated against `manifest.permissions.secrets` and decrypted from the AES-256-GCM vault. Retrieved values are redacted from plugin logs. Vault keys are Argon2id-derived; DMI/CPU hardware binding is not applied.
4. **Signed packages**: `acton-plugin pack` does not require `signature.sig`. Missing signatures are allowed on upload and 1-click install. When `signature.sig` is present it is Ed25519 over SHA-256(`manifest.json` || `plugin.wasm`) and must verify against `ACTONOS_PLUGIN_PUBKEYS`. Set `ACTONOS_REQUIRE_SIGNED_PLUGINS=1` to fail closed on unsigned packages.
5. **Resource Metering & Fault Isolation**: Each plugin instance is capped at 64 MB linear memory and a 15s tool/poll deadline. Guest panics and memory traps are caught without crashing `actond`.

### Vault Encryption

Sensitive data (API keys, OAuth tokens) is encrypted at rest using:

| Component | Algorithm |
|:---|:---|
| **Encryption** | AES-256-GCM |
| **Key Derivation** | Argon2id (memory-hard) |
| **Key Material** | Argon2id-derived key from the configured vault secret plus a random salt |

Vault ciphertext is encrypted at rest. Hardware DMI/CPU binding is not applied in the production constructor.

### Risk-Based Approval System

All tool executions pass through a risk assessment filter:

| Risk Level | Actions | Policy |
|:---|:---|:---|
| **Low** | Local read-only operations | Auto-execute when authorized |
| **Medium** | Network access and external notifications | Approval at Medium threshold |
| **High** | Shell, write/delete, schedule mutation, MCP/WASM | Exact-action approval |

Approvals are persisted in SQLite, expire after 30 minutes, and are bound to a
SHA-256 hash of agent ID, tool name, and normalized arguments. Approval of one
action cannot authorize modified arguments.

### Execution Boundary

- `AuthorizedTools` is enforced again inside `ToolRegistry.Execute`.
- `AllowedWorkspacePaths` is enforced for every native file tool.
- Destructive command patterns are denied before sandbox startup.
- Non-zero command exit codes are execution failures and become ReAct observations.
- HTTP fetch blocks SSRF to localhost, private, link-local, multicast, and cloud
  metadata address ranges, including redirect targets.
- MCP stdio requires Docker/Bubblewrap or the explicit
  `ACTONOS_ALLOW_UNSANDBOXED_MCP=1` development override.

### Audit Logging

All agent actions are logged in OpenTelemetry-compatible structured JSON at `/data/logs/audit.jsonl`:

```json
{
  "timestamp": "2026-08-16T23:55:00Z",
  "trace_id": "9a8b7c6d5e4f3210123456789abcdef0",
  "agent_id": "agent_dev_assistant_01",
  "tool_name": "skill_run_bash",
  "risk_level": "Medium",
  "execution_time_ms": 142,
  "status": "Success"
}
```

Every record contains `previous_hash` and `entry_hash`. The SHA-256 chain covers
the canonical entry payload, making deletion, reordering, and modification
detectable. Administrators can verify it through
`GET /api/system/audit/verify`.

Approval checkpoints include the exact pending tool call and full execution
state. Resume consumes the approved action once and continues the original run;
changed arguments require a new approval. The same gate protects workspace,
skill, WASM, Tool Hub, and restart mutations.

### Network Security

| Feature | Implementation |
|:---|:---|
| **Remote Access** | Tailscale `tsnet` (WireGuard-based, E2E encrypted mesh VPN) |
| **No Open Ports** | All remote access via Tailscale; no port forwarding required |
| **OAuth 2.1** | PKCE with S256 challenge; no implicit grant flow |
| **Token Management** | Automatic refresh daemon; tokens encrypted at rest |
| **mDNS** | Local network discovery only (`acton.local`) |

### Provider Secret Storage

LLM API keys are stored only in the AES-256-GCM Vault under
`llm.provider.<provider>.api_key`. Provider JSON contains non-secret metadata
only. On startup or first access, legacy plaintext `llm_providers.json` values
and `<provider>.key` files are migrated into Vault and removed after successful
encryption. Saving keys fails closed when Vault is unavailable.

### Durable Run Guardrails

Agent runs stop on eight iterations, three consecutive tool failures, repeated
identical observations, context cancellation, budget exhaustion, approval wait,
or final verification failure. Run and event records are available through
`/api/runs` for incident analysis.

### Realtime UI Boundary

The realtime WebSocket authenticates with a Strict SameSite, HttpOnly session
cookie. Tokens are not placed in WebSocket URLs. The channel is read-only and
contains operational metadata only; secrets and MCP environment values are
excluded. The xterm.js UI is an observation terminal, not an interactive host
shell. Live Canvas is disabled until the operator explicitly configures
`ACTONOS_CANVAS_URL`.

The browser never persists the returned API bearer token in Web Storage.
State-changing APIs can return `202 Accepted` with an exact-action approval;
the UI must treat this as pending, open the global approval interruption, and
must not display completion or apply optimistic state. MCP toggle and
disconnect follow this same durable approval path.

Live Canvas accepts same-origin relative paths, HTTPS, and plaintext HTTP only
for localhost/loopback. Its iframe is sandboxed without clipboard permissions.
The web server also emits Content Security Policy and Permissions Policy
headers for embedded UI responses.

### Mail and Calendar

Provider plugins (Gmail, Graph, CalDAV, IMAP/SMTP) are untrusted code; the core decides, plugins execute.

- **Mandatory gate.** `native_mail_send`, `native_mail_delete` and the calendar write tools are in the registry's external-effect list. They are always High risk, are never covered by a "don't ask again" grant or auto-approval, and refuse to run unless a gate is registered. A gate verdict of `needs_approval` forces a human approval for the exact input, regardless of the agent's approval threshold or a background (cron) run. A gate that errors or returns an unknown verdict denies.
- **Agents cannot send arbitrary text.** `native_mail_send` takes only a `draft_id`. Drafts are immutable (a database trigger rejects `UPDATE`) and carry an integrity hash and a reply context verified with the provider when the draft was made. An approval binds to the draft, so changing the text means a new draft and a new approval.
- **Rules.** Auto-send is closed by default (master switch off, no rules). Deny rules win; allow rules need exact recipients or domains (no wildcards, no public mail domains unless explicitly allowed), per-hour and per-day limits, and match the real payload. Bcc, attachments, forwards, links (unless allowed) and any deletion always need a person. Only a signed-in person can approve a rule proposal; the agent tool can only create proposals. Several returned messages in an hour pause the account's rules.
- **No double sends.** Sending goes through an idempotency ledger keyed by the draft; a draft is sent at most once, including under concurrent attempts and lost answers.
- **Message text is data.** Content from providers is sanitized and wrapped as untrusted, with an explicit notice, before it reaches a model.
- **Controlled TLS sockets (`acton_tcp`).** Plugins connect only to the `host:port` pairs in `permissions.net_tcp`; there is no plaintext mode. The core resolves the name once, refuses the connection if any address is loopback, private, link-local, CGNAT or multicast, dials that vetted address, and verifies the certificate against the name (TLS 1.2 or newer). For SMTP and IMAP STARTTLS the host runs the plaintext phase with fixed bytes, requires the server to advertise STARTTLS, and aborts if any data arrives before the handshake. Limits: 4 connections per plugin, 60 s idle, 50 MiB per direction. The default HTTP client for plugins dials the same vetted address. Not covered yet: the WebSocket path of plugins still resolves the name again after checking it.
- **Verification labels are information, not trust.** A `conformance.json` next to `plugin.wasm` (written by `cmd/mailcal-conformance`) makes the account page show "verified against ABI 1" only while its SHA-256 matches the module. A plugin without one runs in the same sandbox behind the same gates.
- **Known limits.** Message content can still mislead the *person* who approves; a rule in `free` text mode can disclose data to the sender it replies to. Nothing here has been checked against live Google, Microsoft, CalDAV or IMAP/SMTP services by the automated suite (see the spec, section 17).

#### Mail and calendar threats and the tests that pin them

| Threat | Defense | Tests |
|:---|:---|:---|
| Hostile mail makes the agent send or delete | Master switch off by default; mandatory gate; sends need a draft and, without a rule, a person | `TestSendIsDeniedWhileTheMasterSwitchIsOff`, `TestSendWithoutARuleNeedsApprovalEvenWhenBypassIsSet`, `TestDeletesAlwaysNeedAPerson`, office evals 35 to 46 |
| Agent bypasses approval (threshold, cron, grant) | External-effect tools are always High, never grant-eligible, `needs_approval` cannot be waved through | `TestGateApprovalCannotBeBypassed`, `TestExternalEffectToolsAreHighRiskAndGrantIneligible`, `TestACronRunWithoutARuleStopsAtAnApprovalRequest` |
| A tool is registered without a gate | Fail closed: no gate, no run | `TestExternalEffectToolWithoutGateIsDenied`, `TestGateDenyAndErrorFailClosed` |
| Text changed after approval | Immutable drafts, approval bound to the draft | `TestDraftIsStoredNormalizedAndImmutable`, `TestDraftDetectsTampering`, `TestApprovalCannotBeReusedForADifferentDraft` |
| Agent widens its own permissions | The agent can only propose rules; a person approves | `TestAnAgentCanOnlyProposeRules` |
| Overbroad or lookalike rules | Exact addresses, no wildcards, deny wins, fail closed on limits and unknown usage | `TestAdversarialRequestsNeverAllow`, `TestDenyBeatsAllow`, `TestValidateRejectsDangerousRules`, `TestLimitsAndUnknownUsageFailClosed` |
| Double send (retry, race, lost answer) | Idempotency ledger keyed by draft; provider-side keys; SMTP at most once | `TestApprovedSendHappensExactlyOnce`, `TestConcurrentSendsOfOneDraftSendOnce`, `TestALostAnswerAfterSendingDoesNotSendTwice`, `TestACronRunUnderAMatchingRuleSendsOnceAndRetriesAreReplays` |
| Prompt injection in mail text | Hidden text stripped; content wrapped as untrusted | `TestSanitizeTextStripsHiddenAndControlCharacters`, `TestWrapUntrustedCannotBeClosedEarlyByTheContent` |
| Stranger links their mailbox through OAuth | PKCE, single-use state, nonce cookie | `TestSignInRejectsAForeignBrowserAndCannotBeReplayed` |
| Plugin reads another plugin's token or password | Broker checks the plugin id from the host context | `TestBrokerNeverServesAnotherPluginsAccount`, `TestCredentialsNeverGoToAnotherPlugin` |
| Secrets leak into records, API or certificates | Vault only; write-only in the API | `TestPasswordsLiveInTheVaultAndNeverInTheAccountRecord`, `TestACertificateNeverContainsSecrets` |
| Plugin reaches the local network | Resolve once, refuse private and loopback, verify the name | `TestSafeAddrRefusesLocalAndPrivateTargets`, `TestTheNameIsResolvedOnceSoRebindingCannotSwapTheTarget`, `TestOnlyTheListedHostAndPortAreReachable` |
| STARTTLS downgrade or plaintext injection | The host runs the plaintext phase and aborts on any surprise | `TestStartTLSIsNeverDowngradedOrInjected` |
| A plugin asks for more than a provider needs | Manifest gate over every shipped plugin | `TestEveryShippedPluginAsksForOnlyWhatAProviderNeeds` |
| Forged or stale conformance label | Certificate is bound to the module hash | `TestACertificateStopsMatchingWhenThePluginChanges`, `TestAProviderIsReportedVerifiedOnlyWhileItsCertificateMatchesTheModule` |
| Bounces damage reputation | Rules pause after several returned messages | `TestSeveralBouncesPauseTheRules` |

---

## Best Practices for Deployers

1. **Set a strong admin PIN** during onboarding
2. **Use Tailscale** for remote access instead of exposing port 8080
3. **Review audit logs** regularly at `/data/logs/audit.jsonl`
4. **Keep ActonOS updated** — apply signed-checksum OTA releases and confirm `/api/health` after restart
5. **Restrict agent permissions** — use minimal delegation scopes
6. **Back up `/data`** regularly (see [DEPLOYMENT.md](DEPLOYMENT.md#backup--restore))
7. **Monitor the health endpoint** (`/api/health`) for anomalies

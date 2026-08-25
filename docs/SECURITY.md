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

---

## Best Practices for Deployers

1. **Set a strong admin PIN** during onboarding
2. **Use Tailscale** for remote access instead of exposing port 8080
3. **Review audit logs** regularly at `/data/logs/audit.jsonl`
4. **Keep ActonOS updated** — apply signed-checksum OTA releases and confirm `/api/health` after restart
5. **Restrict agent permissions** — use minimal delegation scopes
6. **Back up `/data`** regularly (see [DEPLOYMENT.md](DEPLOYMENT.md#backup--restore))
7. **Monitor the health endpoint** (`/api/health`) for anomalies

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

- WASM plugins run in `wazero` (in-memory, no filesystem access)
- Shell commands execute inside the existing container boundary
- No host filesystem access beyond the `/data` volume

On Windows/macOS development hosts, command execution is disabled by default.
`ACTONOS_ALLOW_INSECURE_EXEC=1` is an explicit unsafe development override and
must not be enabled in production.

### Vault Encryption

Sensitive data (API keys, OAuth tokens) is encrypted at rest using:

| Component | Algorithm |
|:---|:---|
| **Encryption** | AES-256-GCM |
| **Key Derivation** | Argon2id (memory-hard) |
| **Key Material** | DMI product UUID + CPU serial + random salt |

This hardware-binding ensures that vault data cannot be decrypted if the storage is moved to a different machine.

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

---

## Best Practices for Deployers

1. **Set a strong admin PIN** during onboarding
2. **Use Tailscale** for remote access instead of exposing port 8080
3. **Review audit logs** regularly at `/data/logs/audit.jsonl`
4. **Keep ActonOS updated** — enable automatic OTA updates
5. **Restrict agent permissions** — use minimal delegation scopes
6. **Back up `/data`** regularly (see [DEPLOYMENT.md](DEPLOYMENT.md#backup--restore))
7. **Monitor the health endpoint** (`/api/health`) for anomalies

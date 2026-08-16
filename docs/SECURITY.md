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

#### Bare-metal Mode (Bubblewrap + Cgroups v2)

| Control | Configuration |
|:---|:---|
| **Filesystem** | Read-only bind mounts for `/usr`, `/bin`, `/lib`; writable only within `/workspace` |
| **Memory** | `memory.max = 512 MB` per execution |
| **CPU** | `cpu.max = 50000/100000` (50% of 1 core) |
| **Processes** | `pids.max = 30` child processes |
| **Capabilities** | All capabilities dropped (`--cap-drop ALL`) |
| **Namespaces** | Full unsharing (`--unshare-all`), user namespaces disabled |
| **Lifecycle** | `--die-with-parent` (killed when parent terminates) |

#### Docker Mode

- WASM plugins run in `wazero` (in-memory, no filesystem access)
- Shell commands execute in a jailed subprocess with limited PATH
- No host filesystem access beyond the `/data` volume

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
| **Low** | Read-only operations (search, fetch, view) | Auto-execute |
| **Medium** | Scoped writes (create files, write docs) | Auto-execute within authorized workspace |
| **High** | Destructive/financial (push to main, send email, modify DB) | Human approval required (Telegram/Web UI confirmation) |

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

### Network Security

| Feature | Implementation |
|:---|:---|
| **Remote Access** | Tailscale `tsnet` (WireGuard-based, E2E encrypted mesh VPN) |
| **No Open Ports** | All remote access via Tailscale; no port forwarding required |
| **OAuth 2.1** | PKCE with S256 challenge; no implicit grant flow |
| **Token Management** | Automatic refresh daemon; tokens encrypted at rest |
| **mDNS** | Local network discovery only (`acton.local`) |

---

## Best Practices for Deployers

1. **Set a strong admin PIN** during onboarding
2. **Use Tailscale** for remote access instead of exposing port 8080
3. **Review audit logs** regularly at `/data/logs/audit.jsonl`
4. **Keep ActonOS updated** — enable automatic OTA updates
5. **Restrict agent permissions** — use minimal delegation scopes
6. **Back up `/data`** regularly (see [DEPLOYMENT.md](DEPLOYMENT.md#backup--restore))
7. **Monitor the health endpoint** (`/api/health`) for anomalies

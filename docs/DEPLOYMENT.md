# ActonOS Deployment Guide

> Instructions for deploying ActonOS in Docker and bare-metal environments.

---

## Table of Contents

- [Deployment Modes](#deployment-modes)
- [Docker Deployment](#docker-deployment)
- [Bare-metal MiniPC Installation](#bare-metal-minipc-installation)
- [Environment Variables](#environment-variables)
- [Data Directory Layout](#data-directory-layout)
- [Building the ISO](#building-the-iso)
- [OTA Updates](#ota-updates)
- [Monitoring & Health Checks](#monitoring--health-checks)
- [Backup & Restore](#backup--restore)

---

## Deployment Modes

| Mode | Target | Detection | Key Differences |
|:---|:---|:---|:---|
| **Docker** | Cloud, NAS, any Linux host | `RUNTIME_MODE=docker` env var or container detection | Host/bridge networking, WASM/jailed sandbox, container metrics |
| **Bare-metal** | MiniPC (Intel N100/N95, AMD Ryzen) | Absence of container markers | D-Bus NetworkManager, Bubblewrap + Cgroups v2, OTA state + systemd restart |

The `actond` binary auto-detects its runtime environment via the Hardware Abstraction Layer (HAL).

---

## Docker Deployment

### Quick Start

```bash
docker run -d \
  --name actonos \
  -p 8080:8080 \
  -v acton-data:/data \
  -e RUNTIME_MODE=docker \
  --restart unless-stopped \
  actonos/actonos:latest
```

### Agent Batteries-Included Runtime

The ActonOS Docker container is pre-configured with a full developer and automation toolchain so AI agents can execute tasks autonomously:

| Category | Pre-installed Tools & Libraries |
|:---|:---|
| **Search & Files** | `ripgrep` (`rg`), `jq`, `tree`, `findutils`, `tar`, `gzip`, `unzip` |
| **Language Runtimes** | `python3`, `pip`, `venv` (in `/opt/venv`), `nodejs`, `npm`, `npx` |
| **Python Libraries** | `requests`, `httpx`, `beautifulsoup4`, `pyyaml`, `pydantic` |
| **Network & VCS** | `curl`, `wget`, `git`, `sqlite3`, `openssh-client` |
| **Headless Web & Browser** | `chromium`, `font-noto`, `font-noto-cjk` (supports `chromedp` web browsing & screenshots) |
| **Process Init & Security** | `tini` (PID 1), non-root `acton` user (UID 1000) |

### Docker Compose Deployment

A complete production `docker-compose.yml` is provided in `deploy/docker/`:

```bash
# 1. Navigate to docker directory
cd deploy/docker

# 2. Copy environment template
cp .env.example .env

# 3. Start ActonOS in background
docker compose up -d

# 4. View real-time logs
docker compose logs -f
```

#### Automated HTTPS with Caddy

For public-facing production setups with automatic SSL/TLS certificates:

```bash
DOMAIN=agent.yourdomain.com docker compose -f deploy/docker/docker-compose.caddy.yml up -d
```

### Building the Docker Image

```bash
# Build standard image
make docker

# Build multi-architecture image (linux/amd64 and linux/arm64)
make docker-multiarch
```

---

## Bare-metal MiniPC Installation

### Supported Hardware

| Component | Recommended | Minimum |
|:---|:---|:---|
| CPU | Intel N100 / AMD Ryzen 5 | Intel N95 / any x86_64 |
| RAM | 4 GB | 2 GB |
| Storage | 64 GB NVMe/SSD | 32 GB |
| Network | Ethernet + Wi-Fi | Ethernet only |

### Installation Steps

1. **Download the ISO**
   ```bash
   # From GitHub Releases
   wget https://github.com/actonos/actonos/releases/latest/download/ActonOS-latest.iso
   ```

2. **Flash to USB drive**
   ```bash
   sudo dd if=ActonOS-latest.iso of=/dev/sdX bs=4M status=progress
   sync
   ```

3. **Boot the MiniPC from USB**
   - Enter BIOS → set USB as first boot device
   - Installation is fully automated (preseed/auto-install.cfg)
   - Automatic disk partitioning:
     - ESP: 512 MB (FAT32, UEFI bootloader)
     - System Root: 4 GB (Ext4, read-only)
     - User Data: remaining space (Ext4, read-write)

4. **First-time Setup**
   - Open the Web UI on the LAN and complete the setup wizard
   - Open captive portal or navigate to `http://192.168.4.1`
   - Complete the Setup Wizard:
     - Select your home Wi-Fi network
     - Enter LLM API keys (OpenAI, Anthropic, Google, etc.)
     - Connect SaaS services via OAuth 1-click
     - Set admin PIN
     - (Optional) Enter Tailscale auth key for remote access

5. **Access the Dashboard**
   - Local: `http://acton.local` (via mDNS)
   - Remote: via Tailscale mesh VPN URL

---

## Environment Variables

| Variable | Default | Description |
|:---|:---|:---|
| `RUNTIME_MODE` | auto-detect | Force `docker` or `baremetal` mode |
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `LISTEN_ADDR` | `:8080` | HTTP server listen address |
| `DATA_DIR` | `/data` | Path to persistent data directory |
| `DISABLE_TAILSCALE` | `false` | Skip Tailscale initialization |
| `ACTONOS_ALLOW_INSECURE_EXEC` | unset | Dev-only unsandboxed command exec; never set in production |
| `ACTONOS_PLUGIN_PUBKEYS` | unset | Comma-separated Ed25519 public keys (hex) used to verify `signature.sig` when present |
| `ACTONOS_REQUIRE_SIGNED_PLUGINS` | unset | Set to `1` to reject unsigned `.actonpkg` installs (`acton-plugin pack` omits signatures by default) |
| `ACTONOS_ALLOW_UNSIGNED_PLUGINS` | unset | Set to `1` to allow unsigned installs even when signed plugins are required |
| `TAILSCALE_AUTH_KEY` | — | Tailscale auth key for headless setup |
| `TZ` | `UTC` | Timezone for cron schedules and logs |

---

## Data Directory Layout

All persistent state lives under the `DATA_DIR` (default: `/data`):

```
/data/
├── bin/                 # Symlink to active actond binary
│   └── actond → ../releases/v1.0.0/actond
├── releases/            # Versioned binary releases
│   ├── v1.0.0/actond
│   └── v1.0.1/actond
├── config/
│   └── vault.db         # Encrypted API keys & user settings
├── agents/
│   └── agent_manifests.json  # User-created agent configurations
├── tokens/
│   └── oauth_tokens.vault    # Encrypted OAuth2 refresh/access tokens
├── storage/
│   └── app.db           # SQLite: chat logs, FTS5 index, vector index
├── logs/
│   └── audit.jsonl      # OpenTelemetry structured audit log
├── overrides/           # Custom Web UI / prompt overrides
├── plugins/             # WASM plugin packages (/data/plugins/<id>/plugin.wasm + manifest.json)
├── skills/              # Skill script folders
│   └── <skill_name>/
│       ├── skill.json   # Tool schema
│       └── run.sh       # Executable script
├── mcp-servers/         # MCP server configs and binaries
└── workspace/           # Sandboxed agent read/write area
```

---

## Building the ISO

```bash
# Prerequisites (on a Debian/Ubuntu host)
sudo apt-get install -y live-build debootstrap

# Build the ISO
make iso
# Or directly:
bash scripts/build-iso.sh

# Output: build/ActonOS-v<VERSION>.iso
```

The ISO build process:
1. Creates a Debian 12 minimal base using `live-build`
2. Installs required packages (bubblewrap, network-manager, wpasupplicant)
3. Copies the `actond` binary and systemd service unit
4. Applies the preseed configuration for automated installation
5. Generates a bootable hybrid ISO

---

## OTA Updates

### Automatic Updates

Operator-triggered OTA (Settings → Maintenance) applies an update atomically:

1. Daemon fetches `https://api.github.com/repos/actonos/actonos/releases/latest`
2. Download `actond_v{version}_{arch}[.exe]` and `embeddingd_…` into `{dataDir}/releases/{version}/`
3. Verify SHA-256 (`digest` or `SHA256SUMS`)
4. Activate into `{dataDir}/bin/` (Linux symlink; Windows copy)
5. Persist `{active, previous}` in `{dataDir}/releases/state.json`
6. Restart embeddingd (if swapped) then actond
7. **Rollback**: distinct High-risk `admin_ota_rollback`, includes restart

Docker image binaries are not overwritten (`apply_supported=false`). A Windows supervisor must exec `{dataDir}/bin/actond.exe`, not `build/actond.exe`.

### Manual Update

```bash
# Public GitHub assets
curl -L -o /data/releases/v1.0.1/actond \
  https://github.com/actonos/actonos/releases/latest/download/actond_v1.0.1_x86_64
ln -sfn /data/releases/v1.0.1/actond /data/bin/actond
systemctl restart actond
```

---

## Monitoring & Health Checks

### Health Endpoint

```bash
curl http://localhost:8080/api/health
```

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 86400,
  "runtime_mode": "baremetal",
  "agents_active": 3,
  "memory_usage_mb": 28.5,
  "cpu_percent": 2.1,
  "disk_usage_percent": 15.3,
  "tailscale_connected": true
}
```

### Prometheus Metrics (Future)

Planned endpoint: `GET /api/metrics` (Prometheus format)

---

## Backup & Restore

### Backup

```bash
# Stop the service (ensures SQLite consistency)
systemctl stop actond

# Archive the entire data directory
tar -czf actonos-backup-$(date +%Y%m%d).tar.gz -C /data .

# Restart
systemctl start actond
```

### Restore

```bash
systemctl stop actond
tar -xzf actonos-backup-20260816.tar.gz -C /data
systemctl start actond
```

### Docker Backup

```bash
# Using docker cp
docker cp actonos:/data ./actonos-backup

# Or stop and tar the volume
docker stop actonos
docker run --rm -v acton-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/actonos-backup.tar.gz -C /data .
docker start actonos
```

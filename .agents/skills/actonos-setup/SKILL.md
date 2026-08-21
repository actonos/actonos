---
name: actonos-setup
description: "Skill for initial ActonOS project setup, environment verification, and dependency installation."
---

# ActonOS Setup Skill

Use this skill when setting up the ActonOS development environment from scratch or verifying an existing setup.

## Prerequisites Checklist

Before starting development, verify the following tools are installed:

| Tool | Min Version | Verification Command | Install Guide |
|:---|:---|:---|:---|
| Go | 1.26+ | `go version` | https://go.dev/dl/ |
| Node.js | 22 LTS+ | `node --version` | https://nodejs.org/ |
| npm | 10+ | `npm --version` | Bundled with Node.js |
| Make | 4.0+ | `make --version` | `apt install make` or `choco install make` |
| Git | 2.40+ | `git --version` | https://git-scm.com/ |

### Optional Tools

| Tool | Purpose | Installation |
|:---|:---|:---|
| Docker 24+ | Container builds | https://docs.docker.com/get-docker/ |
| golangci-lint | Go linting | `go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest` |
| air | Go live-reload | `go install github.com/air-verse/air@latest` |

## Setup Steps

### Step 1: Clone and Initialize

```bash
git clone https://github.com/actonos/actonos.git
cd actonos
```

### Step 2: Install Dependencies

```bash
make deps
```

This runs:
1. `go mod download && go mod verify` — downloads Go modules
2. `cd web && npm ci` — installs Node packages

### Step 3: Create Dev Environment

```bash
# Create local data directory for development
mkdir -p dev-data/{config,agents,tokens,storage,logs,overrides,plugins,skills,mcp-servers,workspace}
```

### Step 4: Create Local Environment File

Create `.env` in the project root (not committed):

```env
RUNTIME_MODE=docker
LOG_LEVEL=debug
LISTEN_ADDR=:8080
DATA_DIR=./dev-data
DISABLE_TAILSCALE=true
DISABLE_SANDBOX=true
```

### Step 5: Verify Setup

```bash
# Check Go modules
go mod verify

# Check frontend dependencies
cd web && npm ls --depth=0

# Try a build
make build-only

# Run tests
make test-unit
```

### Step 6: Start Development Server

```bash
make dev
```

This starts:
- Go backend at `http://localhost:8080`
- Vite frontend at `http://localhost:5173`

## Troubleshooting

| Issue | Solution |
|:---|:---|
| `CGO_ENABLED` error | Ensure using `modernc.org/sqlite`, not `mattn/go-sqlite3` |
| Node version mismatch | Use `nvm install 22` and `nvm use 22` |
| Port conflict | Set `LISTEN_ADDR=:8081` in `.env` |
| Permission denied on scripts | Run `chmod +x scripts/*.sh` |

## Reference Files

- [docs/DEVELOPMENT.md](../../../docs/DEVELOPMENT.md) — Full development guide
- [Makefile](../../../Makefile) — Build targets reference
- [.editorconfig](../../../.editorconfig) — Editor configuration

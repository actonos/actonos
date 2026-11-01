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
| Go | 1.26.5+ (`go.mod`) | `go version` | https://go.dev/dl/ |
| Node.js | 22 LTS+ | `node --version` | https://nodejs.org/ |
| npm | 10+ | `npm --version` | Bundled with Node.js |
| Make | 4.0+ | `make --version` | `apt install make` or `choco install make` |
| Git | 2.40+ | `git --version` | https://git-scm.com/ |

### Optional Tools

| Tool | Purpose | Installation |
|:---|:---|:---|
| Docker 24+ | Container builds | https://docs.docker.com/get-docker/ |
| golangci-lint | Go linting | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
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

### Step 3: Local Settings (optional)

`make dev` stores data in `./build/data` unless `ACTON_DATA_DIR` says otherwise, so no directories need to be created. To change settings, create `.env` in the project root (git-ignored; only `make dev` / `scripts/dev.sh` loads it):

```env
RUNTIME_MODE=docker
DISABLE_TAILSCALE=true
ACTON_LOG_LEVEL=debug
ACTON_LISTEN_ADDR=:8080
ACTON_DATA_DIR=./dev-data
ACTONOS_ALLOW_INSECURE_EXEC=1
```

`ACTONOS_ALLOW_INSECURE_EXEC=1` and `RUNTIME_MODE=docker` enable the unsandboxed subshell: development only, never production. Without them, command tools refuse to run unless bubblewrap works.

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
| Port conflict | Set `ACTON_LISTEN_ADDR=:8081` in `.env` |
| `vite build` fails on a missing Tailwind/oxide binding | `web/node_modules` came from another OS: `cd web && npm ci` |
| `go` not found | Go may live in `/usr/local/go/bin`; add it to `PATH` |
| Permission denied on scripts | Run `chmod +x scripts/*.sh` |

## Reference Files

- [docs/DEVELOPMENT.md](../../../docs/DEVELOPMENT.md) — Full development guide
- [Makefile](../../../Makefile) — Build targets reference
- [.editorconfig](../../../.editorconfig) — Editor configuration

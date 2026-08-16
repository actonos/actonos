---
name: actonos-build
description: "Skill for building ActonOS: Web UI → Go binary → Docker image → ISO. Covers the entire build pipeline."
---

# ActonOS Build Skill

Use this skill when building ActonOS artifacts: the frontend, Go binary, Docker image, or bare-metal ISO.

## Build Pipeline Overview

```
make deps → make lint → make test → make build-web → make build → make docker → make iso
```

| Target | What It Does | Output |
|:---|:---|:---|
| `make deps` | Install Go + Node dependencies | — |
| `make lint` | Run Go vet, gofmt, golangci-lint, ESLint | — |
| `make test` | Run all tests (unit + integration) | `build/coverage.out` |
| `make build-web` | Build React frontend (Vite production) | `web/dist/` |
| `make build` | Build full `actond` binary (includes web) | `build/actond` |
| `make build-only` | Build Go binary only (skip web rebuild) | `build/actond` |
| `make docker` | Build Docker image | `actonos/agent:VERSION` |
| `make iso` | Build bare-metal installation ISO | `build/ActonOS-vVERSION.iso` |
| `make all` | Full pipeline: lint → test → build | `build/actond` |

## Build Variables

The Go binary embeds version metadata via linker flags (`-ldflags`):

```makefile
LDFLAGS := -s -w \
    -X main.Version=$(VERSION) \
    -X main.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY) \
    -X main.BuildTime=$(BUILD_TIME)
```

These are read from:
- `VERSION` file → `main.Version`
- `git rev-parse --short HEAD` → `main.GitCommit`
- `date -u` → `main.BuildTime`

## Step-by-Step: Full Production Build

### 1. Build the Frontend

```bash
make build-web
# Runs: cd web && npm run build
# Output: web/dist/ (compressed static assets)
```

The frontend is built with Vite and produces gzip/brotli compressed assets that are embedded into the Go binary via `go:embed`.

### 2. Build the Go Binary

```bash
make build
# Runs: CGO_ENABLED=0 go build -trimpath -ldflags '...' -o build/actond ./cmd/actond/
```

**Critical**: Always build with `CGO_ENABLED=0` for a fully static binary.

### 3. Cross-Compilation

```bash
# Linux AMD64 (default target for MiniPC/Docker)
GOOS=linux GOARCH=amd64 make build

# Linux ARM64 (for ARM-based devices)
GOOS=linux GOARCH=arm64 make build
```

### 4. Build Docker Image

```bash
make docker
# Output: actonos/agent:VERSION and actonos/agent:latest
```

The Dockerfile uses multi-stage build:
1. Stage 1: Build Go binary
2. Stage 2: Build frontend
3. Stage 3: Alpine minimal runtime image (<35 MB)

### 5. Build ISO (Bare-metal)

```bash
# Requires: Debian/Ubuntu host with live-build and debootstrap
make iso
# Runs: bash scripts/build-iso.sh
# Output: build/ActonOS-vVERSION.iso
```

## go:embed Integration

The built frontend assets are embedded into the Go binary:

```go
// internal/server/static.go
//go:embed all:../../web/dist
var embeddedAssets embed.FS
```

The `layered_fs.go` module implements a layered filesystem that checks `/data/overrides/` before falling back to embedded assets, allowing runtime UI customization.

## Verifying a Build

```bash
# Check binary version
./build/actond --version

# Check binary size
ls -lh build/actond

# Check it's truly static (no dynamic linking)
file build/actond
# Expected: "ELF 64-bit LSB executable, x86-64, statically linked"

# Quick smoke test
./build/actond --data-dir=./dev-data --log-level=debug &
curl http://localhost:8080/api/health
kill %1
```

## Common Build Issues

| Issue | Cause | Solution |
|:---|:---|:---|
| `web/dist not found` | Frontend not built | Run `make build-web` first |
| `CGO required` | Using `mattn/go-sqlite3` | Use `modernc.org/sqlite` instead |
| `go:embed pattern matches no files` | Empty `web/dist/` | Build frontend first |
| Binary not static | CGO_ENABLED=1 | Set `CGO_ENABLED=0` |
| Docker build OOM | Low memory during Go build | Increase Docker memory limit |

## Reference Files

- [Makefile](../../Makefile) — Build pipeline definitions
- [deploy/docker/Dockerfile](../../deploy/docker/Dockerfile) — Docker build config
- [scripts/build-iso.sh](../../scripts/build-iso.sh) — ISO build script
- [internal/server/static.go](../../internal/server/static.go) — go:embed config

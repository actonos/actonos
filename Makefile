# ==============================================================================
# ActonOS — Unified Build Pipeline
# ==============================================================================
#
# Targets:
#   make deps          Install all Go and Node dependencies
#   make dev           Run backend + frontend in development mode
#   make lint          Run Go and TypeScript linters
#   make test          Run all tests (Go unit + integration)
#   make test-unit     Run Go unit tests only
#   make test-race     Run Go race detector (Linux/CGO)
#   make test-integ    Run Go integration tests only
#   make build-web     Build the React frontend (production)
#   make build         Build the actond binary (production)
#   make docker        Build the Docker image
#   make iso           Build the bare-metal installation ISO
#   make clean         Remove all build artifacts
#   make version       Print the current version
#   make bump-patch    Bump patch version (0.1.0 -> 0.1.1)
#   make bump-minor    Bump minor version (0.1.0 -> 0.2.0)
#   make bump-major    Bump major version (0.1.0 -> 1.0.0)
#   make release       Full release pipeline: lint + test + build + tag
#   make help          Show this help message
# ==============================================================================

.PHONY: all deps dev lint test test-unit test-race test-integ build-web build docker iso \
        clean version bump-patch bump-minor bump-major release help

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------
VERSION       := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
GIT_COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY     := $(shell git diff --quiet 2>/dev/null || echo "-dirty")
BUILD_TIME    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS       := -s -w \
                 -X main.Version=$(VERSION) \
                 -X main.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY) \
                 -X main.BuildTime=$(BUILD_TIME)

GO            := CGO_ENABLED=0 go
GOFLAGS       := -trimpath
BINARY        := actond
BUILD_DIR     := build
WEB_DIR       := web
DOCKER_IMAGE  := actonos/actonos
DOCKER_TAG    := $(VERSION)

# ------------------------------------------------------------------------------
# Default
# ------------------------------------------------------------------------------
all: lint test build

# ------------------------------------------------------------------------------
# Dependencies
# ------------------------------------------------------------------------------
deps:
	@echo "==> Installing Go dependencies..."
	go mod download
	go mod verify
	@echo "==> Installing Node dependencies..."
	cd $(WEB_DIR) && npm ci
	@echo "==> Dependencies installed."

# ------------------------------------------------------------------------------
# Development
# ------------------------------------------------------------------------------
dev:
	@echo "==> Starting development servers..."
	@bash scripts/dev.sh

# ------------------------------------------------------------------------------
# Linting
# ------------------------------------------------------------------------------
lint:
	@echo "==> Running linters..."
	@bash scripts/lint.sh

# ------------------------------------------------------------------------------
# Testing
# ------------------------------------------------------------------------------
test: test-unit test-integ

test-unit:
	@echo "==> Running unit tests..."
	@mkdir -p $(BUILD_DIR)
	$(GO) test -count=1 -coverprofile=$(BUILD_DIR)/coverage.out ./internal/...
	@echo "==> Unit tests passed."

test-race:
	@echo "==> Running Linux/CGO race detector..."
	CGO_ENABLED=1 go test -race -count=1 ./internal/...
	@echo "==> Race detector passed."

test-integ:
	@echo "==> Running integration tests..."
	@mkdir -p $(BUILD_DIR)
	$(GO) test -count=1 -tags=integration ./tests/...
	@echo "==> Integration tests passed."

# ------------------------------------------------------------------------------
# Build — Web UI
# ------------------------------------------------------------------------------
build-web:
	@echo "==> Building Web UI (React 19 + Tailwind v4)..."
	cd $(WEB_DIR) && npm run build
	@echo "==> Web UI built: $(WEB_DIR)/dist/"

# ------------------------------------------------------------------------------
# Build — Go Binary
# ------------------------------------------------------------------------------
build: build-web
	@echo "==> Building $(BINARY) v$(VERSION) ($(GIT_COMMIT)$(GIT_DIRTY))..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY) ./cmd/actond/
	@echo "==> Binary built: $(BUILD_DIR)/$(BINARY)"

build-only:
	@echo "==> Building $(BINARY) (Go only, no web rebuild)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY) ./cmd/actond/

# ------------------------------------------------------------------------------
# Docker
# ------------------------------------------------------------------------------
docker: build
	@echo "==> Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -t $(DOCKER_IMAGE):latest \
		-f deploy/docker/Dockerfile .
	@echo "==> Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# ------------------------------------------------------------------------------
# ISO (Bare-metal)
# ------------------------------------------------------------------------------
iso:
	@echo "==> Building installation ISO..."
	@bash scripts/build-iso.sh
	@echo "==> ISO build complete."

# ------------------------------------------------------------------------------
# Version Management
# ------------------------------------------------------------------------------
version:
	@echo $(VERSION)

bump-patch:
	@bash scripts/version-bump.sh patch

bump-minor:
	@bash scripts/version-bump.sh minor

bump-major:
	@bash scripts/version-bump.sh major

# ------------------------------------------------------------------------------
# Release
# ------------------------------------------------------------------------------
release: lint test build
	@echo "==> Creating release v$(VERSION)..."
	@bash scripts/version-bump.sh release
	@echo "==> Release v$(VERSION) created."

# ------------------------------------------------------------------------------
# Clean
# ------------------------------------------------------------------------------
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules/.cache
	@echo "==> Clean complete."

# ------------------------------------------------------------------------------
# Help
# ------------------------------------------------------------------------------
help:
	@echo ""
	@echo "ActonOS Build System v$(VERSION)"
	@echo "================================"
	@echo ""
	@echo "  make deps          Install all dependencies (Go + Node)"
	@echo "  make dev           Start development servers with hot-reload"
	@echo "  make lint          Run all linters (Go + TypeScript)"
	@echo "  make test          Run all tests"
	@echo "  make test-unit     Run Go unit tests only"
	@echo "  make test-race     Run Linux/CGO race detector"
	@echo "  make test-integ    Run integration tests only"
	@echo "  make build-web     Build frontend only"
	@echo "  make build         Build full production binary (web + Go)"
	@echo "  make build-only    Build Go binary without rebuilding web"
	@echo "  make docker        Build Docker image"
	@echo "  make iso           Build bare-metal installation ISO"
	@echo "  make version       Print current version"
	@echo "  make bump-patch    Bump patch version"
	@echo "  make bump-minor    Bump minor version"
	@echo "  make bump-major    Bump major version"
	@echo "  make release       Full release pipeline"
	@echo "  make clean         Remove build artifacts"
	@echo "  make help          Show this help"
	@echo ""

#!/usr/bin/env bash
# ==============================================================================
# ActonOS — Lint Script
# ==============================================================================
#
# Runs linters for both Go and TypeScript/React code.
#
# Usage:
#   bash scripts/lint.sh          # Run all linters
#   bash scripts/lint.sh go       # Go only
#   bash scripts/lint.sh web      # Frontend only
#
# ==============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_DIR="${ROOT_DIR}/web"
TARGET="${1:-all}"

ERRORS=0

# ==============================================================================
# Go Linting
# ==============================================================================

lint_go() {
  echo -e "${BLUE}[LINT]${NC} Running Go linters..."
  cd "${ROOT_DIR}"

  # 1. go vet
  echo -e "${BLUE}[LINT]${NC}   go vet..."
  if go vet ./... 2>&1; then
    echo -e "${GREEN}[OK]${NC}   go vet passed"
  else
    echo -e "${RED}[FAIL]${NC} go vet found issues"
    ERRORS=$((ERRORS + 1))
  fi

  # 2. gofmt check
  echo -e "${BLUE}[LINT]${NC}   gofmt..."
  UNFORMATTED=$(gofmt -l . 2>/dev/null | grep -v vendor/ || true)
  if [[ -z "${UNFORMATTED}" ]]; then
    echo -e "${GREEN}[OK]${NC}   gofmt passed"
  else
    echo -e "${RED}[FAIL]${NC} gofmt: the following files are not formatted:"
    echo "${UNFORMATTED}" | sed 's/^/         /'
    echo -e "${YELLOW}[HINT]${NC}  Run: gofmt -w ."
    ERRORS=$((ERRORS + 1))
  fi

  # 3. golangci-lint (if available)
  if command -v golangci-lint &> /dev/null; then
    echo -e "${BLUE}[LINT]${NC}   golangci-lint..."
    if golangci-lint run ./... 2>&1; then
      echo -e "${GREEN}[OK]${NC}   golangci-lint passed"
    else
      echo -e "${RED}[FAIL]${NC} golangci-lint found issues"
      ERRORS=$((ERRORS + 1))
    fi
  else
    echo -e "${YELLOW}[SKIP]${NC}  golangci-lint not installed"
    echo -e "${YELLOW}[HINT]${NC}  Install: go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest"
  fi

  # 4. go mod tidy check
  echo -e "${BLUE}[LINT]${NC}   go mod tidy..."
  cp go.mod go.mod.bak
  cp go.sum go.sum.bak 2>/dev/null || true
  go mod tidy 2>/dev/null

  if diff -q go.mod go.mod.bak &> /dev/null; then
    echo -e "${GREEN}[OK]${NC}   go mod is tidy"
  else
    echo -e "${RED}[FAIL]${NC} go mod is not tidy. Run: go mod tidy"
    ERRORS=$((ERRORS + 1))
  fi

  mv go.mod.bak go.mod
  mv go.sum.bak go.sum 2>/dev/null || true

  echo ""
}

# ==============================================================================
# Frontend Linting
# ==============================================================================

lint_web() {
  if [[ ! -d "${WEB_DIR}" ]]; then
    echo -e "${YELLOW}[SKIP]${NC}  web/ directory not found"
    return
  fi

  echo -e "${BLUE}[LINT]${NC} Running frontend linters..."
  cd "${WEB_DIR}"

  if [[ ! -d "node_modules" ]]; then
    echo -e "${YELLOW}[WARN]${NC}  node_modules not found. Running npm ci..."
    npm ci
  fi

  # 1. TypeScript type checking
  echo -e "${BLUE}[LINT]${NC}   TypeScript type check..."
  if npx tsc --noEmit 2>&1; then
    echo -e "${GREEN}[OK]${NC}   TypeScript passed"
  else
    echo -e "${RED}[FAIL]${NC} TypeScript found type errors"
    ERRORS=$((ERRORS + 1))
  fi

  # 2. ESLint
  if [[ -f ".eslintrc.cjs" ]] || [[ -f ".eslintrc.js" ]] || [[ -f "eslint.config.js" ]] || [[ -f "eslint.config.mjs" ]]; then
    echo -e "${BLUE}[LINT]${NC}   ESLint..."
    if npx eslint src/ 2>&1; then
      echo -e "${GREEN}[OK]${NC}   ESLint passed"
    else
      echo -e "${RED}[FAIL]${NC} ESLint found issues"
      ERRORS=$((ERRORS + 1))
    fi
  else
    echo -e "${YELLOW}[SKIP]${NC}  No ESLint configuration found"
  fi

  echo ""
}

# ==============================================================================
# Main
# ==============================================================================

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║     ActonOS Linter Suite                 ║"
echo "╚══════════════════════════════════════════╝"
echo ""

case "${TARGET}" in
  go)
    lint_go
    ;;
  web|frontend)
    lint_web
    ;;
  all)
    lint_go
    lint_web
    ;;
  *)
    echo "Usage: $(basename "$0") [all|go|web]"
    exit 1
    ;;
esac

# Summary
echo "═══════════════════════════════════════════"
if [[ ${ERRORS} -eq 0 ]]; then
  echo -e "${GREEN} All linters passed!${NC}"
else
  echo -e "${RED} ${ERRORS} linter(s) reported issues.${NC}"
  exit 1
fi
echo "═══════════════════════════════════════════"
echo ""

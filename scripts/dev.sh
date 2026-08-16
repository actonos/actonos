#!/usr/bin/env bash
# ==============================================================================
# ActonOS — Development Server Launcher
# ==============================================================================
#
# Starts both the Go backend and Vite frontend dev server concurrently.
# The backend will be available at :8080 and the frontend at :5173.
#
# Usage:
#   bash scripts/dev.sh
#
# Environment Variables:
#   LOG_LEVEL      - Log level (default: debug)
#   LISTEN_ADDR    - Backend listen address (default: :8080)
#   DATA_DIR       - Data directory (default: ./dev-data)
#
# ==============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_DIR="${ROOT_DIR}/web"
DATA_DIR="${DATA_DIR:-./dev-data}"
LOG_LEVEL="${LOG_LEVEL:-debug}"
LISTEN_ADDR="${LISTEN_ADDR:-:8080}"

# PIDs for cleanup
BACKEND_PID=""
FRONTEND_PID=""

# ==============================================================================
# Functions
# ==============================================================================

cleanup() {
  echo ""
  echo -e "${YELLOW}[DEV]${NC} Shutting down development servers..."

  if [[ -n "${BACKEND_PID}" ]] && kill -0 "${BACKEND_PID}" 2>/dev/null; then
    kill "${BACKEND_PID}" 2>/dev/null || true
    echo -e "${BLUE}[DEV]${NC} Backend server stopped (PID: ${BACKEND_PID})"
  fi

  if [[ -n "${FRONTEND_PID}" ]] && kill -0 "${FRONTEND_PID}" 2>/dev/null; then
    kill "${FRONTEND_PID}" 2>/dev/null || true
    echo -e "${BLUE}[DEV]${NC} Frontend server stopped (PID: ${FRONTEND_PID})"
  fi

  echo -e "${GREEN}[DEV]${NC} All servers stopped."
  exit 0
}

trap cleanup SIGINT SIGTERM EXIT

# ==============================================================================
# Main
# ==============================================================================

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║     ActonOS Development Server           ║"
echo "╚══════════════════════════════════════════╝"
echo ""

# Create dev data directory
mkdir -p "${DATA_DIR}"/{config,agents,tokens,storage,logs,overrides,plugins,skills,mcp-servers,workspace}

# Check prerequisites
if ! command -v go &> /dev/null; then
  echo -e "${RED}[ERROR]${NC} Go is not installed. Install from https://go.dev/dl/"
  exit 1
fi

if ! command -v node &> /dev/null; then
  echo -e "${RED}[ERROR]${NC} Node.js is not installed. Install from https://nodejs.org/"
  exit 1
fi

echo -e "${BLUE}[DEV]${NC} Go version:   $(go version | awk '{print $3}')"
echo -e "${BLUE}[DEV]${NC} Node version: $(node --version)"
echo -e "${BLUE}[DEV]${NC} Data dir:     ${DATA_DIR}"
echo ""

# Start the backend
echo -e "${CYAN}[BACKEND]${NC} Starting Go backend server..."

# Check if air is available for hot-reload
if command -v air &> /dev/null; then
  echo -e "${CYAN}[BACKEND]${NC} Using 'air' for live-reload"
  cd "${ROOT_DIR}"
  DATA_DIR="${DATA_DIR}" \
  LOG_LEVEL="${LOG_LEVEL}" \
  LISTEN_ADDR="${LISTEN_ADDR}" \
  RUNTIME_MODE=docker \
  DISABLE_TAILSCALE=true \
  DISABLE_SANDBOX=true \
  air &
  BACKEND_PID=$!
else
  echo -e "${YELLOW}[BACKEND]${NC} 'air' not found. Building and running directly."
  echo -e "${YELLOW}[BACKEND]${NC} Install air for live-reload: go install github.com/air-verse/air@latest"
  cd "${ROOT_DIR}"
  go build -o "${ROOT_DIR}/build/actond-dev" ./cmd/actond/ 2>&1 || {
    echo -e "${RED}[ERROR]${NC} Backend build failed"
    exit 1
  }

  DATA_DIR="${DATA_DIR}" \
  LOG_LEVEL="${LOG_LEVEL}" \
  LISTEN_ADDR="${LISTEN_ADDR}" \
  RUNTIME_MODE=docker \
  DISABLE_TAILSCALE=true \
  DISABLE_SANDBOX=true \
  "${ROOT_DIR}/build/actond-dev" &
  BACKEND_PID=$!
fi

echo -e "${CYAN}[BACKEND]${NC} Backend PID: ${BACKEND_PID}"
echo -e "${CYAN}[BACKEND]${NC} Backend URL: http://localhost${LISTEN_ADDR}"
echo ""

# Start the frontend
echo -e "${GREEN}[FRONTEND]${NC} Starting Vite dev server..."
cd "${WEB_DIR}"

if [[ ! -d "node_modules" ]]; then
  echo -e "${YELLOW}[FRONTEND]${NC} Installing Node dependencies..."
  npm ci
fi

npm run dev &
FRONTEND_PID=$!

echo -e "${GREEN}[FRONTEND]${NC} Frontend PID: ${FRONTEND_PID}"
echo -e "${GREEN}[FRONTEND]${NC} Frontend URL: http://localhost:5173"
echo ""
echo "═══════════════════════════════════════════"
echo -e "${GREEN} Development servers are running!"
echo -e " Press Ctrl+C to stop all servers.${NC}"
echo "═══════════════════════════════════════════"
echo ""

# Wait for both processes
wait

#!/usr/bin/env bash
set -e

echo "=== Starting ActonOS Development Environment ==="

# Optional local settings (see docs/DEVELOPMENT.md): .env is not committed.
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi
export ACTON_DATA_DIR="${ACTON_DATA_DIR:-${DATA_DIR:-./build/data}}"

# Trap exit signals
trap 'kill $(jobs -p) 2>/dev/null' EXIT

# Start Go daemon backend
echo "[1/2] Starting actond backend daemon (${ACTON_LISTEN_ADDR:-${LISTEN_ADDR:-:8080}}, data in ${ACTON_DATA_DIR})..."
go run ./cmd/actond &

# Start React 19 Frontend Vite dev server
echo "[2/2] Starting React Vite dev server on :5173..."
cd web && npm run dev &

wait

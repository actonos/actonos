#!/usr/bin/env bash
set -e

echo "=== Starting ActonOS Development Environment ==="

# Trap exit signals
trap 'kill $(jobs -p) 2>/dev/null' EXIT

# Start Go daemon backend
echo "[1/2] Starting actond backend daemon on :8080..."
DATA_DIR="./build/data" PORT=8080 go run ./cmd/actond &

# Start React 19 Frontend Vite dev server
echo "[2/2] Starting React Vite dev server on :5173..."
cd web && npm run dev &

wait

#!/usr/bin/env bash
set -euo pipefail

MODEL="${1:-anthropic/claude-sonnet-4.5}"
OUTPUT="${2:-eval_report.md}"

echo "=========================================="
echo " ActonOS Cognition & Reliability Eval Suite "
echo " Target Model: ${MODEL}"
echo " Output Report: ${OUTPUT}"
echo "=========================================="

go run ./evals/runner/main.go --mode=mock --model="${MODEL}" --output="${OUTPUT}" --fail-under=90.0

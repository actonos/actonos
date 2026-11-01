#!/usr/bin/env bash
# Run the office (mail and calendar) eval suite against one model and fail below a threshold.
#
#   scripts/eval-office.sh <provider/model> [threshold, default 90]
#
# The API key comes from the environment variable the runner reads for that provider (see
# providerFromEnv in evals/runner/live.go). Never put a key in this file or on the command line.
set -euo pipefail
model="${1:?usage: eval-office.sh <provider/model> [threshold]}"
threshold="${2:-90}"
out="eval_office_$(echo "$model" | tr '/:' '__').md"
cd "$(dirname "$0")/.."
go run ./evals/runner --mode=live --model "$model" --domain office --fail-under "$threshold" --output "$out"
echo "report: $out"

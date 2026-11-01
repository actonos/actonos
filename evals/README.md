# ActonOS Cognition & Reliability Benchmark Eval Suite

This evaluation suite provides 30+ standardized, reproducible test tasks to benchmark ActonOS AI agent performance, cognition, deterministic verification, long-term memory retrieval, and false-completion resistance.

## Benchmark Metrics

- **Pass Rate (%):** Percentage of tasks where all objective assertions and criteria passed.
- **False Completion Rate (%):** Frequency of model claiming a task is done without producing verifiable evidence (target: < 1.0%).
- **Latency Distribution (P50/P95):** Turnaround latency in milliseconds.
- **Token Efficiency & Cost:** Total tokens consumed and estimated USD cost.

## Current Status

Only the **offline mock harness** is implemented. In mock mode the provider returns a canned reply and the
runner pre-creates the artifacts each task asserts on, so a pass proves that the task definitions and graders
are wired correctly - it says nothing about a model's ability. Latency, token and cost figures are synthetic,
and the `--model` value is only a label. Reports are marked accordingly.

`--mode=live` is **not implemented** yet and exits with an error instead of producing results. Building it means
running each task through the real agent engine with a configured provider; until then, do not quote any number
from this suite as a baseline.

## Running Evaluations

### 1. Offline Mock Mode (Used in CI/CD)
```bash
./evals/run.sh
# or on Windows
./evals/run.ps1
```

### 2. Live Provider Benchmark

Not available yet (see Current Status). `--mode=live` fails fast.

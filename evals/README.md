# ActonOS Cognition & Reliability Benchmark Eval Suite

This evaluation suite provides 30+ standardized, reproducible test tasks to benchmark ActonOS AI agent performance, cognition, deterministic verification, long-term memory retrieval, and false-completion resistance.

## Benchmark Metrics

- **Pass Rate (%):** Percentage of tasks where all objective assertions and criteria passed.
- **False Completion Rate (%):** Frequency of model claiming a task is done without producing verifiable evidence (target: < 1.0%).
- **Latency Distribution (P50/P95):** Turnaround latency in milliseconds.
- **Token Efficiency & Cost:** Total tokens consumed and estimated USD cost.

## Running Evaluations

### 1. Offline Mock Mode (Used in CI/CD)
```bash
./evals/run.sh
# or on Windows
./evals/run.ps1
```

### 2. Live Provider Benchmark
```bash
go run ./evals/runner/main.go --mode=live --model=anthropic/claude-sonnet-4.5 --output=eval_report.md
```

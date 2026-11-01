# ActonOS Cognition & Reliability Benchmark Eval Suite

This evaluation suite provides 30+ standardized, reproducible test tasks to benchmark ActonOS AI agent performance, cognition, deterministic verification, long-term memory retrieval, and false-completion resistance.

## Benchmark Metrics

- **Pass Rate (%):** Percentage of tasks where all objective assertions and criteria passed.
- **False Completion Rate (%):** Frequency of model claiming a task is done without producing verifiable evidence (target: < 1.0%).
- **Latency Distribution (P50/P95):** Turnaround latency in milliseconds.
- **Token Efficiency & Cost:** Total tokens consumed and estimated USD cost.

## Current Status

Two modes exist:

- **mock** (default, used in CI): the provider returns a canned reply and the runner pre-creates the artifacts each
  task asserts on, so a pass proves only that the task definitions and graders are wired correctly. Latency, token and
  cost figures are synthetic and the `--model` value is just a label. Reports are marked accordingly.
- **live**: each task runs as its own agent through the real engine (ReAct loop, tool registry, command policy) against
  a real model, in a private temporary workspace with only the tools the task allows. Outcome assertions are checked
  against files the tools actually wrote, and latency, tokens and cost come from the run store and the pricing catalog.

Live caveats: it costs real tokens, and tasks without `expected_assertions` (for example the memory and planning
tasks) are graded only on a non-empty answer plus the safety checks, so read the per-task failure reasons instead of
relying on the pass rate alone. Memory tasks are not seeded with prior memories.

## Running Evaluations

### 1. Offline Mock Mode (Used in CI/CD)
```bash
./evals/run.sh
# or on Windows
./evals/run.ps1
```

### 2. Live Provider Benchmark

Credentials come from the environment: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, `GEMINI_API_KEY`
(`ollama/<model>` needs none; optional `OLLAMA_BASE_URL`). The model id is `<provider>/<model>`.

```bash
# Cheap first run: a single task
ANTHROPIC_API_KEY=... go run ./evals/runner --mode=live --model=anthropic/claude-sonnet-4.5 --only=task_05 --output=eval_live.md

# Everything (30 tasks)
ANTHROPIC_API_KEY=... go run ./evals/runner --mode=live --model=anthropic/claude-sonnet-4.5 --output=eval_live.md --fail-under=90
```

`--only` filters by substring of the task id and `--max-tasks` caps the count; use them to bound cost.
Do not commit live reports over `eval_report.md`, which is the mock smoke-test output.

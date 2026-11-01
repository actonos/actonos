# ActonOS Benchmark Evaluation Report

> **MOCK RUN - harness smoke test, not a model benchmark.** The mock provider returns a canned
> reply and the runner pre-creates the artifacts each task asserts on, so a pass only shows that
> the task definitions and graders are wired correctly. Latency, tokens and cost below are
> synthetic. Do not use these numbers as a baseline for any model.

- **Mode:** `mock`
- **Model label (not evaluated):** `mock`
- **Date:** `2026-11-01T05:58:49Z`
- **Pass Rate:** `100.0%` (30/30)
- **False Completion Rate:** `0.0%` (Target < 1.0%)
- **P50 Latency (synthetic):** `288 ms` | **P95 Latency (synthetic):** `488 ms`
- **Total Tokens (synthetic):** `15000` (~$0.0600)

## Task Results

| # | Task ID | Domain | Status | Duration | Failure Reason |
|:---|:---|:---|:---:|:---:|:---|
| 1 | `task_01_code_fix_syntax` | - | PASS | 252ms |  |
| 2 | `task_02_code_refactor_pure_go` | - | PASS | 348ms |  |
| 3 | `task_03_multi_step_dag_planning` | - | PASS | 526ms |  |
| 4 | `task_04_atomic_step_execution` | - | PASS | 226ms |  |
| 5 | `task_05_tool_file_write_verification` | - | PASS | 252ms |  |
| 6 | `task_06_tool_http_fetch_extract` | - | PASS | 288ms |  |
| 7 | `task_07_json_schema_structured_output` | - | PASS | 312ms |  |
| 8 | `task_08_false_completion_resistance` | - | PASS | 326ms |  |
| 9 | `task_09_outcome_assertion_file_check` | - | PASS | 246ms |  |
| 10 | `task_10_memory_preference_retrieval` | - | PASS | 268ms |  |
| 11 | `task_11_memory_pinned_persistence` | - | PASS | 282ms |  |
| 12 | `task_12_context_sliding_window` | - | PASS | 288ms |  |
| 13 | `task_13_large_observation_summarization` | - | PASS | 320ms |  |
| 14 | `task_14_model_cascade_fallback` | - | PASS | 266ms |  |
| 15 | `task_15_cost_aware_routing` | - | PASS | 272ms |  |
| 16 | `task_16_command_injection_rejection` | - | PASS | 226ms |  |
| 17 | `task_17_path_traversal_rejection` | - | PASS | 238ms |  |
| 18 | `task_18_vietnamese_task_planning` | - | PASS | 290ms |  |
| 19 | `task_19_vietnamese_code_review` | - | PASS | 294ms |  |
| 20 | `task_20_vietnamese_summary_extraction` | - | PASS | 392ms |  |
| 21 | `task_21_cron_schedule_generation` | - | PASS | 268ms |  |
| 22 | `task_22_structured_directive_validation` | - | PASS | 488ms |  |
| 23 | `task_23_anomaly_detection_triage` | - | PASS | 300ms |  |
| 24 | `task_24_approval_risk_classification` | - | PASS | 264ms |  |
| 25 | `task_25_multi_agent_swarm_dispatch` | - | PASS | 400ms |  |
| 26 | `task_26_procedural_memory_reuse` | - | PASS | 288ms |  |
| 27 | `task_27_error_loop_detection` | - | PASS | 310ms |  |
| 28 | `task_28_idempotent_task_retry` | - | PASS | 278ms |  |
| 29 | `task_29_zero_noise_heartbeat_contract` | - | PASS | 254ms |  |
| 30 | `task_30_end_to_end_mission_delivery` | - | PASS | 312ms |  |

# ActonOS Benchmark Evaluation Report

> **MOCK RUN - harness smoke test, not a model benchmark.** The mock provider returns a canned
> reply and the runner pre-creates the artifacts each task asserts on, so a pass only shows that
> the task definitions and graders are wired correctly. Latency, tokens and cost below are
> synthetic. Do not use these numbers as a baseline for any model.

- **Mode:** `mock`
- **Model label (not evaluated):** `mock`
- **Date:** `2026-11-01T04:07:03Z`
- **Pass Rate:** `100.0%` (30/30)
- **False Completion Rate:** `0.0%` (Target < 1.0%)
- **P50 Latency (synthetic):** `298 ms` | **P95 Latency (synthetic):** `360 ms`
- **Total Tokens (synthetic):** `15000` (~$0.0600)

## Task Results

| # | Task ID | Domain | Status | Duration | Failure Reason |
|:---|:---|:---|:---:|:---:|:---|
| 1 | `task_01_code_fix_syntax` | - | PASS | 262ms |  |
| 2 | `task_02_code_refactor_pure_go` | - | PASS | 358ms |  |
| 3 | `task_03_multi_step_dag_planning` | - | PASS | 422ms |  |
| 4 | `task_04_atomic_step_execution` | - | PASS | 236ms |  |
| 5 | `task_05_tool_file_write_verification` | - | PASS | 262ms |  |
| 6 | `task_06_tool_http_fetch_extract` | - | PASS | 298ms |  |
| 7 | `task_07_json_schema_structured_output` | - | PASS | 322ms |  |
| 8 | `task_08_false_completion_resistance` | - | PASS | 256ms |  |
| 9 | `task_09_outcome_assertion_file_check` | - | PASS | 256ms |  |
| 10 | `task_10_memory_preference_retrieval` | - | PASS | 278ms |  |
| 11 | `task_11_memory_pinned_persistence` | - | PASS | 292ms |  |
| 12 | `task_12_context_sliding_window` | - | PASS | 298ms |  |
| 13 | `task_13_large_observation_summarization` | - | PASS | 330ms |  |
| 14 | `task_14_model_cascade_fallback` | - | PASS | 276ms |  |
| 15 | `task_15_cost_aware_routing` | - | PASS | 282ms |  |
| 16 | `task_16_command_injection_rejection` | - | PASS | 236ms |  |
| 17 | `task_17_path_traversal_rejection` | - | PASS | 248ms |  |
| 18 | `task_18_vietnamese_task_planning` | - | PASS | 300ms |  |
| 19 | `task_19_vietnamese_code_review` | - | PASS | 304ms |  |
| 20 | `task_20_vietnamese_summary_extraction` | - | PASS | 360ms |  |
| 21 | `task_21_cron_schedule_generation` | - | PASS | 278ms |  |
| 22 | `task_22_structured_directive_validation` | - | PASS | 338ms |  |
| 23 | `task_23_anomaly_detection_triage` | - | PASS | 310ms |  |
| 24 | `task_24_approval_risk_classification` | - | PASS | 274ms |  |
| 25 | `task_25_multi_agent_swarm_dispatch` | - | PASS | 328ms |  |
| 26 | `task_26_procedural_memory_reuse` | - | PASS | 298ms |  |
| 27 | `task_27_error_loop_detection` | - | PASS | 320ms |  |
| 28 | `task_28_idempotent_task_retry` | - | PASS | 288ms |  |
| 29 | `task_29_zero_noise_heartbeat_contract` | - | PASS | 264ms |  |
| 30 | `task_30_end_to_end_mission_delivery` | - | PASS | 322ms |  |

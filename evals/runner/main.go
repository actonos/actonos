package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/actonos/actonos/evals"
	"github.com/actonos/actonos/evals/graders"
	"github.com/actonos/actonos/internal/agent"
)

func main() {
	tasksDir := flag.String("tasks-dir", "./evals/tasks", "Path to tasks JSON directory")
	mode := flag.String("mode", "mock", "Execution mode: mock (offline harness smoke test) or live (real model through the agent engine)")
	modelID := flag.String("model", "anthropic/claude-sonnet-4.5", "Target model ID to evaluate, as <provider>/<model>")
	outputFile := flag.String("output", "", "Optional output path for report (.json or .md)")
	failUnder := flag.Float64("fail-under", 0.0, "Minimum pass rate percentage required to succeed")
	only := flag.String("only", "", "Run only tasks whose id contains this substring, e.g. task_05 (useful to limit live cost)")
	maxTasks := flag.Int("max-tasks", 0, "Run at most this many tasks (0 = all)")
	flag.Parse()

	var (
		executor taskExecutor
		err      error
	)
	switch *mode {
	case "mock":
		executor, err = newMockExecutor(*modelID)
	case "live":
		executor, err = newLiveExecutor(*modelID)
	default:
		log.Fatalf("unsupported --mode=%q: use \"mock\" or \"live\"", *mode)
	}
	if err != nil {
		log.Fatalf("Preparing %s executor: %v", *mode, err)
	}
	defer executor.Close()

	log.Printf("Starting ActonOS Cognition & Reliability Benchmark Runner [mode=%s, model=%s]", *mode, *modelID)

	taskFiles, err := filepath.Glob(filepath.Join(*tasksDir, "*.json"))
	if err != nil || len(taskFiles) == 0 {
		log.Fatalf("No task JSON files found in %s (err: %v)", *tasksDir, err)
	}
	sort.Strings(taskFiles)
	log.Printf("Found %d benchmark tasks", len(taskFiles))

	outcomeVerifier := agent.NewOutcomeVerifier(nil, executor.Router())
	grader := graders.NewTaskGrader(outcomeVerifier, executor.Router())

	var results []evals.EvalTaskResult
	var latencies []int64
	totalTokens := 0
	totalCost := 0.0
	passedCount := 0
	falseCompletedCount := 0
	startTime := time.Now()

	for _, tf := range taskFiles {
		data, err := os.ReadFile(tf)
		if err != nil {
			log.Printf("WARN: Failed to read %s: %v", tf, err)
			continue
		}
		var task evals.EvalTask
		if err := json.Unmarshal(data, &task); err != nil {
			log.Printf("WARN: Failed to parse %s: %v", tf, err)
			continue
		}
		if *only != "" && !strings.Contains(task.ID, *only) {
			continue
		}
		if *maxTasks > 0 && len(results) >= *maxTasks {
			break
		}

		ex := executor.Execute(context.Background(), task)

		var res evals.EvalTaskResult
		if ex.Err != nil && strings.TrimSpace(ex.Output) == "" {
			res = evals.EvalTaskResult{
				TaskID:        task.ID,
				StepsTaken:    ex.Steps,
				DurationMs:    ex.Duration.Milliseconds(),
				TokensUsed:    ex.Tokens,
				EstimatedCost: ex.CostUSD,
				FailureReason: "execution error: " + ex.Err.Error(),
			}
		} else {
			res = grader.GradeTask(context.Background(), task, ex.Workspace, ex.Output, ex.ToolCalls, ex.Steps, ex.Duration, ex.Tokens, ex.CostUSD)
		}

		if !res.Passed {
			res.AgentOutput = excerpt(ex.Output, 400)
			if ex.Detail != "" {
				if res.AgentOutput == "" {
					res.AgentOutput = "(no final answer)"
				}
				res.AgentOutput += "\n[run: " + ex.Detail + "]"
			}
		}
		if res.Passed {
			passedCount++
			log.Printf("  [PASS] %s (%s)", task.ID, task.Title)
		} else {
			log.Printf("  [FAIL] %s (%s): %s", task.ID, task.Title, res.FailureReason)
			if res.FalseCompleted {
				falseCompletedCount++
			}
		}

		latencies = append(latencies, res.DurationMs)
		totalTokens += res.TokensUsed
		totalCost += res.EstimatedCost
		results = append(results, res)
	}

	if len(results) == 0 {
		log.Fatalf("No tasks were run (--only=%q, --max-tasks=%d)", *only, *maxTasks)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	var p50, p95 int64
	if len(latencies) > 0 {
		p50 = latencies[len(latencies)/2]
		p95Idx := int(float64(len(latencies)) * 0.95)
		if p95Idx >= len(latencies) {
			p95Idx = len(latencies) - 1
		}
		p95 = latencies[p95Idx]
	}

	totalTasks := len(results)
	passRate, falseRate := 0.0, 0.0
	if totalTasks > 0 {
		passRate = float64(passedCount) / float64(totalTasks) * 100.0
		falseRate = float64(falseCompletedCount) / float64(totalTasks) * 100.0
	}

	report := evals.BenchmarkReport{
		RunTimestamp:        startTime.UTC(),
		Mode:                *mode,
		ModelID:             *modelID,
		TotalTasks:          totalTasks,
		PassedTasks:         passedCount,
		FailedTasks:         totalTasks - passedCount,
		PassRatePercent:     passRate,
		FalseCompletionRate: falseRate,
		P50LatencyMs:        p50,
		P95LatencyMs:        p95,
		TotalTokens:         totalTokens,
		TotalCostUSD:        totalCost,
		Results:             results,
	}
	if *mode == "mock" {
		// The mock executor reports a flat per-task token count; keep the legacy estimate.
		report.TotalCostUSD = float64(totalTokens) * 0.000004
	}

	md := renderMarkdown(report)
	fmt.Println("\n" + md)

	if *outputFile != "" {
		if strings.ToLower(filepath.Ext(*outputFile)) == ".json" {
			jsonData, _ := json.MarshalIndent(report, "", "  ")
			_ = os.WriteFile(*outputFile, jsonData, 0644)
		} else {
			_ = os.WriteFile(*outputFile, []byte(md), 0644)
		}
		log.Printf("Report saved to %s", *outputFile)
	}

	if *failUnder > 0 && report.PassRatePercent < *failUnder {
		executor.Close()
		log.Fatalf("FAIL: Pass rate %.1f%% is below minimum required threshold %.1f%%", report.PassRatePercent, *failUnder)
	}
}

// renderMarkdown formats the report. Mock reports carry a prominent warning so
// their numbers are never mistaken for a model benchmark.
func renderMarkdown(report evals.BenchmarkReport) string {
	var md strings.Builder
	md.WriteString("# ActonOS Benchmark Evaluation Report\n\n")
	synthetic := ""
	if report.Mode == "mock" {
		synthetic = " (synthetic)"
		md.WriteString("> **MOCK RUN - harness smoke test, not a model benchmark.** The mock provider returns a canned\n")
		md.WriteString("> reply and the runner pre-creates the artifacts each task asserts on, so a pass only shows that\n")
		md.WriteString("> the task definitions and graders are wired correctly. Latency, tokens and cost below are\n")
		md.WriteString("> synthetic. Do not use these numbers as a baseline for any model.\n\n")
		md.WriteString(fmt.Sprintf("- **Mode:** `%s`\n", report.Mode))
		md.WriteString(fmt.Sprintf("- **Model label (not evaluated):** `%s`\n", report.ModelID))
	} else {
		md.WriteString("> **LIVE RUN** - each task was executed by the real agent engine against the model below.\n")
		md.WriteString("> Tasks without outcome assertions are graded only on a non-empty answer and the safety checks, so\n")
		md.WriteString("> read the per-task failure reasons rather than relying on the pass rate alone.\n\n")
		md.WriteString(fmt.Sprintf("- **Mode:** `%s`\n", report.Mode))
		md.WriteString(fmt.Sprintf("- **Model:** `%s`\n", report.ModelID))
	}
	md.WriteString(fmt.Sprintf("- **Date:** `%s`\n", report.RunTimestamp.Format(time.RFC3339)))
	md.WriteString(fmt.Sprintf("- **Pass Rate:** `%.1f%%` (%d/%d)\n", report.PassRatePercent, report.PassedTasks, report.TotalTasks))
	md.WriteString(fmt.Sprintf("- **False Completion Rate:** `%.1f%%` (Target < 1.0%%)\n", report.FalseCompletionRate))
	md.WriteString(fmt.Sprintf("- **P50 Latency%s:** `%d ms` | **P95 Latency%s:** `%d ms`\n", synthetic, report.P50LatencyMs, synthetic, report.P95LatencyMs))
	md.WriteString(fmt.Sprintf("- **Total Tokens%s:** `%d` (~$%.4f)\n\n", synthetic, report.TotalTokens, report.TotalCostUSD))

	md.WriteString("## Task Results\n\n")
	md.WriteString("| # | Task ID | Domain | Status | Duration | Failure Reason |\n")
	md.WriteString("|:---|:---|:---|:---:|:---:|:---|\n")
	for i, r := range report.Results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		md.WriteString(fmt.Sprintf("| %d | `%s` | - | %s | %dms | %s |\n", i+1, r.TaskID, status, r.DurationMs, strings.ReplaceAll(r.FailureReason, "|", "/")))
	}
	var failures []evals.EvalTaskResult
	for _, r := range report.Results {
		if !r.Passed {
			failures = append(failures, r)
		}
	}
	if len(failures) > 0 {
		md.WriteString("\n## Failure Details\n\n")
		for _, r := range failures {
			md.WriteString(fmt.Sprintf("### `%s`\n\n- **Reason:** %s\n", r.TaskID, r.FailureReason))
			if r.AgentOutput != "" {
				fence := "`" + "``"
				md.WriteString("- **Final answer (truncated):**\n\n" + fence + "text\n" + strings.ReplaceAll(r.AgentOutput, fence, "~~~") + "\n" + fence + "\n")
			}
			md.WriteString("\n")
		}
	}
	return md.String()
}

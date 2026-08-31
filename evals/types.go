package evals

import (
	"time"

	"github.com/actonos/actonos/internal/agent"
)

// BenchmarkDifficulty represents task complexity level.
type BenchmarkDifficulty string

const (
	DifficultyEasy   BenchmarkDifficulty = "easy"
	DifficultyMedium BenchmarkDifficulty = "medium"
	DifficultyHard   BenchmarkDifficulty = "hard"
)

// EvalTask defines the structure of a standardized evaluation test case.
type EvalTask struct {
	ID                 string                  `json:"id"`
	Title              string                  `json:"title"`
	Domain             string                  `json:"domain"` // "coding", "planning", "tool_use", "memory", "safety", "vietnamese"
	Difficulty         BenchmarkDifficulty     `json:"difficulty"`
	Language           string                  `json:"language"` // "en", "vi", "multilingual"
	UserGoal           string                  `json:"user_goal"`
	ExpectedKind       string                  `json:"expected_kind,omitempty"` // "produce", "research", "verify"
	AvailableTools     []string                `json:"available_tools,omitempty"`
	ExpectedAssertions []agent.OutcomeAssertion `json:"expected_assertions,omitempty"`
	Rubric             string                  `json:"rubric,omitempty"`
	TimeoutSec         int                     `json:"timeout_sec,omitempty"`
}

// EvalTaskResult records the execution evaluation of a single task.
type EvalTaskResult struct {
	TaskID         string                  `json:"task_id"`
	Passed         bool                    `json:"passed"`
	FalseCompleted bool                    `json:"false_completed"`
	StepsTaken     int                     `json:"steps_taken"`
	DurationMs     int64                   `json:"duration_ms"`
	TokensUsed     int                     `json:"tokens_used"`
	EstimatedCost  float64                 `json:"estimated_cost_usd"`
	FailureReason  string                  `json:"failure_reason,omitempty"`
	Assertions     []agent.AssertionResult `json:"assertions,omitempty"`
}

// BenchmarkReport summarizes an entire evaluation run.
type BenchmarkReport struct {
	RunTimestamp        time.Time        `json:"run_timestamp"`
	ModelID             string           `json:"model_id"`
	TotalTasks          int              `json:"total_tasks"`
	PassedTasks         int              `json:"passed_tasks"`
	FailedTasks         int              `json:"failed_tasks"`
	PassRatePercent     float64          `json:"pass_rate_percent"`
	FalseCompletionRate float64          `json:"false_completion_rate_percent"`
	P50LatencyMs        int64            `json:"p50_latency_ms"`
	P95LatencyMs        int64            `json:"p95_latency_ms"`
	TotalTokens         int              `json:"total_tokens"`
	TotalCostUSD        float64          `json:"total_cost_usd"`
	Results             []EvalTaskResult `json:"results"`
}

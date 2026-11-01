package evals

import (
	"encoding/json"
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
	ID                 string                   `json:"id"`
	Title              string                   `json:"title"`
	Domain             string                   `json:"domain"` // "coding", "planning", "tool_use", "memory", "safety", "vietnamese"
	Difficulty         BenchmarkDifficulty      `json:"difficulty"`
	Language           string                   `json:"language"` // "en", "vi", "multilingual"
	UserGoal           string                   `json:"user_goal"`
	ExpectedKind       string                   `json:"expected_kind,omitempty"` // "produce", "research", "verify"
	AvailableTools     []string                 `json:"available_tools,omitempty"`
	ExpectedAssertions []agent.OutcomeAssertion `json:"expected_assertions,omitempty"`
	Rubric             string                   `json:"rubric,omitempty"`
	TimeoutSec         int                      `json:"timeout_sec,omitempty"`
	// Fixtures are files (relative path -> content) placed in the task workspace
	// before the agent runs, for tasks that operate on existing input.
	Fixtures map[string]string `json:"fixtures,omitempty"`
	// Mailcal gives the task a reference mailbox and calendar behind the real mail tools.
	Mailcal *MailcalSetup `json:"mailcal,omitempty"`
	// Expect lists checks on the answer and on the mailbox after the run.
	Expect *Expectations `json:"expect,omitempty"`
}

// MailcalSetup selects the dataset and what the agent is allowed to do with it.
type MailcalSetup struct {
	Seed string `json:"seed,omitempty"` // "default"
	// Extras appends named messages to the reference dataset (see mailcalExtras in the runner).
	Extras []string `json:"extras,omitempty"`
	// Accounts names the mailboxes to create; the first is the default. Empty means one mailbox.
	Accounts []string `json:"accounts,omitempty"`
	// EnableWrites turns the master switch on. Left off, every send and calendar change is refused.
	EnableWrites bool `json:"enable_writes,omitempty"`
	// Rules are auto-send rules (policy.Rule JSON without an account id) created for the first account.
	Rules []json.RawMessage `json:"rules,omitempty"`
}

// Expectations are verified after the agent finishes, in addition to expected_assertions.
type Expectations struct {
	AnswerContains []string `json:"answer_contains,omitempty"` // case-insensitive substrings of the final answer
	DraftTo        []string `json:"draft_to,omitempty"`        // a draft addressed to each of these exists
	NoDraftTo      []string `json:"no_draft_to,omitempty"`     // no draft is addressed to any of these
	NothingSent    bool     `json:"nothing_sent,omitempty"`    // the Sent folder is unchanged
	SentTo         []string `json:"sent_to,omitempty"`         // a message left for each of these (twice listed: two attempts in the mock)
	NoSentTo       []string `json:"no_sent_to,omitempty"`      // nothing left for any of these
	SentCount      *int     `json:"sent_count,omitempty"`      // exactly this many messages left
	// ApprovalsPending counts distinct send or calendar attempts the gate stopped for a person's approval.
	ApprovalsPending *int `json:"approvals_pending,omitempty"`
	EventsCreated    *int `json:"events_created,omitempty"` // exactly this many new calendar events
	// EventsAccepted lists event ids the account owner must have accepted by the end.
	EventsAccepted []string `json:"events_accepted,omitempty"`
}

// EvalTaskResult records the execution evaluation of a single task.
type EvalTaskResult struct {
	TaskID         string  `json:"task_id"`
	Passed         bool    `json:"passed"`
	FalseCompleted bool    `json:"false_completed"`
	StepsTaken     int     `json:"steps_taken"`
	DurationMs     int64   `json:"duration_ms"`
	TokensUsed     int     `json:"tokens_used"`
	EstimatedCost  float64 `json:"estimated_cost_usd"`
	FailureReason  string  `json:"failure_reason,omitempty"`
	// AgentOutput is a truncated copy of the final answer, kept for failed tasks.
	AgentOutput string                  `json:"agent_output,omitempty"`
	Assertions  []agent.AssertionResult `json:"assertions,omitempty"`
}

// BenchmarkReport summarizes an entire evaluation run.
type BenchmarkReport struct {
	RunTimestamp        time.Time        `json:"run_timestamp"`
	Mode                string           `json:"mode"`
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

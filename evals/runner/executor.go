package main

import (
	"context"
	"time"

	"github.com/actonos/actonos/evals"
	"github.com/actonos/actonos/internal/llm"
)

// taskExecution is what an executor hands to the grader after attempting a task.
type taskExecution struct {
	Output    string
	ToolCalls []llm.ToolCall
	Steps     int
	Tokens    int
	CostUSD   float64
	Duration  time.Duration
	// Workspace is the directory the outcome assertions are checked against.
	Workspace string
	// Err is set when the attempt itself failed (timeout, provider error, ...).
	Err error
	// Detail describes how the run ended; it is shown for failed tasks.
	Detail string
	// Mailcal is the task's mailbox fixture, when it has one. The runner verifies and closes it.
	Mailcal *mailcalFixture
}

// taskExecutor runs one eval task. The mock executor fabricates results so the
// harness can be smoke-tested offline; the live executor drives the real agent
// engine against a real model.
type taskExecutor interface {
	Execute(ctx context.Context, task evals.EvalTask) taskExecution
	// Router is the LLM router the grader may use for rubric-based judging.
	Router() *llm.ModelCascadeRouter
	Close()
}

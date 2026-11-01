package main

import (
	"github.com/actonos/actonos/evals/graders"
	"github.com/actonos/actonos/internal/agent"
)

func newTestGrader(executor taskExecutor) *graders.TaskGrader {
	return graders.NewTaskGrader(agent.NewOutcomeVerifier(nil, executor.Router()), executor.Router())
}

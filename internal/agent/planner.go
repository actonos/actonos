package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/actonos/actonos/internal/llm"
)

// PlanStep represents a single decomposed step in a complex goal.
type PlanStep struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	AgentRole    string   `json:"agent_role"` // "code", "data", "report", "general"
	Dependencies []string `json:"dependencies"`
	Status       string   `json:"status"` // "pending", "in_progress", "completed", "failed"
	Result       string   `json:"result,omitempty"`
}

// TaskPlan represents a structured execution plan for complex multi-agent goals.
type TaskPlan struct {
	Goal      string     `json:"goal"`
	Steps     []PlanStep `json:"steps"`
	CreatedAt time.Time  `json:"created_at"`
}

// ExecutePlan runs dependency-ready steps in deterministic topological order.
func (p *Planner) ExecutePlan(
	ctx context.Context,
	plan *TaskPlan,
	execute func(context.Context, PlanStep) (string, error),
) error {
	if plan == nil || len(plan.Steps) == 0 {
		return errors.New("plan has no executable steps")
	}
	known := make(map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.ID == "" || known[step.ID] {
			return fmt.Errorf("plan contains an empty or duplicate step id %q", step.ID)
		}
		known[step.ID] = true
	}
	for _, step := range plan.Steps {
		for _, dependency := range step.Dependencies {
			if !known[dependency] {
				return fmt.Errorf("step %s references unknown dependency %s", step.ID, dependency)
			}
		}
	}

	completed := map[string]bool{}
	for len(completed) < len(plan.Steps) {
		progressed := false
		for index := range plan.Steps {
			step := &plan.Steps[index]
			if completed[step.ID] {
				continue
			}
			ready := true
			for _, dependency := range step.Dependencies {
				if !completed[dependency] {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			progressed = true
			step.Status = "in_progress"
			result, err := execute(ctx, *step)
			if err != nil {
				step.Status = "failed"
				step.Result = err.Error()
				return fmt.Errorf("executing plan step %s: %w", step.ID, err)
			}
			step.Status = "completed"
			step.Result = result
			completed[step.ID] = true
		}
		if !progressed {
			return errors.New("plan dependency graph contains a cycle")
		}
	}
	return nil
}

// Planner handles goal decomposition and multi-agent execution planning.
type Planner struct {
	llmRouter *llm.ModelCascadeRouter
}

// NewPlanner creates a new Planner instance.
func NewPlanner(llmRouter *llm.ModelCascadeRouter) *Planner {
	return &Planner{llmRouter: llmRouter}
}

// DecomposeGoal decomposes a complex user prompt into structured subtasks.
func (p *Planner) DecomposeGoal(ctx context.Context, goal string, availableAgents []AgentManifest) (*TaskPlan, error) {
	plan := &TaskPlan{
		Goal:      goal,
		CreatedAt: time.Now().UTC(),
	}

	prompt := fmt.Sprintf(`You are the ActonOS Orchestration Planner.
Decompose the following complex user goal into 2 to 5 actionable, sequential steps.
Available agent roles: code, data, report, general.

Goal: %s

Return ONLY valid JSON in this exact structure:
[
  {
    "id": "task_1",
    "description": "Step description",
    "agent_role": "code",
    "dependencies": []
  }
]`, goal)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	temp := 0.1
	opts := llm.CompletionOptions{
		Temperature: &temp,
	}

	resp, err := p.llmRouter.CompleteWithCascade(ctx, []string{"anthropic/claude-sonnet-4.5", "google/gemini-2.5-flash"}, messages, opts)
	if err != nil {
		slog.Warn("planner fallback to basic decomposition", "error", err)
		plan.Steps = []PlanStep{
			{ID: "task_1", Description: "Execute initial analysis for: " + goal, AgentRole: "general", Status: "pending"},
			{ID: "task_2", Description: "Consolidate and verify results for: " + goal, AgentRole: "report", Dependencies: []string{"task_1"}, Status: "pending"},
		}
		return plan, nil
	}

	var steps []PlanStep
	if err := json.Unmarshal([]byte(resp.Content), &steps); err != nil {
		plan.Steps = []PlanStep{
			{ID: "task_1", Description: goal, AgentRole: "general", Status: "pending"},
		}
		return plan, nil
	}

	for i := range steps {
		steps[i].Status = "pending"
	}
	plan.Steps = steps
	return plan, nil
}

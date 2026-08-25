package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/llm"
)

// PlanStep represents a single decomposed step in a complex goal.
type PlanStep struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	AgentRole    string   `json:"agent_role"` // "code", "data", "report", "general" or specific agent_id
	Dependencies []string `json:"dependencies"`
	Status       string   `json:"status"` // "pending", "in_progress", "completed", "failed"
	Result       string   `json:"result,omitempty"`
}

// TaskPlan represents an execution graph of subtasks for a high-level goal.
type TaskPlan struct {
	Goal      string     `json:"goal"`
	Steps     []PlanStep `json:"steps"`
	CreatedAt time.Time  `json:"created_at"`
}

// Planner decomposes high-level goals into executable DAG plans.
type Planner struct {
	llmRouter *llm.ModelCascadeRouter
}

// NewPlanner creates a new Planner instance.
func NewPlanner(llmRouter *llm.ModelCascadeRouter) *Planner {
	return &Planner{llmRouter: llmRouter}
}

// DecomposeGoal decomposes a complex user prompt into structured subtasks using the configured agent models.
func (p *Planner) DecomposeGoal(ctx context.Context, goal string, availableAgents []AgentManifest, modelCascade ...string) (*TaskPlan, error) {
	plan := &TaskPlan{
		Goal:      goal,
		CreatedAt: time.Now().UTC(),
	}

	// 1. Build cascade of models dynamically from passed models or available agents
	var cascade []string
	seen := make(map[string]bool)
	for _, m := range modelCascade {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			cascade = append(cascade, m)
		}
	}
	if len(cascade) == 0 {
		for _, ag := range availableAgents {
			if ag.ModelConfig.PrimaryModel != "" && !seen[ag.ModelConfig.PrimaryModel] {
				seen[ag.ModelConfig.PrimaryModel] = true
				cascade = append(cascade, ag.ModelConfig.PrimaryModel)
			}
			if ag.ModelConfig.FallbackModel != "" && !seen[ag.ModelConfig.FallbackModel] {
				seen[ag.ModelConfig.FallbackModel] = true
				cascade = append(cascade, ag.ModelConfig.FallbackModel)
			}
		}
	}
	if len(cascade) == 0 {
		cascade = []string{"anthropic/claude-sonnet-4.5", "google/gemini-2.5-flash"}
	}

	prompt := BuildPlannerPrompt(goal, availableAgents)
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	opts := llm.CompletionOptions{
		ReasoningEffort: llm.DefaultReasoningEffort,
	}

	resp, err := p.llmRouter.CompleteWithCascade(ctx, cascade, messages, opts)
	if err != nil {
		slog.Warn("planner fallback to basic decomposition", "cascade", cascade, "error", err)
		fallbackRole := "general"
		if len(availableAgents) > 0 && availableAgents[0].AgentID != "" {
			fallbackRole = availableAgents[0].AgentID
		}
		plan.Steps = []PlanStep{
			{ID: "task_1", Description: "Execute initial analysis for: " + goal, AgentRole: fallbackRole, Status: "pending"},
			{ID: "task_2", Description: "Consolidate and verify results for: " + goal, AgentRole: fallbackRole, Dependencies: []string{"task_1"}, Status: "pending"},
		}
		return plan, nil
	}

	var steps []PlanStep
	if err := ExtractAndUnmarshalJSON(resp.Content, &steps); err != nil {
		slog.Warn("planner failed to parse JSON, falling back to default plan", "error", err, "raw", resp.Content)
		fallbackRole := "general"
		if len(availableAgents) > 0 && availableAgents[0].AgentID != "" {
			fallbackRole = availableAgents[0].AgentID
		}
		plan.Steps = []PlanStep{
			{ID: "task_1", Description: goal, AgentRole: fallbackRole, Status: "pending"},
		}
		return plan, nil
	}

	for i := range steps {
		steps[i].Status = "pending"
	}
	plan.Steps = steps
	return plan, nil
}

// ExecutePlan runs each step in topological order, respecting dependencies.
func (p *Planner) ExecutePlan(ctx context.Context, plan *TaskPlan, stepExecutor func(ctx context.Context, step PlanStep) (string, error)) error {
	if plan == nil || len(plan.Steps) == 0 {
		return errors.New("cannot execute empty plan")
	}

	completed := make(map[string]bool)
	stepMap := make(map[string]*PlanStep)
	for i := range plan.Steps {
		stepMap[plan.Steps[i].ID] = &plan.Steps[i]
	}

	for {
		progressMade := false
		allDone := true

		for i := range plan.Steps {
			step := &plan.Steps[i]
			if step.Status == "completed" {
				continue
			}
			allDone = false

			// Check if all dependencies are satisfied
			depsMet := true
			for _, depID := range step.Dependencies {
				if !completed[depID] {
					depsMet = false
					break
				}
			}

			if depsMet && step.Status == "pending" {
				step.Status = "in_progress"
				res, err := stepExecutor(ctx, *step)
				if err != nil {
					step.Status = "failed"
					step.Result = err.Error()
					return fmt.Errorf("step %s (%s) failed: %w", step.ID, step.Description, err)
				}
				step.Status = "completed"
				step.Result = res
				completed[step.ID] = true
				progressMade = true
			}
		}

		if allDone {
			break
		}
		if !progressMade {
			return errors.New("deadlock detected in task plan dependencies")
		}
	}

	return nil
}

// NextReadyStep returns the first pending step whose dependencies are completed.
func (p *Planner) NextReadyStep(plan *TaskPlan) (*PlanStep, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return nil, nil
	}
	completed := make(map[string]bool)
	for _, step := range plan.Steps {
		if step.Status == "completed" {
			completed[step.ID] = true
		}
	}
	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Status != "pending" && step.Status != "" {
			continue
		}
		ready := true
		for _, depID := range step.Dependencies {
			if !completed[depID] {
				ready = false
				break
			}
		}
		if ready {
			return step, nil
		}
	}
	return nil, nil
}

// MarkStep records a step outcome on the in-memory plan.
func (p *TaskPlan) MarkStep(id, status, result string) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].ID == id {
			p.Steps[i].Status = status
			p.Steps[i].Result = result
			return
		}
	}
}

// ProgressPercent is completed steps / total steps as an integer 0–100.
func (p *TaskPlan) ProgressPercent() int {
	if p == nil || len(p.Steps) == 0 {
		return 0
	}
	done := 0
	for _, step := range p.Steps {
		if step.Status == "completed" {
			done++
		}
	}
	return (done * 100) / len(p.Steps)
}

// StepStatus returns the recorded status for a step ID, or empty if missing.
func (p *TaskPlan) StepStatus(id string) string {
	if p == nil {
		return ""
	}
	for _, step := range p.Steps {
		if step.ID == id {
			return step.Status
		}
	}
	return ""
}

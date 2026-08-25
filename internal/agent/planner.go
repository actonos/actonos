package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/llm"
)

const (
	// StepKindProduce requires a tool-created artifact before the step can close.
	StepKindProduce = "produce"
	// StepKindResearch gathers facts; prose is enough to close the step.
	StepKindResearch = "research"
	// StepKindVerify checks prior artifacts; prose is enough to close the step.
	StepKindVerify = "verify"

	StepStatusPending    = "pending"
	StepStatusInProgress = "in_progress"
	StepStatusCompleted  = "completed"
	StepStatusFailed     = "failed"
	// StepStatusPaused is an approval wait. It is not a failure; ResumeApproved continues it.
	StepStatusPaused = "paused"

	approvalPausedMarker = "[APPROVAL_PAUSED]"
)

// PlanStep represents a single decomposed step in a complex goal.
type PlanStep struct {
	ID           string   `json:"id"`
	Title        string   `json:"title,omitempty"`
	Description  string   `json:"description"`
	Acceptance   string   `json:"acceptance,omitempty"`
	AgentRole    string   `json:"agent_role"` // "code", "data", "report", "general" or specific agent_id
	Kind         string   `json:"kind,omitempty"`   // produce | research | verify
	Atomic       bool     `json:"atomic,omitempty"` // whole goal is this single step
	Dependencies []string `json:"dependencies"`
	Status       string   `json:"status"` // pending | in_progress | completed | failed | paused
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

	prompt := BuildPlannerPrompt(goal, availableAgents, SkillCatalogFrom(ctx)...)
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	opts := llm.CompletionOptions{
		ReasoningEffort: llm.DefaultReasoningEffort,
	}

	resp, err := p.llmRouter.CompleteWithCascade(ctx, cascade, messages, opts)
	if err != nil {
		slog.Warn("planner fallback to basic decomposition", "cascade", cascade, "error", err)
		plan.Steps = defaultPlanSteps(goal, availableAgents)
		return plan, nil
	}

	var steps []PlanStep
	if err := ExtractAndUnmarshalJSON(resp.Content, &steps); err != nil {
		slog.Warn("planner failed to parse JSON, falling back to default plan", "error", err, "raw", resp.Content)
		plan.Steps = defaultPlanSteps(goal, availableAgents)
		return plan, nil
	}

	plan.Steps = normalizePlanSteps(steps, fallbackPlanRole(availableAgents))
	if len(plan.Steps) == 0 {
		plan.Steps = defaultPlanSteps(goal, availableAgents)
	}
	// A 1-step plan is only legal when the planner marked it atomic. Otherwise
	// expand so a wrap-up after the first artifact cannot close the mission.
	// The planner LLM reads the user goal in any language; the kernel does not.
	if len(plan.Steps) == 1 && !plan.Steps[0].Atomic {
		plan.Steps = defaultPlanSteps(goal, availableAgents)
	}
	return plan, nil
}

func fallbackPlanRole(agents []AgentManifest) string {
	if len(agents) > 0 && agents[0].AgentID != "" {
		return agents[0].AgentID
	}
	return "general"
}

func defaultPlanSteps(goal string, agents []AgentManifest) []PlanStep {
	role := fallbackPlanRole(agents)
	trimmed := strings.TrimSpace(goal)
	return []PlanStep{
		{
			ID:          "task_1",
			Title:       "Gather inputs",
			Description: "Collect the facts, files, and constraints required to execute: " + trimmed,
			Acceptance:  "Named sources or files are identified and the execution constraints are explicit.",
			AgentRole:   role,
			Kind:        StepKindResearch,
			Status:      "pending",
		},
		{
			ID:           "task_2",
			Title:        "Produce and verify",
			Description:  "Create the requested deliverable and verify it against the original goal: " + trimmed,
			Acceptance:   "The deliverable exists with tool evidence and matches the goal.",
			AgentRole:    role,
			Kind:         StepKindProduce,
			Dependencies: []string{"task_1"},
			Status:       "pending",
		},
	}
}

const maxPlanSteps = 5

func normalizePlanSteps(steps []PlanStep, fallbackRole string) []PlanStep {
	if fallbackRole == "" {
		fallbackRole = "general"
	}
	seen := make(map[string]bool)
	out := make([]PlanStep, 0, len(steps))
	for _, step := range steps {
		if len(out) >= maxPlanSteps {
			break
		}
		title := collapseSpace(step.Title)
		desc := collapseSpace(step.Description)
		if desc == "" {
			desc = title
		}
		if desc == "" {
			continue
		}
		id := collapseSpace(step.ID)
		if id == "" {
			id = fmt.Sprintf("task_%d", len(out)+1)
		}
		if seen[id] {
			id = fmt.Sprintf("task_%d", len(out)+1)
		}
		seen[id] = true
		role := collapseSpace(step.AgentRole)
		if role == "" {
			role = fallbackRole
		}
		var deps []string
		depSeen := map[string]bool{}
		for _, dep := range step.Dependencies {
			dep = collapseSpace(dep)
			if dep == "" || dep == id || depSeen[dep] {
				continue
			}
			depSeen[dep] = true
			deps = append(deps, dep)
		}
		out = append(out, PlanStep{
			ID:           id,
			Title:        title,
			Description:  desc,
			Acceptance:   collapseSpace(step.Acceptance),
			AgentRole:    role,
			Kind:         normalizeStepKind(step.Kind),
			Atomic:       step.Atomic,
			Dependencies: deps,
			Status:       "pending",
			Result:       step.Result,
		})
	}
	known := make(map[string]bool, len(out))
	for _, step := range out {
		known[step.ID] = true
	}
	for i := range out {
		filtered := out[i].Dependencies[:0]
		for _, dep := range out[i].Dependencies {
			if known[dep] {
				filtered = append(filtered, dep)
			}
		}
		out[i].Dependencies = filtered
	}
	return out
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func normalizeStepKind(kind string) string {
	switch strings.ToLower(collapseSpace(kind)) {
	case StepKindProduce, StepKindResearch, StepKindVerify:
		return strings.ToLower(collapseSpace(kind))
	default:
		return ""
	}
}

func (s *PlanStep) requiresArtifact() bool {
	return s != nil && s.Kind == StepKindProduce
}

var planStepIDRe = regexp.MustCompile(`<step_id>([^<]+)</step_id>`)

// PlanStepIDFromPrompt extracts the DAG step id from a plan-step execution prompt.
func PlanStepIDFromPrompt(prompt string) string {
	match := planStepIDRe.FindStringSubmatch(prompt)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func isApprovalPauseResult(result string) bool {
	return strings.Contains(result, approvalPausedMarker) ||
		strings.Contains(strings.ToLower(result), "human approval required") ||
		strings.Contains(result, "approval_id=")
}

// reopenApprovalFailedSteps turns approval-shaped failures back into pending
// work so a mission that was marked failed when a tool paused for approval
// can continue after the operator decides.
func (p *TaskPlan) reopenApprovalFailedSteps() bool {
	if p == nil {
		return false
	}
	changed := false
	for i := range p.Steps {
		step := &p.Steps[i]
		if step.Status == StepStatusFailed && isApprovalPauseResult(step.Result) {
			step.Status = StepStatusPending
			changed = true
		}
	}
	return changed
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

// HasReadyStep reports whether a pending step can run now.
func (p *TaskPlan) HasReadyStep() bool {
	step, err := (*Planner)(nil).NextReadyStep(p)
	return err == nil && step != nil
}

func (p *TaskPlan) markRemainingComplete(result string) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].Status != "completed" && p.Steps[i].Status != "failed" {
			p.Steps[i].Status = "completed"
			if strings.TrimSpace(p.Steps[i].Result) == "" {
				p.Steps[i].Result = result
			}
		}
	}
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

// AllStepsCompleted reports whether every DAG step finished successfully.
func (p *TaskPlan) AllStepsCompleted() bool {
	if p == nil || len(p.Steps) == 0 {
		return false
	}
	for _, step := range p.Steps {
		if step.Status != "completed" {
			return false
		}
	}
	return true
}

// CompletionSummary is a durable log line used when the DAG itself is the
// completion signal (no extra LLM turn, no [TASK_COMPLETED] token required).
func (p *TaskPlan) CompletionSummary() string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("All plan steps completed.")
	for _, step := range p.Steps {
		result := strings.TrimSpace(step.Result)
		if result == "" {
			result = step.Status
		}
		if len(result) > 180 {
			result = result[:180] + "..."
		}
		fmt.Fprintf(&b, "\n- %s: %s", step.ID, result)
	}
	return b.String()
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

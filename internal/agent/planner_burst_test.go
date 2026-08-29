package agent

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

func TestExecuteBurstPlanStepsConcurrently(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatalf("creating agent mgr: %v", err)
	}
	taskMgr, err := NewTaskManager(db.SQLDB(), t.TempDir())
	if err != nil {
		t.Fatalf("creating task mgr: %v", err)
	}

	router := llm.NewModelCascadeRouter()
	mockProv := llm.NewMockProvider("mock-model", "Done step work successfully")
	mockProv.CompleteFunc = func(_ context.Context, _ []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		return &llm.Response{
			Content: "Done step work successfully",
			Usage:   llm.Usage{TotalTokens: 42},
		}, nil
	}
	router.RegisterProvider("mock-model", mockProv)

	planner := NewPlanner(router)
	toolReg := tools.NewToolRegistry(nil)

	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetPlanner(planner)
	engine.SetTaskManager(taskMgr)
	engine.SetToolRegistry(toolReg)

	ctx := context.Background()
	manifest := AgentManifest{
		AgentID: "agent_burst_worker",
		Name:    "Burst Worker",
		Status:  StatusActive,
		ModelConfig: llm.ModelConfig{
			PrimaryModel: "mock-model",
		},
		DelegationScope: DelegationScope{
			MaxConcurrentRuns: 5,
		},
	}
	if _, err := agentMgr.Create(ctx, manifest); err != nil {
		t.Fatalf("creating agent: %v", err)
	}

	// Create a plan with 3 independent tasks (no dependencies between each other)
	plan := &TaskPlan{
		Goal: "Process 3 parallel feeds",
		Steps: []PlanStep{
			{
				ID:          "step_1",
				Title:       "Fetch Feed A",
				Description: "Read feed A data",
				AgentRole:   "agent_burst_worker",
				Status:      "pending",
				Kind:        StepKindResearch,
			},
			{
				ID:          "step_2",
				Title:       "Fetch Feed B",
				Description: "Read feed B data",
				AgentRole:   "agent_burst_worker",
				Status:      "pending",
				Kind:        StepKindResearch,
			},
			{
				ID:          "step_3",
				Title:       "Fetch Feed C",
				Description: "Read feed C data",
				AgentRole:   "agent_burst_worker",
				Status:      "pending",
				Kind:        StepKindResearch,
			},
		},
		CreatedAt: time.Now().UTC(),
	}

	task, err := taskMgr.CreateTask(ctx, AutonomousTask{
		Title:           "Parallel Feed Mission",
		Description:     "Process 3 parallel feeds",
		AssignedAgentID: "agent_burst_worker",
		Plan:            plan,
	})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	taskCtx := context.WithValue(ctx, "task_id", task.ID)

	// Execute via burst pulse
	resp, updatedPlan, err := engine.ExecuteBurstPlanSteps(taskCtx, "agent_burst_worker", task.Description, plan, nil)
	if err != nil {
		t.Fatalf("ExecuteBurstPlanSteps failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response from burst execution")
	}

	if updatedPlan == nil {
		t.Fatal("expected non-nil updated plan")
	}

	// Verify all 3 independent steps are completed in this single burst execution
	if !updatedPlan.AllStepsCompleted() {
		t.Fatalf("expected all 3 independent steps to complete in 1 burst pulse, statuses: %s, %s, %s",
			updatedPlan.StepStatus("step_1"), updatedPlan.StepStatus("step_2"), updatedPlan.StepStatus("step_3"))
	}
	if resp.Usage.TotalTokens < 42*3 {
		t.Fatalf("expected total tokens to aggregate at least %d, got %d", 42*3, resp.Usage.TotalTokens)
	}
}

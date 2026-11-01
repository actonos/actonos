package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/actonos/actonos/evals"
	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
)

// mockExecutor pre-creates the artifacts each task asserts on and fabricates
// latency and token counts. It validates the task definitions and graders only;
// it says nothing about any model.
type mockExecutor struct {
	root   string
	router *llm.ModelCascadeRouter
}

func newMockExecutor(modelID string) (*mockExecutor, error) {
	root, err := os.MkdirTemp("", "actonos-eval-*")
	if err != nil {
		return nil, err
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider(modelID, llm.NewMockProvider(modelID, `{"status":"ok"}`))
	return &mockExecutor{root: root, router: router}, nil
}

func (m *mockExecutor) Router() *llm.ModelCascadeRouter { return m.router }

func (m *mockExecutor) Close() { _ = os.RemoveAll(m.root) }

func (m *mockExecutor) Execute(_ context.Context, task evals.EvalTask) taskExecution {
	workspace := filepath.Join(m.root, task.ID)
	_ = os.MkdirAll(workspace, 0755)

	var toolCalls []llm.ToolCall
	if task.ExpectedKind == "produce" {
		for _, assertion := range task.ExpectedAssertions {
			if assertion.Kind == agent.AssertFileExists || assertion.Kind == agent.AssertFileContains || assertion.Kind == agent.AssertJSONSchema {
				target := filepath.Join(workspace, assertion.Target)
				content := []byte("Initial benchmark payload")
				if assertion.Kind == agent.AssertJSONSchema {
					content = []byte(`{"agent_id":"a1","name":"Agent 1","status":"active","priority":"P1","id":"dir_1","title":"Daily"}`)
				} else if assertion.Expected != "" {
					content = []byte("Header\n" + assertion.Expected + "\nFooter")
				}
				_ = os.WriteFile(target, content, 0644)
				toolCalls = append(toolCalls, llm.ToolCall{
					ID:       "call_eval_" + task.ID,
					Function: llm.FunctionCall{Name: "native_file_write"},
				})
			}
		}
	}

	return taskExecution{
		Output:    "Task executed successfully.",
		ToolCalls: toolCalls,
		Steps:     1,
		Tokens:    500,
		CostUSD:   0.002,
		Duration:  time.Duration(150+len(task.UserGoal)*2) * time.Millisecond,
		Workspace: workspace,
	}
}

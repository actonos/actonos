package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// mockJSON builds a JSON object containing every key the assertion requires.
func mockJSON(expected string) []byte {
	obj := map[string]string{}
	for _, key := range strings.Split(expected, ",") {
		if key = strings.TrimSpace(key); key != "" {
			obj[key] = "mock"
		}
	}
	if len(obj) == 0 {
		obj["status"] = "ok"
	}
	raw, _ := json.Marshal(obj)
	return raw
}

func (m *mockExecutor) Router() *llm.ModelCascadeRouter { return m.router }

func (m *mockExecutor) Close() { _ = os.RemoveAll(m.root) }

func (m *mockExecutor) Execute(_ context.Context, task evals.EvalTask) taskExecution {
	workspace := filepath.Join(m.root, task.ID)
	_ = os.MkdirAll(workspace, 0755)
	if err := writeFixtures(workspace, task.Fixtures); err != nil {
		return taskExecution{Workspace: workspace, Err: err}
	}

	var toolCalls []llm.ToolCall
	if task.ExpectedKind == "produce" {
		for _, assertion := range task.ExpectedAssertions {
			if assertion.Kind == agent.AssertFileExists || assertion.Kind == agent.AssertFileContains || assertion.Kind == agent.AssertJSONSchema {
				target := filepath.Join(workspace, assertion.Target)
				content := []byte("Initial benchmark payload")
				if assertion.Kind == agent.AssertJSONSchema {
					content = mockJSON(assertion.Expected)
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

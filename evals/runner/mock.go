package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/actonos/actonos/evals"
	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"

	_ "modernc.org/sqlite"
)

// mockExecutor pre-creates the artifacts each task asserts on and fabricates
// latency and token counts. It validates the task definitions and graders only;
// it says nothing about any model.
type mockExecutor struct {
	root     string
	router   *llm.ModelCascadeRouter
	db       *sql.DB
	registry *tools.ToolRegistry
}

func newMockExecutor(modelID string) (*mockExecutor, error) {
	root, err := os.MkdirTemp("", "actonos-eval-*")
	if err != nil {
		return nil, err
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider(modelID, llm.NewMockProvider(modelID, `{"status":"ok"}`))
	db, err := sql.Open("sqlite", filepath.Join(root, "mock.db"))
	if err != nil {
		return nil, err
	}
	return &mockExecutor{root: root, router: router, db: db, registry: tools.NewToolRegistry(nil)}, nil
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

func (m *mockExecutor) Close() {
	_ = m.db.Close()
	_ = os.RemoveAll(m.root)
}

func (m *mockExecutor) Execute(ctx context.Context, task evals.EvalTask) taskExecution {
	workspace := filepath.Join(m.root, task.ID)
	_ = os.MkdirAll(workspace, 0755)
	if err := writeFixtures(workspace, task.Fixtures); err != nil {
		return taskExecution{Workspace: workspace, Err: err}
	}

	output := "Task executed successfully."
	var mail *mailcalFixture
	if task.Mailcal != nil {
		var err error
		if mail, err = newMailcalFixture(ctx, m.db, m.registry, task.Mailcal); err != nil {
			return taskExecution{Workspace: workspace, Err: err}
		}
		said, err := mail.perform(ctx, task.Expect)
		if err != nil {
			return taskExecution{Workspace: workspace, Err: err}
		}
		if said != "" {
			output = said
		}
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
		Output:    output,
		Mailcal:   mail,
		ToolCalls: toolCalls,
		Steps:     1,
		Tokens:    500,
		CostUSD:   0.002,
		Duration:  time.Duration(150+len(task.UserGoal)*2) * time.Millisecond,
		Workspace: workspace,
	}
}

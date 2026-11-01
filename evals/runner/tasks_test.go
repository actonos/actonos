package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/evals"
)

func loadTasks(t *testing.T) map[string]evals.EvalTask {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "tasks", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no task files found: %v", err)
	}
	tasks := map[string]evals.EvalTask{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var task evals.EvalTask
		if err := json.Unmarshal(raw, &task); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		base := strings.TrimSuffix(filepath.Base(f), ".json")
		if task.ID != "task_"+base {
			t.Errorf("%s: id %q does not match the file name", f, task.ID)
		}
		if _, dup := tasks[task.ID]; dup {
			t.Errorf("duplicate task id %q", task.ID)
		}
		tasks[task.ID] = task
	}
	return tasks
}

// A task that operates on an existing file must ship that file, otherwise the
// model is graded on something it cannot possibly do.
func TestTasksThatReadExistingInputShipFixtures(t *testing.T) {
	tasks := loadTasks(t)
	for id, files := range map[string][]string{
		"task_01_code_fix_syntax":               {"main.go"},
		"task_02_code_refactor_pure_go":         {"db.go"},
		"task_20_vietnamese_summary_extraction": {"email.txt"},
	} {
		task := tasks[id]
		for _, name := range files {
			if _, ok := task.Fixtures[name]; !ok {
				t.Errorf("%s needs fixture %s", id, name)
			}
			if !strings.Contains(task.UserGoal, name) {
				t.Errorf("%s: goal should mention %s so the model knows where the input is", id, name)
			}
		}
	}
}

func TestEveryProduceTaskHasAnAssertionOrIsASafetyCheck(t *testing.T) {
	for id, task := range loadTasks(t) {
		if task.ExpectedKind == "produce" && len(task.ExpectedAssertions) == 0 && task.Expect == nil && task.Domain != "safety" {
			t.Errorf("%s produces output but asserts nothing", id)
		}
	}
}

func TestWriteFixturesStaysInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := writeFixtures(dir, map[string]string{"sub/dir/a.txt": "x"}); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "a.txt")); err != nil || string(raw) != "x" {
		t.Fatalf("fixture not written: %q %v", raw, err)
	}
	for _, bad := range []string{"../escape.txt", "/etc/passwd", "a/../../escape.txt"} {
		if err := writeFixtures(dir, map[string]string{bad: "x"}); err == nil {
			t.Errorf("fixture path %q must be rejected", bad)
		}
	}
}

func TestMockRunPassesEveryTaskWithFixtures(t *testing.T) {
	executor, err := newMockExecutor("mock")
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	grader := newTestGrader(executor)
	for id, task := range loadTasks(t) {
		ex := executor.Execute(t.Context(), task)
		if ex.Err != nil {
			t.Fatalf("%s: %v", id, ex.Err)
		}
		res := grader.GradeTask(t.Context(), task, ex.Workspace, ex.Output, ex.ToolCalls, ex.Steps, ex.Duration, ex.Tokens, ex.CostUSD)
		if !res.Passed {
			t.Errorf("%s should pass in the mock harness: %s", id, res.FailureReason)
		}
	}
}

func TestExcerptTruncatesOnRuneBoundaries(t *testing.T) {
	if got := excerpt("  ngắn  ", 10); got != "ngắn" {
		t.Fatalf("got %q", got)
	}
	got := excerpt("Tiếng Việt có dấu rất dài", 8)
	if !strings.HasSuffix(got, "...") || len([]rune(got)) != 11 {
		t.Fatalf("got %q", got)
	}
}

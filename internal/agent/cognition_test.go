package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

func TestPlanner_DecomposeGoal(t *testing.T) {
	router := llm.NewModelCascadeRouter()
	mock := llm.NewMockProvider("anthropic/claude-3-7-sonnet", `[{"id":"task_1","description":"Analyze code","agent_role":"code","dependencies":[]}]`)
	router.RegisterProvider("anthropic/claude-3-7-sonnet", mock)

	planner := NewPlanner(router)
	plan, err := planner.DecomposeGoal(context.Background(), "Build a microservice", nil)
	if err != nil {
		t.Fatalf("DecomposeGoal failed: %v", err)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ID != "task_1" {
		t.Fatalf("unexpected task id: %s", plan.Steps[0].ID)
	}
}

func TestVerifier_StaticAnalysis(t *testing.T) {
	verifier := NewVerifier()

	// 1. Verify Forbidden Commands
	if err := verifier.VerifyCommand("rm -rf / --no-preserve-root"); err == nil {
		t.Fatal("expected error on forbidden command, got nil")
	}

	if err := verifier.VerifyCommand("ls -la /workspace"); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	// 2. Verify Path Escape
	tempDir := t.TempDir()
	workspace := filepath.Join(tempDir, "workspace")

	if err := verifier.VerifyPath("../etc/passwd", workspace); err == nil {
		t.Fatal("expected path escape error, got nil")
	}

	if err := verifier.VerifyPath("src/main.go", workspace); err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
}

func TestUserProfile_And_ContextManager(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	mgr, err := NewUserProfileManager(db, tempDir)
	if err != nil {
		t.Fatalf("creating user profile manager: %v", err)
	}

	ctx := context.Background()
	profile := UserProfile{
		UserName:           "Bieber",
		Language:           "vi",
		CommunicationStyle: "concise",
		Preferences:        map[string]string{"theme": "dark"},
	}
	if err := mgr.UpdateProfile(ctx, profile); err != nil {
		t.Fatalf("updating profile: %v", err)
	}

	p := mgr.GetProfile()
	if p.UserName != "Bieber" || p.Language != "vi" {
		t.Fatalf("unexpected profile values: %+v", p)
	}

	// Context Manager test
	ctxMgr := NewContextManager(4096)
	augmented := ctxMgr.BuildAugmentedSystemPrompt("Base Instructions", p, nil)
	if !strings.Contains(augmented, "User: Bieber") || !strings.Contains(augmented, "Preferred Language: vi") {
		t.Fatalf("augmented prompt missing profile info: %s", augmented)
	}
}

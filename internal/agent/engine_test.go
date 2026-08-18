package agent

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

func TestEngine_ExecuteStep(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, _ := NewAgentManager(db, eventBus)

	llmRouter := llm.NewModelCascadeRouter()
	mockLLM := llm.NewMockProvider("claude-3-7-sonnet", "Hello, I am ready to help you.")
	llmRouter.RegisterProvider("claude-3-7-sonnet", mockLLM)

	engine := NewEngine(agentMgr, eventBus, llmRouter, nil)
	ctx := context.Background()

	created, err := agentMgr.Create(ctx, AgentManifest{
		Name:               "Assistant",
		SystemInstructions: "You are a helpful assistant.",
		ModelConfig: llm.ModelConfig{
			PrimaryModel: "claude-3-7-sonnet",
		},
	})
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	resp, err := engine.ExecuteStep(ctx, created.AgentID, "Hello!")
	if err != nil {
		t.Fatalf("ExecuteStep failed: %v", err)
	}

	if resp.Content != "Hello, I am ready to help you." {
		t.Fatalf("unexpected response content: %s", resp.Content)
	}
}

func TestCalculateEntropy(t *testing.T) {
	// Deterministic probability distribution [1.0] -> Entropy = 0
	h1 := CalculateEntropy([]float64{1.0})
	if math.Abs(h1-0.0) > 0.001 {
		t.Fatalf("expected entropy 0 for deterministic outcome, got %f", h1)
	}

	// Uniform distribution over 2 outcomes [0.5, 0.5] -> Entropy = 1.0 bit
	h2 := CalculateEntropy([]float64{0.5, 0.5})
	if math.Abs(h2-1.0) > 0.001 {
		t.Fatalf("expected entropy 1.0 for uniform binary outcome, got %f", h2)
	}
}

func TestEngineDurableApprovalResumeEndToEnd(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	approvalMgr := tools.NewApprovalManager(db.SQLDB())
	registry := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(registry, workspace)
	registry.SetApprovalManager(approvalMgr)
	registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{
			AuthorizedTools: []string{"native_file_write"}, ApprovalThreshold: "High",
			AllowedPaths: []string{"*"},
		}, nil
	})

	provider := llm.NewMockProvider("resume-model", "")
	provider.CompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		if provider.CompleteCalls == 1 {
			return &llm.Response{
				Model: "resume-model", Usage: llm.Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
				Content: "Writing the requested file.",
				ToolCalls: []llm.ToolCall{{
					ID: "call-write", Type: "function",
					Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"result.txt","content":"exactly once"}`)},
				}},
			}, nil
		}
		if len(messages) == 0 || messages[len(messages)-1].Role != llm.RoleTool {
			t.Fatalf("resume did not continue from tool observation: %+v", messages)
		}
		return &llm.Response{
			Model: "resume-model", Content: "The requested file was written and verified successfully.",
			Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("resume-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	runStore := NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)

	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Resume agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "resume-model"},
		AuthorizedTools: []string{"native_file_write"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.ExecuteStep(context.Background(), manifest.AgentID, "Write result.txt with exactly once")
	var approvalRequired *tools.ApprovalRequiredError
	if !errors.As(err, &approvalRequired) {
		t.Fatalf("expected durable approval pause, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "result.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("tool executed before approval: %v", statErr)
	}
	pendingRuns, err := runStore.List(context.Background(), 10)
	if err != nil || len(pendingRuns) != 1 || pendingRuns[0].Status != RunApprovalPending {
		t.Fatalf("run was not paused durably: %+v err=%v", pendingRuns, err)
	}

	approved, err := approvalMgr.Decide(context.Background(), approvalRequired.Approval.ID, "approved", "tester", "verified")
	if err != nil {
		t.Fatal(err)
	}
	response, err := engine.ResumeApproved(context.Background(), *approved)
	if err != nil {
		t.Fatal(err)
	}
	if response.Content == "" || response.Usage.TotalTokens != 11 || provider.CompleteCalls != 2 {
		t.Fatalf("unexpected resumed response: response=%+v calls=%d", response, provider.CompleteCalls)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil || string(data) != "exactly once" {
		t.Fatalf("approved tool result mismatch: data=%q err=%v", data, err)
	}
	runs, err := runStore.List(context.Background(), 10)
	if err != nil || runs[0].ID != pendingRuns[0].ID || runs[0].Status != RunCompleted ||
		runs[0].TerminationReason != "goal_completed" || runs[0].TotalTokens != 11 {
		t.Fatalf("same durable run did not complete: %+v err=%v", runs, err)
	}
	events, err := runStore.Events(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var approvalExecutions int
	for _, event := range events {
		if event.Type == "approval_execution" {
			approvalExecutions++
		}
	}
	if approvalExecutions != 1 {
		t.Fatalf("approved action executed %d times; events=%+v", approvalExecutions, events)
	}
}

func TestEngineStreamingReActToolExecutionAndEvents(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte("stream observation"), 0644); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(registry, workspace)
	registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{
			AuthorizedTools: []string{"native_file_read"}, ApprovalThreshold: "High",
			AllowedPaths: []string{"*"},
		}, nil
	})
	provider := llm.NewMockProvider("stream-model", "")
	provider.StreamCompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (<-chan llm.StreamChunk, error) {
		ch := make(chan llm.StreamChunk, 3)
		if provider.StreamCompleteCalls == 1 {
			ch <- llm.StreamChunk{DeltaContent: "Inspecting.", ToolCalls: []llm.ToolCall{{
				ID: "read-one", Type: "function",
				Function: llm.FunctionCall{Name: "native_file_read", Arguments: json.RawMessage(`{"path":"input.txt"}`)},
			}}, Usage: &llm.Usage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3}}
		} else {
			if len(messages) == 0 || messages[len(messages)-1].Role != llm.RoleTool ||
				!strings.Contains(messages[len(messages)-1].Content, "stream observation") {
				t.Errorf("streaming tool observation missing: %+v", messages)
			}
			ch <- llm.StreamChunk{DeltaContent: "Verified stream observation.", Usage: &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}}
		}
		ch <- llm.StreamChunk{Done: true}
		close(ch)
		return ch, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("stream-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	engine.SetRunStore(NewRunStore(db.SQLDB()))
	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Streaming agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "stream-model"},
		AuthorizedTools: []string{"native_file_read"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan AgentStreamEvent, 64)
	response, err := engine.ExecuteStepStream(context.Background(), manifest.AgentID, "Read and verify input.txt", events)
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "Verified stream observation." || response.Usage.TotalTokens != 8 ||
		provider.StreamCompleteCalls != 2 {
		t.Fatalf("unexpected streaming response: %+v calls=%d", response, provider.StreamCompleteCalls)
	}
	seen := map[StreamEventType]bool{}
	for event := range events {
		seen[event.Type] = true
	}
	for _, eventType := range []StreamEventType{
		EventStreamThought, EventStreamToken, EventStreamToolCall,
		EventStreamToolResult, EventStreamAudit, EventStreamDone,
	} {
		if !seen[eventType] {
			t.Fatalf("missing streaming event %s; seen=%v", eventType, seen)
		}
	}
}

func TestEngineDurableApprovalResumeWithChainedToolCalls(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	approvalMgr := tools.NewApprovalManager(db.SQLDB())
	registry := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(registry, workspace)
	registry.SetApprovalManager(approvalMgr)
	registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{
			AuthorizedTools: []string{"native_file_write", "native_file_read"},
			ApprovalThreshold: "High",
			AllowedPaths:      []string{"*"},
		}, nil
	})

	provider := llm.NewMockProvider("chained-model", "")
	provider.CompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		if provider.CompleteCalls == 1 {
			return &llm.Response{
				Model: "chained-model", Usage: llm.Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
				Content: "Writing step.",
				ToolCalls: []llm.ToolCall{{
					ID: "call-write", Type: "function",
					Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"test.txt","content":"chain_content"}`)},
				}},
			}, nil
		}
		if provider.CompleteCalls == 2 {
			// After resume, model executes a low-risk read tool to verify content.
			return &llm.Response{
				Model: "chained-model", Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
				Content: "Reading step to verify.",
				ToolCalls: []llm.ToolCall{{
					ID: "call-read", Type: "function",
					Function: llm.FunctionCall{Name: "native_file_read", Arguments: json.RawMessage(`{"path":"test.txt"}`)},
				}},
			}, nil
		}
		return &llm.Response{
			Model: "chained-model", Content: "Verified content: chain_content",
			Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("chained-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	runStore := NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)

	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Chained agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "chained-model"},
		AuthorizedTools: []string{"native_file_write", "native_file_read"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.ExecuteStep(context.Background(), manifest.AgentID, "Write and verify test.txt")
	var approvalRequired *tools.ApprovalRequiredError
	if !errors.As(err, &approvalRequired) {
		t.Fatalf("expected approval pause, got %v", err)
	}

	approved, err := approvalMgr.Decide(context.Background(), approvalRequired.Approval.ID, "approved", "admin", "ok")
	if err != nil {
		t.Fatal(err)
	}
	response, err := engine.ResumeApproved(context.Background(), *approved)
	if err != nil {
		t.Fatalf("ResumeApproved failed: %v", err)
	}
	if response.Content != "Verified content: chain_content" || provider.CompleteCalls != 3 {
		t.Fatalf("unexpected response: %+v calls=%d", response, provider.CompleteCalls)
	}
}

func TestEngineDurableApprovalResumeWithChainedApproval(t *testing.T) {
	db, eventBus := setupTestDB(t)
	agentMgr, err := NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	approvalMgr := tools.NewApprovalManager(db.SQLDB())
	registry := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(registry, workspace)
	registry.SetApprovalManager(approvalMgr)
	registry.SetPolicyResolver(func(context.Context, string) (tools.AgentToolPolicy, error) {
		return tools.AgentToolPolicy{
			AuthorizedTools: []string{"native_file_write", "native_file_delete"},
			ApprovalThreshold: "High",
			AllowedPaths:      []string{"*"},
		}, nil
	})

	provider := llm.NewMockProvider("chained-appr-model", "")
	provider.CompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (*llm.Response, error) {
		if provider.CompleteCalls == 1 {
			return &llm.Response{
				Model: "chained-appr-model", Usage: llm.Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
				Content: "First action.",
				ToolCalls: []llm.ToolCall{{
					ID: "call-write-1", Type: "function",
					Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"a.txt","content":"data"}`)},
				}},
			}, nil
		}
		if provider.CompleteCalls == 2 {
			// Second action also requires high-risk approval (native_file_delete)
			return &llm.Response{
				Model: "chained-appr-model", Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
				Content: "Second action requiring approval.",
				ToolCalls: []llm.ToolCall{{
					ID: "call-delete-1", Type: "function",
					Function: llm.FunctionCall{Name: "native_file_delete", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
				}},
			}, nil
		}
		return &llm.Response{
			Model: "chained-appr-model", Content: "Finished deleting a.txt.",
			Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		}, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("chained-appr-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	runStore := NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)

	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Chained appr agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "chained-appr-model"},
		AuthorizedTools: []string{"native_file_write", "native_file_delete"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.ExecuteStep(context.Background(), manifest.AgentID, "Write then delete a.txt")
	var approvalReq1 *tools.ApprovalRequiredError
	if !errors.As(err, &approvalReq1) {
		t.Fatalf("expected first approval pause, got %v", err)
	}

	appr1, err := approvalMgr.Decide(context.Background(), approvalReq1.Approval.ID, "approved", "admin", "ok")
	if err != nil {
		t.Fatal(err)
	}

	// Resuming approval 1 should trigger approval 2!
	_, err = engine.ResumeApproved(context.Background(), *appr1)
	var approvalReq2 *tools.ApprovalRequiredError
	if !errors.As(err, &approvalReq2) {
		t.Fatalf("expected chained approval pause, got %v", err)
	}
	if approvalReq2.Approval.ToolName != "native_file_delete" {
		t.Fatalf("expected approval for native_file_delete, got %s", approvalReq2.Approval.ToolName)
	}

	appr2, err := approvalMgr.Decide(context.Background(), approvalReq2.Approval.ID, "approved", "admin", "ok")
	if err != nil {
		t.Fatal(err)
	}

	// Resuming approval 2 should complete!
	resp, err := engine.ResumeApproved(context.Background(), *appr2)
	if err != nil {
		t.Fatalf("second resume failed: %v", err)
	}
	if resp.Content != "Finished deleting a.txt." || provider.CompleteCalls != 3 {
		t.Fatalf("unexpected final response: %+v calls=%d", resp, provider.CompleteCalls)
	}
}


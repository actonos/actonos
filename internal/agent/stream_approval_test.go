package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/tools"
)

func TestEngineStreamingApprovalWaitsThenContinues(t *testing.T) {
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
	provider := llm.NewMockProvider("stream-approve-model", "")
	provider.StreamCompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (<-chan llm.StreamChunk, error) {
		ch := make(chan llm.StreamChunk, 3)
		if provider.StreamCompleteCalls == 1 {
			ch <- llm.StreamChunk{DeltaContent: "Writing.", ToolCalls: []llm.ToolCall{{
				ID: "write-one", Type: "function",
				Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"approved.txt","content":"from-stream"}`)},
			}}, Usage: &llm.Usage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3}}
		} else {
			if len(messages) == 0 || messages[len(messages)-1].Role != llm.RoleTool ||
				!strings.Contains(messages[len(messages)-1].Content, "approved.txt") {
				t.Errorf("approved observation missing: %+v", messages)
			}
			ch <- llm.StreamChunk{DeltaContent: "Wrote the approved file.", Usage: &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}}
		}
		ch <- llm.StreamChunk{Done: true}
		close(ch)
		return ch, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("stream-approve-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	engine.SetRunStore(NewRunStore(db.SQLDB()))
	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Streaming approval agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "stream-approve-model"},
		AuthorizedTools: []string{"native_file_write"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan AgentStreamEvent, 64)
	var collected []AgentStreamEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range events {
			collected = append(collected, ev)
			if ev.Type == EventStreamApprovalRequired && ev.Approval != nil {
				if _, decideErr := approvalMgr.Decide(context.Background(), ev.Approval.ID, "approved", "tester", "ok"); decideErr != nil {
					t.Errorf("deciding approval: %v", decideErr)
					return
				}
				engine.NotifyApprovalDecision(ev.Approval.ID, true, "ok")
			}
		}
	}()

	response, err := engine.ExecuteStepStream(context.Background(), manifest.AgentID, "Write approved.txt", events)
	<-done
	if err != nil {
		t.Fatalf("stream should continue after approval, got %v", err)
	}
	if response == nil || response.Content != "Wrote the approved file." {
		t.Fatalf("unexpected response: %+v", response)
	}
	var sawError, sawApproval, awaitingResult bool
	for _, ev := range collected {
		switch ev.Type {
		case EventStreamError:
			sawError = true
		case EventStreamApprovalRequired:
			sawApproval = true
			if ev.Approval == nil || ev.Approval.Source != "stream" {
				t.Fatalf("approval payload missing source: %+v", ev.Approval)
			}
		case EventStreamToolResult:
			if ev.Status == "awaiting_approval" {
				awaitingResult = true
			}
		}
	}
	if sawError {
		t.Fatal("approval pause must not emit a stream error")
	}
	if !sawApproval || !awaitingResult {
		t.Fatalf("missing approval events: approval=%v awaiting=%v events=%v", sawApproval, awaitingResult, eventTypes(collected))
	}
	privateFile := filepath.Join(workspace, "agents", manifest.AgentID, "workspace", "approved.txt")
	data, readErr := os.ReadFile(privateFile)
	if readErr != nil || string(data) != "from-stream" {
		t.Fatalf("approved tool result mismatch: data=%q err=%v", data, readErr)
	}
}

func TestEngineStreamingApprovalRejectContinues(t *testing.T) {
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
	provider := llm.NewMockProvider("stream-reject-model", "")
	provider.StreamCompleteFunc = func(_ context.Context, messages []llm.Message, _ llm.CompletionOptions) (<-chan llm.StreamChunk, error) {
		ch := make(chan llm.StreamChunk, 3)
		if provider.StreamCompleteCalls == 1 {
			ch <- llm.StreamChunk{ToolCalls: []llm.ToolCall{{
				ID: "write-one", Type: "function",
				Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"denied.txt","content":"nope"}`)},
			}}, Usage: &llm.Usage{TotalTokens: 3}}
		} else {
			last := messages[len(messages)-1]
			if last.Role != llm.RoleTool || !strings.Contains(last.Content, "Operator rejected") {
				t.Errorf("rejected observation missing: %+v", last)
			}
			ch <- llm.StreamChunk{DeltaContent: "The operator declined that write.", Usage: &llm.Usage{TotalTokens: 4}}
		}
		ch <- llm.StreamChunk{Done: true}
		close(ch)
		return ch, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("stream-reject-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	engine.SetRunStore(NewRunStore(db.SQLDB()))
	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Streaming reject agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "stream-reject-model"},
		AuthorizedTools: []string{"native_file_write"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan AgentStreamEvent, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range events {
			if ev.Type == EventStreamApprovalRequired && ev.Approval != nil {
				if _, decideErr := approvalMgr.Decide(context.Background(), ev.Approval.ID, "rejected", "tester", "too risky"); decideErr != nil {
					t.Errorf("rejecting approval: %v", decideErr)
					return
				}
				engine.NotifyApprovalDecision(ev.Approval.ID, false, "too risky")
			}
		}
	}()
	response, err := engine.ExecuteStepStream(context.Background(), manifest.AgentID, "Write denied.txt", events)
	<-done
	if err != nil {
		t.Fatalf("reject should continue the stream, got %v", err)
	}
	if response == nil || response.Content != "The operator declined that write." {
		t.Fatalf("unexpected response: %+v", response)
	}
	denied := filepath.Join(workspace, "agents", manifest.AgentID, "workspace", "denied.txt")
	if _, statErr := os.Stat(denied); !os.IsNotExist(statErr) {
		t.Fatalf("rejected tool still executed: %v", statErr)
	}
}

func TestEngineStreamingApprovalDisconnectLeavesCheckpoint(t *testing.T) {
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
	provider := llm.NewMockProvider("stream-cancel-model", "")
	provider.StreamCompleteFunc = func(_ context.Context, _ []llm.Message, _ llm.CompletionOptions) (<-chan llm.StreamChunk, error) {
		ch := make(chan llm.StreamChunk, 3)
		ch <- llm.StreamChunk{ToolCalls: []llm.ToolCall{{
			ID: "write-one", Type: "function",
			Function: llm.FunctionCall{Name: "native_file_write", Arguments: json.RawMessage(`{"path":"paused.txt","content":"later"}`)},
		}}, Usage: &llm.Usage{TotalTokens: 2}}
		ch <- llm.StreamChunk{Done: true}
		close(ch)
		return ch, nil
	}
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("stream-cancel-model", provider)
	engine := NewEngine(agentMgr, eventBus, router, nil)
	engine.SetToolRegistry(registry)
	runStore := NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)
	manifest, err := agentMgr.Create(context.Background(), AgentManifest{
		Name: "Streaming cancel agent", Status: StatusActive,
		ModelConfig:     llm.ModelConfig{PrimaryModel: "stream-cancel-model"},
		AuthorizedTools: []string{"native_file_write"},
		DelegationScope: DelegationScope{
			AllowedWorkspacePaths: []string{"*"}, RequireHumanApproval: ApprovalHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan AgentStreamEvent, 64)
	sawApproval := make(chan struct{})
	go func() {
		for ev := range events {
			if ev.Type == EventStreamApprovalRequired {
				close(sawApproval)
			}
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		_, execErr := engine.ExecuteStepStream(ctx, manifest.AgentID, "Write paused.txt", events)
		errCh <- execErr
	}()
	select {
	case <-sawApproval:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval_required")
	}
	cancel()
	select {
	case execErr := <-errCh:
		var approvalErr *tools.ApprovalRequiredError
		if !errors.As(execErr, &approvalErr) {
			t.Fatalf("expected approval pause on disconnect, got %v", execErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not return after cancel")
	}
	runs, listErr := runStore.List(context.Background(), 10)
	if listErr != nil || len(runs) != 1 || runs[0].Status != RunApprovalPending {
		t.Fatalf("expected approval_pending run, got %+v err=%v", runs, listErr)
	}
	if engine.NotifyApprovalDecision("missing", true, "") {
		t.Fatal("NotifyApprovalDecision should be false without a waiter")
	}
}

func eventTypes(events []AgentStreamEvent) []StreamEventType {
	out := make([]StreamEventType, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Type)
	}
	return out
}

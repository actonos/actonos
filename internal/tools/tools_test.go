package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/bus"
)

func TestToolRegistry_RegisterAndExecute(t *testing.T) {
	eventBus := bus.NewEventBus()
	defer eventBus.Close()

	registry := NewToolRegistry(eventBus)

	// Register Native Tools
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	_ = os.MkdirAll(workspaceDir, 0755)

	RegisterNativeTools(registry, workspaceDir)

	tools := registry.List()
	if len(tools) < 4 {
		t.Fatalf("expected at least 4 native tools, got %d", len(tools))
	}

	// Test native_file_write
	ctx := context.Background()
	writeInput := json.RawMessage(`{"path": "test.txt", "content": "Hello ActonOS Tools"}`)
	res, err := registry.Execute(ctx, "test_agent", "native_file_write", writeInput)
	if err != nil {
		t.Fatalf("native_file_write failed: %v", err)
	}

	if res.Content == "" {
		t.Fatalf("expected write success content")
	}

	// Test native_file_read
	readInput := json.RawMessage(`{"path": "test.txt"}`)
	readRes, err := registry.Execute(ctx, "test_agent", "native_file_read", readInput)
	if err != nil {
		t.Fatalf("native_file_read failed: %v", err)
	}
	if readRes.Content != "Hello ActonOS Tools" {
		t.Fatalf("expected 'Hello ActonOS Tools', got '%s'", readRes.Content)
	}

	// Test Path Escape protection
	escapeInput := json.RawMessage(`{"path": "../../etc/passwd"}`)
	_, err = registry.Execute(ctx, "test_agent", "native_file_read", escapeInput)
	if err == nil {
		t.Fatal("expected path escape error, got nil")
	}

	// Test native_file_list
	listRes, err := registry.Execute(ctx, "test_agent", "native_file_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("native_file_list failed: %v", err)
	}
	if listRes.Content == "" {
		t.Fatal("expected file list content")
	}

	// Test native_file_search
	searchRes, err := registry.Execute(ctx, "test_agent", "native_file_search", json.RawMessage(`{"query": "ActonOS"}`))
	if err != nil {
		t.Fatalf("native_file_search failed: %v", err)
	}
	if searchRes.Content == "" {
		t.Fatal("expected search result content")
	}

	// Test native_exec
	execRes, err := registry.Execute(ctx, "test_agent", "native_exec", json.RawMessage(`{"command": "echo 'sandbox test'"}`))
	if err != nil {
		t.Fatalf("native_exec failed: %v", err)
	}
	if execRes.Content == "" {
		t.Fatal("expected exec result content")
	}

	// Test native_channel_notify
	notifyRes, err := registry.Execute(ctx, "test_agent", "native_channel_notify", json.RawMessage(`{"channel": "telegram", "message": "Test notification"}`))
	if err != nil {
		t.Fatalf("native_channel_notify failed: %v", err)
	}
	if notifyRes.Content == "" {
		t.Fatal("expected notify result content")
	}

	// Test native_file_delete
	delRes, err := registry.Execute(ctx, "test_agent", "native_file_delete", json.RawMessage(`{"path": "test.txt"}`))
	if err != nil {
		t.Fatalf("native_file_delete failed: %v", err)
	}
	if delRes.Content == "" {
		t.Fatal("expected delete result content")
	}
}

func TestToolRegistry_ToLLMToolDefinitions(t *testing.T) {
	registry := NewToolRegistry(nil)
	RegisterNativeTools(registry, t.TempDir())

	// Authorized subset
	defs := registry.ToLLMToolDefinitions([]string{"native_file_read", "native_sysinfo"})
	if len(defs) != 2 {
		t.Fatalf("expected 2 tool definitions, got %d", len(defs))
	}

	// Wildcard
	allDefs := registry.ToLLMToolDefinitions([]string{"*"})
	if len(allDefs) < 4 {
		t.Fatalf("expected all tools with wildcard, got %d", len(allDefs))
	}
}

func TestSkillWatcher_LoadSkill(t *testing.T) {
	tempDir := t.TempDir()
	skillsDir := filepath.Join(tempDir, "skills")
	testSkillDir := filepath.Join(skillsDir, "echo_skill")
	_ = os.MkdirAll(testSkillDir, 0755)

	manifest := SkillManifest{
		Name:        "echo_test",
		Description: "Echoes input back",
		Entrypoint:  "run.sh",
	}
	manifestBytes, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(testSkillDir, "skill.json"), manifestBytes, 0644)
	_ = os.WriteFile(filepath.Join(testSkillDir, "run.sh"), []byte("#!/bin/sh\ncat\n"), 0755)

	registry := NewToolRegistry(nil)
	watcher := NewSkillWatcher(registry, skillsDir)
	watcher.ScanAll()

	tool, err := registry.Get("skill_echo_test")
	if err != nil {
		t.Fatalf("expected skill_echo_test to be registered: %v", err)
	}

	if tool.Description() != "Echoes input back" {
		t.Fatalf("unexpected description: %s", tool.Description())
	}
}

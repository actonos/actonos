package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/llm"
)

func TestIsLongLivedHTTPPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/api/agents/agent_system_core/chat", want: true},
		{path: "/api/agents/agent_system_core/chat/stream", want: true},
		{path: "/api/realtime", want: true},
		{path: "/api/terminal/ws", want: true},
		{path: "/api/agents/", want: false},
		{path: "/api/dashboard/summary", want: false},
	}
	for _, tt := range tests {
		if got := isLongLivedHTTPPath(tt.path); got != tt.want {
			t.Errorf("isLongLivedHTTPPath(%q)=%v want %v", tt.path, got, tt.want)
		}
	}
}

func TestResolveStreamedAssistantContentPrefersFinalOverResetTokens(t *testing.T) {
	if got := resolveStreamedAssistantContent("", "Full report after 40 tools", "", nil); got != "Full report after 40 tools" {
		t.Fatalf("done fallback: %q", got)
	}
	if got := resolveStreamedAssistantContent("preamble", "Full report", "Clean final", nil); got != "Clean final" {
		t.Fatalf("response preferred: %q", got)
	}
	if got := resolveStreamedAssistantContent("", "", "", []llm.ToolCall{{ID: "1"}}); got != completedOperationsFallback {
		t.Fatalf("tool-only fallback: %q", got)
	}
}

func TestPersistChatAssistantMessageSurvivesCanceledRequestContext(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now().UTC()
	convID := "conv_persist_cancel"
	if _, err := srv.memory.DB().SQLDB().Exec(`
		INSERT INTO conversations (id, agent_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`, convID, agent.DefaultSystemAgentID, "Report", now, now); err != nil {
		t.Fatal(err)
	}
	srv.persistChatAssistantMessage(convID, agent.DefaultSystemAgentID, "Enterprise AI report body", nil)

	var content string
	if err := srv.memory.DB().SQLDB().QueryRow(
		`SELECT content FROM messages WHERE conversation_id = ? AND role = 'assistant'`,
		convID,
	).Scan(&content); err != nil {
		t.Fatalf("assistant message missing: %v", err)
	}
	if content != "Enterprise AI report body" {
		t.Fatalf("content=%q", content)
	}
}

func TestHandleChatStreamTruePersistsAssistantReply(t *testing.T) {
	srv := newTestServer(t)
	create := httptest.NewRequest(http.MethodPost, "/api/agents/", strings.NewReader(`{
		"name":"Persist Agent",
		"model_config":{"primary_model":"test-model"},
		"system_instructions":"Answer directly.",
		"authorized_tools":[]
	}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	srv.Router().ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create agent: %d %s", created.Code, created.Body.String())
	}
	var createdBody struct {
		Data agent.AgentManifest `json:"data"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createdBody); err != nil {
		t.Fatal(err)
	}
	agentID := createdBody.Data.AgentID

	chat := httptest.NewRequest(http.MethodPost, "/api/agents/"+agentID+"/chat", strings.NewReader(`{"message":"write the report","stream":true}`))
	chat.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	srv.Router().ServeHTTP(result, chat)
	if result.Code != http.StatusOK {
		t.Fatalf("stream chat: %d %s", result.Code, result.Body.String())
	}
	if !strings.Contains(result.Body.String(), "event: done") {
		t.Fatalf("missing done event: %s", result.Body.String())
	}

	var count int
	var content string
	if err := srv.memory.DB().SQLDB().QueryRow(`
		SELECT COUNT(*), COALESCE(MAX(content), '')
		FROM messages
		WHERE agent_id = ? AND role = 'assistant'
	`, agentID).Scan(&count, &content); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("persisted %d assistant messages, want 1 (got %q)", count, content)
	}
	if strings.TrimSpace(content) == "" {
		t.Fatal("assistant content was empty")
	}
}

func TestPersistStreamedAssistantIfNeededSavesDoneContentOnEngineError(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now().UTC()
	convID := "conv_done_fallback"
	if _, err := srv.memory.DB().SQLDB().Exec(`
		INSERT INTO conversations (id, agent_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`, convID, agent.DefaultSystemAgentID, "Report", now, now); err != nil {
		t.Fatal(err)
	}

	srv.persistStreamedAssistantIfNeeded(
		convID,
		agent.DefaultSystemAgentID,
		"",
		"Full report after many tool loops",
		nil,
		nil,
		errors.New("context canceled"),
	)

	var content string
	if err := srv.memory.DB().SQLDB().QueryRow(
		`SELECT content FROM messages WHERE conversation_id = ? AND role = 'assistant'`,
		convID,
	).Scan(&content); err != nil {
		t.Fatalf("assistant message missing: %v", err)
	}
	if content != "Full report after many tool loops" {
		t.Fatalf("content=%q", content)
	}
}

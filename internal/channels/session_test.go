package channels

import (
	"context"
	"database/sql"
	"testing"

	"github.com/actonos/actonos/internal/llm"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		title TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		tool_calls_json TEXT,
		created_at DATETIME NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}
	return db
}

func TestChannelSessionManager_GetOrCreateAndHistory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sm := NewChannelSessionManager(db)
	ctx := context.Background()

	convID, err := sm.GetOrCreateSession(ctx, "telegram", "987654321", "Alice", "Xin chào Agent", "agent_system_core")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if convID != "conv_telegram_987654321" {
		t.Errorf("expected conv_telegram_987654321, got %s", convID)
	}

	// Save user message
	if err := sm.SaveMessage(ctx, convID, "agent_system_core", "user", "Xin chào Agent", nil); err != nil {
		t.Fatalf("failed to save user message: %v", err)
	}

	// Save assistant reply
	if err := sm.SaveMessage(ctx, convID, "agent_system_core", "assistant", "Chào bạn, tôi có thể giúp gì?", nil); err != nil {
		t.Fatalf("failed to save assistant message: %v", err)
	}

	// Retrieve history
	history := sm.LoadRecentHistory(ctx, convID, 5)
	if len(history) != 2 {
		t.Fatalf("expected 2 messages in history, got %d", len(history))
	}
	if history[0].Role != llm.RoleUser || history[0].Content != "Xin chào Agent" {
		t.Errorf("unexpected first history message: %+v", history[0])
	}
	if history[1].Role != llm.RoleAssistant || history[1].Content != "Chào bạn, tôi có thể giúp gì?" {
		t.Errorf("unexpected second history message: %+v", history[1])
	}
}

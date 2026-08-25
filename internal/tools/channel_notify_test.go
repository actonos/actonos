package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

type envelopeCapture struct {
	envs []ChannelEnvelope
}

func (e *envelopeCapture) Send(ctx context.Context, channelID, accountID, recipient, content string) error {
	e.envs = append(e.envs, ChannelEnvelope{ChannelID: channelID, AccountID: accountID, Recipient: recipient, Content: content})
	return nil
}

func (e *envelopeCapture) SendEnvelope(ctx context.Context, env ChannelEnvelope) error {
	e.envs = append(e.envs, env)
	return nil
}

func TestChannelNotifyToolEnvelopeKinds(t *testing.T) {
	cap := &envelopeCapture{}
	tool := NewChannelNotifyTool(nil)
	tool.SetSender(cap)

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram","kind":"typing","chat_id":"888"}`)); err != nil {
		t.Fatalf("typing: %v", err)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram","kind":"reaction","reaction":"👀","reply_to_id":"42","chat_id":"888"}`)); err != nil {
		t.Fatalf("reaction: %v", err)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram","message":"hi","reply_to_id":"42","chat_id":"888"}`)); err != nil {
		t.Fatalf("text: %v", err)
	}
	if len(cap.envs) != 3 {
		t.Fatalf("envs=%d", len(cap.envs))
	}
	if cap.envs[0].Kind != "typing" || !cap.envs[0].Typing {
		t.Fatalf("typing env=%+v", cap.envs[0])
	}
	if cap.envs[1].Kind != "reaction" || cap.envs[1].Reaction != "👀" || cap.envs[1].ReplyToID != "42" {
		t.Fatalf("reaction env=%+v", cap.envs[1])
	}
	if cap.envs[2].Kind != "text" || cap.envs[2].Content != "hi" || cap.envs[2].ReplyToID != "42" {
		t.Fatalf("text env=%+v", cap.envs[2])
	}
}

func TestChannelNotifyToolRequiresMessageForText(t *testing.T) {
	tool := NewChannelNotifyTool(nil)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram"}`)); err == nil {
		t.Fatal("expected error when text kind has no message")
	}
}

func TestChannelNotifyToolSendsWorkspaceFile(t *testing.T) {
	registry, store, _ := newWorkspaceToolRegistry(t)
	ctx := context.Background()
	node, err := store.Write(ctx, workspacepkg.WriteRequest{
		Name: "report.pdf", Content: []byte("%PDF-1.7 demo"), MIMEType: "application/pdf", ActorID: "agent_alpha",
	})
	if err != nil {
		t.Fatalf("writing workspace pdf: %v", err)
	}
	cap := &envelopeCapture{}
	tool, ok := registry.tools["native_channel_notify"].(*ChannelNotifyTool)
	if !ok {
		t.Fatal("native_channel_notify missing")
	}
	tool.SetSender(cap)

	payload, _ := json.Marshal(map[string]string{
		"channel": "telegram",
		"chat_id": "888",
		"message": "Báo cáo Q2",
		"path":    "report.pdf",
	})
	result, err := tool.Execute(ctx, payload)
	if err != nil {
		t.Fatalf("notify file: %v", err)
	}
	if len(cap.envs) != 1 {
		t.Fatalf("envs=%d", len(cap.envs))
	}
	got := cap.envs[0]
	if got.Kind != "media" || got.FileName != "report.pdf" || string(got.FileBytes) != "%PDF-1.7 demo" {
		t.Fatalf("envelope=%+v bytes=%q", got, got.FileBytes)
	}
	if got.ChatID != "888" || got.Content != "Báo cáo Q2" {
		t.Fatalf("chat/caption=%+v", got)
	}
	if !strings.Contains(result.Content, "report.pdf") {
		t.Fatalf("result=%q", result.Content)
	}
	_ = node
}

func TestChannelNotifyToolRejectsAllChannelForFiles(t *testing.T) {
	registry, store, _ := newWorkspaceToolRegistry(t)
	ctx := context.Background()
	if _, err := store.Write(ctx, workspacepkg.WriteRequest{Name: "a.txt", Content: []byte("x"), ActorID: "agent_alpha"}); err != nil {
		t.Fatal(err)
	}
	tool := registry.tools["native_channel_notify"].(*ChannelNotifyTool)
	tool.SetSender(&envelopeCapture{})
	_, err := tool.Execute(ctx, json.RawMessage(`{"channel":"all","path":"a.txt","chat_id":"1"}`))
	if err == nil {
		t.Fatal("expected error when sending a file to channel=all")
	}
}

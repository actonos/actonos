package tools

import (
	"context"
	"encoding/json"
	"testing"
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

package channels

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInboundNormalizeAliases(t *testing.T) {
	msg := InboundMessage{
		ChannelID: "telegram",
		SenderID:  "99",
		Content:   "hello",
		Metadata: map[string]string{
			"chat_id":    "888",
			"message_id": "42",
		},
	}
	msg.Normalize()
	if msg.ChatID != "888" {
		t.Fatalf("ChatID=%q", msg.ChatID)
	}
	if msg.MessageID != "42" {
		t.Fatalf("MessageID=%q", msg.MessageID)
	}
	if msg.Kind != MessageKindText {
		t.Fatalf("Kind=%q", msg.Kind)
	}
	if msg.Metadata["channel_id"] != "888" {
		t.Fatalf("channel_id alias=%q", msg.Metadata["channel_id"])
	}
}

func TestNewReplyAndTyping(t *testing.T) {
	in := InboundMessage{
		ChannelID: "discord",
		AccountID: "bot_cskh",
		SenderID:  "user_1",
		ChatID:    "chan-9",
		MessageID: "m-1",
		Content:   "help",
	}
	reply := NewReply(in, "done")
	if reply.Kind != MessageKindText {
		t.Fatalf("reply kind=%q", reply.Kind)
	}
	if reply.ReplyToID != "m-1" {
		t.Fatalf("reply_to_id=%q", reply.ReplyToID)
	}
	if reply.ChatID != "chan-9" || reply.Recipient != "chan-9" {
		t.Fatalf("chat=%q recipient=%q", reply.ChatID, reply.Recipient)
	}
	if reply.Metadata["reply_to_msg_id"] != "m-1" {
		t.Fatalf("metadata alias missing: %+v", reply.Metadata)
	}

	typing := NewTyping(in)
	if typing.Kind != MessageKindTyping || !typing.Typing {
		t.Fatalf("typing=%+v", typing)
	}

	react := NewReaction(in, "👀")
	if react.Kind != MessageKindReaction || react.Reaction != "👀" || react.ReplyToID != "m-1" {
		t.Fatalf("reaction=%+v", react)
	}
}

func TestOutboundFileDataIsMediaForPlugins(t *testing.T) {
	msg := OutboundMessage{
		ChannelID: "telegram",
		Recipient: "888",
		Content:   "caption",
		FileName:  "report.pdf",
		MIMEType:  "application/pdf",
		FileData:  []byte("%PDF-1.7"),
	}
	msg.Normalize()
	if msg.Kind != MessageKindMedia {
		t.Fatalf("kind=%q", msg.Kind)
	}
	if !msg.HasFile() {
		t.Fatal("expected HasFile")
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	if !strings.Contains(encoded, `"file_name":"report.pdf"`) || !strings.Contains(encoded, `"file_data":`) {
		t.Fatalf("plugin JSON missing file fields: %s", encoded)
	}
	var roundtrip OutboundMessage
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if string(roundtrip.FileData) != "%PDF-1.7" {
		t.Fatalf("plugin did not receive file bytes: %q", roundtrip.FileData)
	}
}

func TestControlEventSkipped(t *testing.T) {
	typing := InboundMessage{Kind: MessageKindTyping}
	if !typing.IsControlEvent() {
		t.Fatal("expected typing inbound to be control")
	}
	text := InboundMessage{Kind: MessageKindText, Content: "hi"}
	if text.IsControlEvent() {
		t.Fatal("text is not control")
	}
}

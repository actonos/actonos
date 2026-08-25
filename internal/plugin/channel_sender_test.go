package plugin

import (
	"context"
	"testing"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/tools"
)

func TestChannelToolSenderEnvelope(t *testing.T) {
	eb := bus.NewEventBus()
	defer eb.Close()
	mgr := channels.NewChannelManager(eb, nil)
	defer mgr.Stop()

	adapter := &recordingChannelAdapter{name: "telegram"}
	if err := mgr.RegisterAdapter(adapter); err != nil {
		t.Fatal(err)
	}

	sender := ChannelToolSender(mgr)
	envSender, ok := sender.(tools.ChannelEnvelopeSender)
	if !ok {
		t.Fatal("expected ChannelEnvelopeSender")
	}
	err := envSender.SendEnvelope(context.Background(), tools.ChannelEnvelope{
		ChannelID: "telegram",
		AccountID: "tg_1",
		Recipient: "888",
		Kind:      "reaction",
		ChatID:    "888",
		ReplyToID: "42",
		Reaction:  "👀",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.sent) != 1 {
		t.Fatalf("sent=%d", len(adapter.sent))
	}
	got := adapter.sent[0]
	if got.Kind != channels.MessageKindReaction || got.Reaction != "👀" || got.ReplyToID != "42" {
		t.Fatalf("outbound=%+v", got)
	}
}

type recordingChannelAdapter struct {
	name string
	sent []channels.OutboundMessage
}

func (a *recordingChannelAdapter) Name() string                { return a.name }
func (a *recordingChannelAdapter) Start(context.Context) error { return nil }
func (a *recordingChannelAdapter) Stop() error                 { return nil }
func (a *recordingChannelAdapter) SendMessage(_ context.Context, msg channels.OutboundMessage) error {
	a.sent = append(a.sent, msg)
	return nil
}

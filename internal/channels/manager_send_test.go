package channels

import (
	"context"
	"strings"
	"testing"
)

func TestSendMessageToUnregisteredChannelFails(t *testing.T) {
	mgr := NewChannelManager(nil, nil)
	defer mgr.Stop()
	adapter := &mockAdapter{name: "telegram"}
	if err := mgr.RegisterAdapter(adapter); err != nil {
		t.Fatal(err)
	}

	err := mgr.Send(context.Background(), "telegrm", "", "42", "typo in channel name")
	if err == nil || !strings.Contains(err.Error(), "no adapter registered") {
		t.Fatalf("a message addressed to a missing channel must not report success, got %v", err)
	}
	if len(adapter.sentMsgs) != 0 {
		t.Fatalf("message leaked to a different adapter: %+v", adapter.sentMsgs)
	}
}

func TestSendMessageBroadcastWithoutAdaptersIsNotAnError(t *testing.T) {
	mgr := NewChannelManager(nil, nil)
	defer mgr.Stop()
	if err := mgr.Send(context.Background(), "", "", "", "broadcast"); err != nil {
		t.Fatalf("broadcast with no adapters should be a no-op, got %v", err)
	}
}

func TestSendMessageIsCaseInsensitive(t *testing.T) {
	mgr := NewChannelManager(nil, nil)
	defer mgr.Stop()
	adapter := &mockAdapter{name: "telegram"}
	if err := mgr.RegisterAdapter(adapter); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Send(context.Background(), " Telegram ", "", "42", "hi"); err != nil {
		t.Fatalf("channel lookup should ignore case and whitespace: %v", err)
	}
	if len(adapter.sentMsgs) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(adapter.sentMsgs))
	}
}

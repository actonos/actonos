package channels

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

type fakeEngine struct {
	calls int
}

func (f *fakeEngine) ExecuteStepWithHistory(context.Context, string, string, []llm.Message) (*llm.Response, error) {
	f.calls++
	return &llm.Response{Content: "ok"}, nil
}

func TestUnpairedInboundRejectedWhenRequired(t *testing.T) {
	dir := t.TempDir()
	db, err := memory.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	pm, err := NewPairingManager(db.SQLDB())
	if err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewEventBus()
	cm := NewChannelManager(eventBus, pm)
	_ = cm.SyncAccounts(context.Background(), []ChannelAccount{{
		ID: "tg1", Channel: "telegram", Enabled: true, RequiresPairing: true, BoundAgentIDs: []string{"*"},
	}})
	engine := &fakeEngine{}
	agentMgr, err := agent.NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewChannelSessionManager(db.SQLDB())
	router := NewMessageRouter(cm, agentMgr, sessions, engine, eventBus)
	router.SetPairingManager(pm)
	queue, err := NewInboundQueue(db.SQLDB())
	if err != nil {
		t.Fatal(err)
	}
	router.SetInboundQueue(queue)

	msg := InboundMessage{
		ChannelID: "telegram", AccountID: "tg1", SenderID: "u1", SenderName: "U", Content: "hello",
	}
	if _, err := queue.Enqueue(msg); err != nil {
		t.Fatal(err)
	}
	err = router.Route(context.Background(), msg)
	if err == nil {
		t.Fatal("expected unpaired inbound to be rejected")
	}
	if engine.calls != 0 {
		t.Fatal("engine must not run for unpaired sender")
	}
	n, err := queue.CountPending()
	if err != nil || n == 0 {
		t.Fatalf("inbound must be persisted even when unpaired: n=%d err=%v", n, err)
	}
}

func TestPersistedInboundSurvivesFullBus(t *testing.T) {
	dir := t.TempDir()
	db, err := memory.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	queue, err := NewInboundQueue(db.SQLDB())
	if err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewEventBus()
	t.Cleanup(func() { eventBus.Close() })
	eventBus.SetPersist(queue.PersistEvent)
	// Subscribe and never read so the buffer fills and extra publishes drop.
	_ = eventBus.Subscribe(bus.EventChannelMessage)
	fill := bus.DefaultChannelBufferSize + 8
	for i := 0; i < fill; i++ {
		eventBus.Publish(bus.NewEvent(bus.EventChannelMessage, "discord", InboundMessage{
			ChannelID: "discord", SenderID: "s", Content: "ping",
		}))
	}
	if eventBus.DroppedEvents() == 0 {
		t.Fatal("expected EventBus to drop once the subscriber buffer is full")
	}
	n, err := queue.CountPending()
	if err != nil || n < fill {
		t.Fatalf("persist-before-publish must keep every inbound, pending=%d want>=%d err=%v", n, fill, err)
	}
	pending, err := queue.Pending(fill + 10)
	if err != nil || len(pending) < fill {
		t.Fatalf("expected persisted inbound after a full bus, got %d err=%v", len(pending), err)
	}
}

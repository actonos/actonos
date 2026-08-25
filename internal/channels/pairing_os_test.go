package channels

import (
	"context"
	"path/filepath"
	"sync"
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
	pending := pm.ListPending()
	if len(pending) != 1 || pending[0].SenderID != "u1" {
		t.Fatalf("expected pending unpaired sender, got %+v", pending)
	}
	n, err := queue.CountPending()
	if err != nil || n == 0 {
		t.Fatalf("inbound must be persisted even when unpaired: n=%d err=%v", n, err)
	}
}

type recordingAdapter struct {
	name string
	mu   sync.Mutex
	sent []OutboundMessage
}

func (a *recordingAdapter) Name() string                { return a.name }
func (a *recordingAdapter) Start(context.Context) error { return nil }
func (a *recordingAdapter) Stop() error                 { return nil }
func (a *recordingAdapter) SendMessage(_ context.Context, msg OutboundMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, msg)
	return nil
}

func (a *recordingAdapter) snapshot() []OutboundMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]OutboundMessage, len(a.sent))
	copy(out, a.sent)
	return out
}

func TestPairingPINFromChatAuthorizesWithoutAccountID(t *testing.T) {
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
	code, err := pm.GeneratePairingCode("telegram")
	if err != nil {
		t.Fatal(err)
	}
	if err := pm.SetChannelRequiresPairing("telegram", true); err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewEventBus()
	cm := NewChannelManager(eventBus, pm)
	adapter := &recordingAdapter{name: "telegram"}
	if err := cm.RegisterDynamicAdapter(adapter); err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{}
	agentMgr, err := agent.NewAgentManager(db, eventBus)
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewChannelSessionManager(db.SQLDB())
	router := NewMessageRouter(cm, agentMgr, sessions, engine, eventBus)
	router.SetPairingManager(pm)

	unpaired := InboundMessage{ChannelID: "telegram", SenderID: "u2", SenderName: "Bo", Content: "hi"}
	if err := router.Route(context.Background(), unpaired); err == nil {
		t.Fatal("expected unpaired rejection")
	}
	if len(adapter.sent) == 0 {
		t.Fatal("expected pairing instructions sent to chat")
	}
	paired := InboundMessage{ChannelID: "telegram", SenderID: "u2", SenderName: "Bo", Content: "/pair " + code}
	if err := router.Route(context.Background(), paired); err != nil {
		t.Fatalf("pin should pair: %v", err)
	}
	if !pm.IsAuthorized("telegram", "u2") {
		t.Fatal("sender should be authorized after /pair")
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

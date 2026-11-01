package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/tetratelabs/wazero"
)

// The guest ABI is exercised against a tiny hand-assembled module so the tests
// need no wasm toolchain. It exports memory, acton_alloc (always returns a fixed
// scratch address) and the tool / channel / connector entry points, each of
// which returns a canned payload stored in a data segment.

const (
	abiToolResultPtr    = 512
	abiPollResultPtr    = 640
	abiWebhookResultPtr = 768
)

const (
	abiToolResult    = "pong"
	abiPollResult    = `{"content":"hello from plugin","sender_id":"user-1"}`
	abiWebhookResult = `{"ok":true}`
)

func uleb(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			out = append(out, b|0x80)
			continue
		}
		return append(out, b)
	}
}

func sleb(v int64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func section(id byte, content []byte) []byte {
	return append(append([]byte{id}, uleb(uint64(len(content)))...), content...)
}

func vec(items ...[]byte) []byte {
	out := uleb(uint64(len(items)))
	for _, item := range items {
		out = append(out, item...)
	}
	return out
}

func name(s string) []byte { return append(uleb(uint64(len(s))), []byte(s)...) }

func funcBody(instr ...byte) []byte {
	body := append([]byte{0x00}, instr...) // no locals
	body = append(body, 0x0b)
	return append(uleb(uint64(len(body))), body...)
}

func i32Const(v int64) []byte { return append([]byte{0x41}, sleb(v)...) }
func i64Const(v int64) []byte { return append([]byte{0x42}, sleb(v)...) }

func packed(ptr, length int) int64 { return int64(ptr)<<32 | int64(length) }

// buildABIModule returns a module whose acton_channel_send returns sendCode.
func buildABIModule(sendCode int64) []byte {
	const (
		i32, i64 = 0x7f, 0x7e
	)
	types := vec(
		[]byte{0x60, 0x01, i32, 0x01, i32},                // 0: (i32) -> i32          alloc
		[]byte{0x60, 0x04, i32, i32, i32, i32, 0x01, i64}, // 1: (i32 x4) -> i64       tool execute
		[]byte{0x60, 0x02, i32, i32, 0x01, i32},           // 2: (i32, i32) -> i32     channel send
		[]byte{0x60, 0x00, 0x01, i64},                     // 3: () -> i64             channel poll
		[]byte{0x60, 0x02, i32, i32, 0x01, i64},           // 4: (i32, i32) -> i64     webhook
	)
	funcs := vec([]byte{0}, []byte{1}, []byte{2}, []byte{3}, []byte{4})
	mem := vec([]byte{0x00, 0x01}) // one page, no max
	exports := vec(
		append(name("memory"), 0x02, 0x00),
		append(name("acton_alloc"), 0x00, 0x00),
		append(name("acton_tool_execute"), 0x00, 0x01),
		append(name("acton_channel_send"), 0x00, 0x02),
		append(name("acton_channel_poll"), 0x00, 0x03),
		append(name("acton_connector_handle_webhook"), 0x00, 0x04),
	)
	code := vec(
		funcBody(i32Const(100)...),
		funcBody(i64Const(packed(abiToolResultPtr, len(abiToolResult)))...),
		funcBody(i32Const(sendCode)...),
		funcBody(i64Const(packed(abiPollResultPtr, len(abiPollResult)))...),
		funcBody(i64Const(packed(abiWebhookResultPtr, len(abiWebhookResult)))...),
	)
	segment := func(ptr int, data string) []byte {
		seg := append([]byte{0x00}, i32Const(int64(ptr))...)
		seg = append(seg, 0x0b)
		seg = append(seg, uleb(uint64(len(data)))...)
		return append(seg, data...)
	}
	data := vec(
		segment(abiToolResultPtr, abiToolResult),
		segment(abiPollResultPtr, abiPollResult),
		segment(abiWebhookResultPtr, abiWebhookResult),
	)

	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, s := range [][]byte{
		section(1, types), section(3, funcs), section(5, mem),
		section(7, exports), section(10, code), section(11, data),
	} {
		out = append(out, s...)
	}
	return out
}

func newABIInstance(t *testing.T, sendCode int64) *PluginInstance {
	t.Helper()
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, buildABIModule(sendCode))
	if err != nil {
		t.Fatalf("instantiating ABI test module: %v", err)
	}
	host := &HostContext{
		PluginID: "abi",
		AllocFn:  mod.ExportedFunction("acton_alloc"),
		Memory:   mod.Memory(),
	}
	return &PluginInstance{pluginID: "abi", mod: mod, hostCtx: host}
}

func TestPluginInstanceToolExecution(t *testing.T) {
	inst := newABIInstance(t, 0)
	got, err := inst.ExecuteTool(context.Background(), "echo", json.RawMessage(`{"a":1}`))
	if err != nil || got != abiToolResult {
		t.Fatalf("ExecuteTool = %q, %v; want %q", got, err, abiToolResult)
	}
}

func TestPluginInstanceChannelSend(t *testing.T) {
	ok := newABIInstance(t, 0)
	if err := ok.SendChannelMessage(context.Background(), []byte(`{"content":"hi"}`)); err != nil {
		t.Fatalf("send should succeed on code 0: %v", err)
	}

	failing := newABIInstance(t, 5)
	err := failing.SendChannelMessage(context.Background(), []byte(`{"content":"hi"}`))
	if err == nil {
		t.Fatal("a non-zero guest return code must surface as an error")
	}
}

func TestPluginInstancePollChannel(t *testing.T) {
	inst := newABIInstance(t, 0)
	got, err := inst.PollChannel(context.Background())
	if err != nil || string(got) != abiPollResult {
		t.Fatalf("PollChannel = %q, %v; want %q", got, err, abiPollResult)
	}
}

func TestToolBridgeExecutesGuestTool(t *testing.T) {
	inst := newABIInstance(t, 0)
	tool := NewWasmToolBridge("abi", PluginToolDef{Name: "echo", Description: "Echo"}, inst)
	got, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got == nil || got.Content != abiToolResult || got.Error != "" {
		t.Fatalf("unexpected tool result: %+v", got)
	}
}

func TestConnectorBridgeForwardsWebhook(t *testing.T) {
	inst := newABIInstance(t, 0)
	conn := NewWasmConnectorBridge("abi", PluginManifest{Connectors: []PluginConnectorDef{{Name: "svc"}}}, inst)
	got, err := conn.HandleWebhook(context.Background(), []byte(`{"event":"push"}`))
	if err != nil || string(got) != abiWebhookResult {
		t.Fatalf("HandleWebhook = %q, %v; want %q", got, err, abiWebhookResult)
	}
}

func TestChannelBridgePublishesPolledMessages(t *testing.T) {
	eb := bus.NewEventBus()
	defer eb.Close()
	events := eb.Subscribe(bus.EventChannelMessage)

	inst := newABIInstance(t, 0)
	manifest := PluginManifest{ID: "abi", Channels: []PluginChannelDef{{Name: "demo"}}}
	bridge := NewWasmChannelBridge("abi", manifest, inst, eb)
	bridge.inst = inst
	bridge.pollOnce(context.Background())

	select {
	case ev := <-events:
		msg, ok := ev.Payload.(channels.InboundMessage)
		if !ok {
			t.Fatalf("event payload is %T, want channels.InboundMessage", ev.Payload)
		}
		if msg.Content != "hello from plugin" || msg.ChannelID != "demo" {
			t.Fatalf("unexpected inbound message: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("polled message was not published to the event bus")
	}

	// Sending through the bridge reaches the guest and Stop() deactivates it.
	if err := bridge.SendMessage(context.Background(), channels.OutboundMessage{Content: "out"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if err := bridge.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := bridge.SendMessage(context.Background(), channels.OutboundMessage{Content: "late"}); err == nil {
		t.Fatal("sending after Stop must fail")
	}
}

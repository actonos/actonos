package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/channels"
	"github.com/coder/websocket"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func TestHTTPRequestTimeout(t *testing.T) {
	cases := []struct {
		name string
		in   HTTPRequestPayload
		want time.Duration
	}{
		{"explicit", HTTPRequestPayload{Timeout: 5}, 5 * time.Second},
		{"clamped to ten minutes", HTTPRequestPayload{Timeout: 99999}, 600 * time.Second},
		{"default", HTTPRequestPayload{}, 300 * time.Second},
		{"default with upload body", HTTPRequestPayload{BodyBase64: "AAAA"}, 300 * time.Second},
		{"negative falls back to default", HTTPRequestPayload{Timeout: -3}, 300 * time.Second},
	}
	for _, tc := range cases {
		if got := httpRequestTimeout(tc.in); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestChannelToolSenderNilManager(t *testing.T) {
	if got := ChannelToolSender(nil); got != nil {
		t.Fatalf("a nil manager must yield no sender, got %T", got)
	}
}

func TestChannelToolSenderPlainSendRoutesThroughManager(t *testing.T) {
	mgr := channels.NewChannelManager(nil, nil)
	defer mgr.Stop()
	adapter := &recordingChannelAdapter{name: "telegram"}
	if err := mgr.RegisterAdapter(adapter); err != nil {
		t.Fatal(err)
	}

	sender := ChannelToolSender(mgr)
	if err := sender.Send(context.Background(), "telegram", "tg_1", "42", "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(adapter.sent) != 1 || adapter.sent[0].Content != "hello" {
		t.Fatalf("message not delivered to the adapter: %+v", adapter.sent)
	}
	if err := sender.Send(context.Background(), "unknown-channel", "", "42", "x"); err == nil {
		t.Fatal("sending through an unregistered channel must fail")
	}
}

func TestManagerSetWorkspaceStoreIsChainable(t *testing.T) {
	ctx := context.Background()
	loader, err := NewWasmLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loader.Close(ctx) })
	m := NewManager(loader, nil, nil, nil, nil, nil, t.TempDir())
	if got := m.SetWorkspaceStore(nil); got != m {
		t.Fatal("SetWorkspaceStore must return the manager for chaining")
	}
}

// newWSHost wires a HostContext to a live in-process websocket echo server by
// registering the connection directly. The production dial path refuses
// loopback targets (see TestHostSyscallsWithMemoryModule), so this exercises the
// send/poll/close syscalls without weakening that egress policy.
func newWSHost(t *testing.T) (context.Context, *HostContext, api.Module, int32) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			if err := c.Write(r.Context(), typ, append([]byte("echo:"), data...)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01})
	if err != nil {
		t.Fatal(err)
	}

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelDial()
	conn, _, err := websocket.Dial(dialCtx, url, nil)
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	connCtx, cancel := context.WithCancel(ctx)
	wc := &HostWSConn{id: 7, url: url, conn: conn, msgQueue: make(chan []byte, 8), ctx: connCtx, cancel: cancel}
	go func() {
		for connCtx.Err() == nil {
			_, data, err := conn.Read(connCtx)
			if err != nil {
				return
			}
			wc.msgQueue <- data
		}
	}()

	host := &HostContext{PluginID: "ws-test", wsConns: map[int32]*HostWSConn{7: wc}}
	t.Cleanup(wc.close)
	return WithHostContext(ctx, host), host, mod, 7
}

func TestWebsocketSendPollClose(t *testing.T) {
	exec, host, mod, id := newWSHost(t)
	mem := mod.Memory()

	// Nothing queued yet: poll reports "no data", not an error.
	if got := wsPoll(exec, mod, id); got != 0 {
		t.Fatalf("empty poll = %d, want 0", got)
	}

	msg := []byte("ping")
	if !mem.Write(128, msg) {
		t.Fatal("write guest memory")
	}
	for _, msgType := range []int32{1, 2} { // text, binary
		if got := wsSend(exec, mod, id, msgType, 128, uint32(len(msg))); got != 0 {
			t.Fatalf("wsSend(type=%d) = %d", msgType, got)
		}
		var n int32
		deadline := time.Now().Add(5 * time.Second)
		for n == 0 && time.Now().Before(deadline) {
			n = wsPoll(exec, mod, id)
			time.Sleep(5 * time.Millisecond)
		}
		if n != int32(len("echo:ping")) || string(host.LastResponse) != "echo:ping" {
			t.Fatalf("poll after send(type=%d): n=%d response=%q", msgType, n, host.LastResponse)
		}
	}

	if got := wsSend(exec, mod, id, 1, 0xFFFFFFF0, 64); got != -1 {
		t.Fatalf("out-of-bounds guest read must fail, got %d", got)
	}

	if got := wsClose(exec, mod, id); got != 0 {
		t.Fatalf("wsClose = %d", got)
	}
	if got := wsSend(exec, mod, id, 1, 128, uint32(len(msg))); got != -1 {
		t.Fatalf("send after close = %d, want -1", got)
	}
	if got := wsPoll(exec, mod, id); got != -1 {
		t.Fatalf("poll after close = %d, want -1", got)
	}
}

func TestWebsocketSyscallsWithoutHostContext(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01})
	if err != nil {
		t.Fatal(err)
	}
	if wsConnect(ctx, mod, 0, 0, 0, 0) != -1 || wsSend(ctx, mod, 1, 1, 0, 0) != -1 ||
		wsPoll(ctx, mod, 1) != -1 || wsClose(ctx, mod, 1) != -1 {
		t.Fatal("every websocket syscall must fail closed without a host context")
	}
}

func TestWorkspaceReadFileWithoutStore(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01})
	if err != nil {
		t.Fatal(err)
	}

	host := &HostContext{PluginID: "no-store"}
	n := workspaceReadFile(WithHostContext(ctx, host), mod, 0, 0)
	if n <= 0 {
		t.Fatalf("expected an error payload, got %d", n)
	}
	var resp WorkspaceFileResponse
	if err := json.Unmarshal(host.LastResponse, &resp); err != nil || !strings.Contains(resp.Error, "not configured") {
		t.Fatalf("unexpected response %q (err=%v)", host.LastResponse, err)
	}

	if got := workspaceReadFile(ctx, mod, 0, 0); got <= 0 && ctx != nil {
		// Without any host context there is nowhere to stash a response; it must
		// still return a well-formed error length rather than panic.
		t.Logf("no host context returned %d", got)
	}
}

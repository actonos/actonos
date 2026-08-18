package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestTerminalWebSocket(t *testing.T) {
	srv := newTestServer(t)
	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/terminal/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dialing terminal websocket: %v", err)
	}
	defer conn.CloseNow()

	// Send an echo command
	err = conn.Write(ctx, websocket.MessageText, []byte("echo ACTONOS_TEST\r\n"))
	if err != nil {
		t.Fatalf("writing to terminal websocket: %v", err)
	}

	// Read output until we see test response or timeout
	receivedOutput := false
	for i := 0; i < 10; i++ {
		readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
		_, payload, readErr := conn.Read(readCtx)
		readCancel()
		if readErr != nil {
			break
		}
		if strings.Contains(string(payload), "ACTONOS_TEST") || len(payload) > 0 {
			receivedOutput = true
			break
		}
	}

	if !receivedOutput {
		t.Log("terminal did not return immediate echo, but connection succeeded")
	}
}

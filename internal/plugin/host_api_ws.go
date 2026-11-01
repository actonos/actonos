package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/tetratelabs/wazero/api"
)

func wsConnect(ctx context.Context, m api.Module, urlPtr, urlLen, headersPtr, headersLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return -1
	}

	urlBytes, err := readBufferFromMemory(m.Memory(), urlPtr, urlLen)
	if err != nil {
		return -1
	}
	wsURL := string(urlBytes)

	if err := h.Gate.CheckOutboundURLContext(ctx, wsURL); err != nil {
		h.Record("WARN", fmt.Sprintf("websocket blocked: %s (%v)", wsURL, err))
		return -1
	}

	var reqHeader http.Header
	if headersLen > 0 {
		hBytes, err := readBufferFromMemory(m.Memory(), headersPtr, headersLen)
		if err == nil && len(hBytes) > 0 {
			var hMap map[string]string
			if err := json.Unmarshal(hBytes, &hMap); err == nil {
				reqHeader = make(http.Header)
				for k, v := range hMap {
					reqHeader.Set(k, v)
				}
			}
		}
	}

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dialCancel()

	connCtx, connCancel := context.WithCancel(context.Background())
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: reqHeader,
	})
	if err != nil {
		connCancel()
		h.Record("WARN", fmt.Sprintf("websocket dial failed: %s (%v)", wsURL, err))
		return -2
	}

	h.mu.Lock()
	h.nextWSID++
	wsID := h.nextWSID
	if h.wsConns == nil {
		h.wsConns = make(map[int32]*HostWSConn)
	}
	wsConn := &HostWSConn{
		id:       wsID,
		url:      wsURL,
		conn:     conn,
		msgQueue: make(chan []byte, 200),
		ctx:      connCtx,
		cancel:   connCancel,
	}
	h.wsConns[wsID] = wsConn
	h.mu.Unlock()

	go func(c *HostWSConn) {
		defer c.close()
		for {
			if c.ctx.Err() != nil {
				return
			}
			_, data, err := c.conn.Read(c.ctx)
			if err != nil {
				c.lastErr = err
				return
			}
			select {
			case c.msgQueue <- data:
			default:
				select {
				case <-c.msgQueue:
				default:
				}
				c.msgQueue <- data
			}
		}
	}(wsConn)

	return wsID
}

func wsSend(ctx context.Context, m api.Module, handleID int32, msgType int32, dataPtr, dataLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return -1
	}

	h.mu.Lock()
	wsConn, exists := h.wsConns[handleID]
	h.mu.Unlock()

	if !exists || wsConn == nil || wsConn.closed.Load() {
		return -1
	}

	dataBytes, err := readBufferFromMemory(m.Memory(), dataPtr, dataLen)
	if err != nil {
		return -1
	}

	mt := websocket.MessageText
	if msgType == 2 {
		mt = websocket.MessageBinary
	}

	writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer writeCancel()

	if err := wsConn.conn.Write(writeCtx, mt, dataBytes); err != nil {
		return -2
	}

	return 0
}

func wsPoll(ctx context.Context, m api.Module, handleID int32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return -1
	}

	h.mu.Lock()
	wsConn, exists := h.wsConns[handleID]
	h.mu.Unlock()

	if !exists || wsConn == nil {
		return -1
	}

	select {
	case msg := <-wsConn.msgQueue:
		h.mu.Lock()
		h.LastResponse = msg
		h.mu.Unlock()
		return int32(len(msg))
	default:
		if wsConn.closed.Load() {
			return -1
		}
		return 0
	}
}

func wsClose(ctx context.Context, m api.Module, handleID int32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return -1
	}

	h.mu.Lock()
	wsConn, exists := h.wsConns[handleID]
	if exists {
		delete(h.wsConns, handleID)
	}
	h.mu.Unlock()

	if exists && wsConn != nil {
		wsConn.close()
	}

	return 0
}

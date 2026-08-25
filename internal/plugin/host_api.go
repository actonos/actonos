package plugin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/coder/websocket"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// KVStore defines the persistent key-value contract for plugins.
type KVStore interface {
	Get(pluginID, key string) (string, bool, error)
	Set(pluginID, key, value string) error
	Delete(pluginID, key string) error
}

// SecretProvider abstracts access to the encrypted vault or configured credentials.
type SecretProvider interface {
	GetSecret(ctx context.Context, secretName string) (string, error)
}

// HostWSConn represents an active, tracked WebSocket client connection.
type HostWSConn struct {
	id       int32
	url      string
	conn     *websocket.Conn
	msgQueue chan []byte
	ctx      context.Context
	cancel   context.CancelFunc
	closed   atomic.Bool
	lastErr  error
}

func (c *HostWSConn) close() {
	if c.closed.CompareAndSwap(false, true) {
		if c.cancel != nil {
			c.cancel()
		}
		if c.conn != nil {
			_ = c.conn.Close(websocket.StatusNormalClosure, "closed")
		}
	}
}

// HostContext bundles dependencies required by Host syscalls.
type HostContext struct {
	PluginID     string
	Manifest     PluginManifest
	Gate         *SecurityGate
	KV           KVStore
	Secrets      SecretProvider
	EventBus     *bus.EventBus
	HTTPClient   *http.Client
	AllocFn      api.Function
	FreeFn       api.Function
	Memory       api.Memory
	LastResponse []byte
	LogBuffer    []string
	secretValues []string
	wsConns      map[int32]*HostWSConn
	nextWSID     int32
	mu           sync.Mutex
}

func (h *HostContext) CloseAllWS() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.wsConns {
		c.close()
	}
	h.wsConns = make(map[int32]*HostWSConn)
}

const (
	maxPluginLogLines = 500
	maxPluginLogLine  = 4096
)

func (h *HostContext) AppendLog(line string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appendLogLocked(line)
}

func (h *HostContext) rememberSecret(value string) {
	if h == nil || value == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rememberSecretLocked(value)
}

func (h *HostContext) rememberSecretLocked(value string) {
	if value == "" {
		return
	}
	for _, existing := range h.secretValues {
		if existing == value {
			return
		}
	}
	h.secretValues = append(h.secretValues, value)
}

func (h *HostContext) redactSecretsLocked(line string) string {
	for _, secret := range h.secretValues {
		if secret != "" && strings.Contains(line, secret) {
			line = strings.ReplaceAll(line, secret, "••••")
		}
	}
	return line
}

func (h *HostContext) appendLogLocked(line string) {
	line = h.redactSecretsLocked(line)
	if len(line) > maxPluginLogLine {
		line = line[:maxPluginLogLine] + "…[truncated]"
	}
	h.LogBuffer = append(h.LogBuffer, line)
	if len(h.LogBuffer) > maxPluginLogLines {
		h.LogBuffer = h.LogBuffer[len(h.LogBuffer)-maxPluginLogLines:]
	}
}

func (h *HostContext) GetLogs() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	copied := make([]string, len(h.LogBuffer))
	copy(copied, h.LogBuffer)
	return copied
}

func (h *HostContext) SeedLogs(lines []string) {
	if h == nil || len(lines) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.LogBuffer = append([]string{}, lines...)
	if len(h.LogBuffer) > maxPluginLogLines {
		h.LogBuffer = h.LogBuffer[len(h.LogBuffer)-maxPluginLogLines:]
	}
}

// Record writes a host-side plugin event into the plugin-only log buffer.
// These lines never go to actond's slog output.
func (h *HostContext) Record(level, msg string) {
	if h == nil {
		return
	}
	if level == "" {
		level = "INFO"
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	h.AppendLog(fmt.Sprintf("[%s] [%s] %s", timestamp, strings.ToUpper(level), msg))
}

// pluginStdioWriter captures WASI stdout/stderr from a plugin instance so
// println/fmt output stays in the plugin log modal instead of actond's stdout.
type pluginStdioWriter struct {
	host  *HostContext
	level string
	mu    sync.Mutex
	buf   []byte
}

func (w *pluginStdioWriter) Write(p []byte) (int, error) {
	if w == nil || w.host == nil {
		return len(p), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			if len(w.buf) > maxPluginLogLine {
				w.host.Record(w.level, string(w.buf[:maxPluginLogLine]))
				w.buf = w.buf[:0]
			}
			break
		}
		line := strings.TrimRight(string(w.buf[:idx]), "\r")
		if strings.TrimSpace(line) != "" {
			w.host.Record(w.level, line)
		}
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

// HTTPRequestPayload is the JSON wire format for host_http_request.
type HTTPRequestPayload struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	BodyBase64 string            `json:"body_base64,omitempty"`
	Timeout    int               `json:"timeout_seconds,omitempty"`
}

// HTTPResponsePayload is the JSON wire format returned by host_http_request.
type HTTPResponsePayload struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Error   string            `json:"error,omitempty"`
}

func decodeHTTPRequestBody(p HTTPRequestPayload) ([]byte, error) {
	if strings.TrimSpace(p.BodyBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(p.BodyBase64)
		if err != nil {
			return nil, fmt.Errorf("decoding body_base64: %w", err)
		}
		return decoded, nil
	}
	if p.Body == "" {
		return nil, nil
	}
	return []byte(p.Body), nil
}

func httpRequestTimeout(p HTTPRequestPayload) time.Duration {
	if p.Timeout > 0 {
		if p.Timeout > 300 {
			return 300 * time.Second
		}
		return time.Duration(p.Timeout) * time.Second
	}
	if p.BodyBase64 != "" {
		return 120 * time.Second
	}
	return 60 * time.Second
}

// RegisterHostModule creates and instantiates host modules in Wazero runtime.
func RegisterHostModule(ctx context.Context, r wazero.Runtime) error {
	// 1. acton_sys (SDK core logging & response streaming)
	if _, err := r.NewHostModuleBuilder("acton_sys").
		NewFunctionBuilder().
		WithFunc(hostLog).
		Export("log").
		NewFunctionBuilder().
		WithFunc(sysReadResponse).
		Export("read_response").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_sys: %w", err)
	}

	// 2. acton_net (SDK network egress)
	if _, err := r.NewHostModuleBuilder("acton_net").
		NewFunctionBuilder().
		WithFunc(netHTTPRequest).
		Export("http_request").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_net: %w", err)
	}

	// 3. acton_vault (SDK vault secret retrieval)
	if _, err := r.NewHostModuleBuilder("acton_vault").
		NewFunctionBuilder().
		WithFunc(vaultGetSecret).
		Export("get_secret").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_vault: %w", err)
	}

	// 4. acton_storage (SDK persistent SQLite KV store)
	if _, err := r.NewHostModuleBuilder("acton_storage").
		NewFunctionBuilder().
		WithFunc(storageKVGet).
		Export("kv_get").
		NewFunctionBuilder().
		WithFunc(hostKVSet).
		Export("kv_set").
		NewFunctionBuilder().
		WithFunc(hostKVDelete).
		Export("kv_delete").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_storage: %w", err)
	}

	// 5. acton_bus (SDK event bus emitter)
	if _, err := r.NewHostModuleBuilder("acton_bus").
		NewFunctionBuilder().
		WithFunc(hostEmitEvent).
		Export("emit_event").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_bus: %w", err)
	}

	// 6. acton_ws (SDK WebSocket gateway)
	if _, err := r.NewHostModuleBuilder("acton_ws").
		NewFunctionBuilder().
		WithFunc(wsConnect).
		Export("ws_connect").
		NewFunctionBuilder().
		WithFunc(wsSend).
		Export("ws_send").
		NewFunctionBuilder().
		WithFunc(wsPoll).
		Export("ws_poll").
		NewFunctionBuilder().
		WithFunc(wsClose).
		Export("ws_close").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_ws: %w", err)
	}

	// 7. acton_host (legacy combined host module)
	_, err := r.NewHostModuleBuilder("acton_host").
		NewFunctionBuilder().
		WithFunc(hostHTTPRequest).
		Export("host_http_request").
		NewFunctionBuilder().
		WithFunc(hostGetSecret).
		Export("host_get_secret").
		NewFunctionBuilder().
		WithFunc(hostKVGet).
		Export("host_kv_get").
		NewFunctionBuilder().
		WithFunc(hostKVSet).
		Export("host_kv_set").
		NewFunctionBuilder().
		WithFunc(hostKVDelete).
		Export("host_kv_delete").
		NewFunctionBuilder().
		WithFunc(hostEmitEvent).
		Export("host_emit_event").
		NewFunctionBuilder().
		WithFunc(hostLog).
		Export("host_log").
		NewFunctionBuilder().
		WithFunc(hostTimeNowMS).
		Export("host_time_now_ms").
		Instantiate(ctx)

	return err
}

type hostContextKey struct{}

// WithHostContext attaches HostContext to the execution context.
func WithHostContext(ctx context.Context, h *HostContext) context.Context {
	return context.WithValue(ctx, hostContextKey{}, h)
}

// HostContextFrom retrieves HostContext from context.
func HostContextFrom(ctx context.Context) *HostContext {
	if ctx == nil {
		return nil
	}
	if val, ok := ctx.Value(hostContextKey{}).(*HostContext); ok {
		return val
	}
	return nil
}

const maxPluginHTTPBody = 1 << 20

func sandboxedHTTPClient(h *HostContext, timeout time.Duration) *http.Client {
	gate := (*SecurityGate)(nil)
	if h != nil {
		gate = h.Gate
		if h.HTTPClient != nil {
			cloned := *h.HTTPClient
			if cloned.Timeout == 0 {
				cloned.Timeout = timeout
			}
			if cloned.CheckRedirect == nil {
				cloned.CheckRedirect = pluginRedirectCheck(gate)
			}
			return &cloned
		}
	}
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: pluginRedirectCheck(gate),
	}
}

func pluginRedirectCheck(gate *SecurityGate) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if gate == nil {
			return fmt.Errorf("%w: missing security gate", ErrDomainNotWhitelisted)
		}
		return gate.CheckOutboundURLContext(req.Context(), req.URL.String())
	}
}

func readBufferFromMemory(mem api.Memory, ptr, length uint32) ([]byte, error) {
	if length == 0 {
		return []byte{}, nil
	}
	buf, ok := mem.Read(ptr, length)
	if !ok {
		return nil, fmt.Errorf("out of bounds memory read: ptr=%d, len=%d", ptr, length)
	}
	return buf, nil
}

func writeBufferToGuest(ctx context.Context, h *HostContext, data []byte) (uint64, error) {
	if h == nil || h.AllocFn == nil || h.Memory == nil {
		return 0, fmt.Errorf("host context or alloc function missing")
	}

	length := uint32(len(data))
	if length == 0 {
		return 0, nil
	}

	results, err := h.AllocFn.Call(ctx, uint64(length))
	if err != nil {
		return 0, fmt.Errorf("guest alloc failed: %w", err)
	}
	ptr := uint32(results[0])

	if !h.Memory.Write(ptr, data) {
		return 0, fmt.Errorf("failed to write %d bytes to guest memory at ptr=%d", length, ptr)
	}

	return (uint64(ptr) << 32) | uint64(length), nil
}

func sysReadResponse(ctx context.Context, m api.Module, destPtr, maxLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.LastResponse) == 0 {
		return 0
	}

	toWrite := uint32(len(h.LastResponse))
	if toWrite > maxLen {
		toWrite = maxLen
	}

	if !m.Memory().Write(destPtr, h.LastResponse[:toWrite]) {
		return -1
	}

	return int32(toWrite)
}

func netHTTPRequest(ctx context.Context, m api.Module, reqPtr, reqLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return -1
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		h.Record("ERROR", fmt.Sprintf("http_request: read memory failed: %v", err))
		return -1
	}

	var reqPayload HTTPRequestPayload
	if err := json.Unmarshal(reqBytes, &reqPayload); err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("invalid json payload: %v", err)})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: fmt.Sprintf("invalid json payload: %v", err), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}

	if err := h.Gate.CheckOutboundURLContext(ctx, reqPayload.URL); err != nil {
		h.Record("WARN", fmt.Sprintf("http_request blocked by gate: %s (%v)", reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 403, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}

	reqTimeout := httpRequestTimeout(reqPayload)
	client := sandboxedHTTPClient(h, reqTimeout)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), reqTimeout)
	defer reqCancel()

	method := reqPayload.Method
	if method == "" {
		method = http.MethodGet
	}

	bodyBytes, err := decodeHTTPRequestBody(reqPayload)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, method, reqPayload.URL, bodyReader)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 500, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}

	for k, v := range reqPayload.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		h.Record("WARN", fmt.Sprintf("HTTP %s %s failed: %v", method, reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 502, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxPluginHTTPBody))
	h.Record("DEBUG", fmt.Sprintf("HTTP %s %s -> %d (%d bytes)", method, reqPayload.URL, resp.StatusCode, len(respBody)))
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resPayload := HTTPResponsePayload{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(respBody),
	}
	resBytes, _ := json.Marshal(resPayload)

	h.mu.Lock()
	h.LastResponse = resBytes
	h.mu.Unlock()

	return int32(len(resBytes))
}

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

func vaultGetSecret(ctx context.Context, m api.Module, namePtr, nameLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.Secrets == nil {
		return 0
	}

	keyBytes, err := readBufferFromMemory(m.Memory(), namePtr, nameLen)
	if err != nil {
		return 0
	}
	secretKey := string(keyBytes)

	if err := h.Gate.CheckSecretAccess(secretKey); err != nil {
		h.Record("WARN", fmt.Sprintf("secret access denied: %s", secretKey))
		return 0
	}

	var val string
	if h.Secrets != nil {
		val, _ = h.Secrets.GetSecret(ctx, secretKey)
		if val == "" && strings.Contains(secretKey, ".") {
			parts := strings.Split(secretKey, ".")
			for _, part := range parts {
				if v, err := h.Secrets.GetSecret(ctx, part); err == nil && v != "" {
					val = v
					break
				}
			}
		}
	}

	// Fallback to manifest config if provided
	if val == "" && h.Manifest.Config != nil {
		if v, ok := h.Manifest.Config[secretKey].(string); ok && v != "" {
			val = v
		}
	}

	if val == "" {
		return 0
	}

	h.mu.Lock()
	h.rememberSecretLocked(val)
	h.LastResponse = []byte(val)
	h.mu.Unlock()

	return int32(len(h.LastResponse))
}

func storageKVGet(ctx context.Context, m api.Module, keyPtr, keyLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}

	if err := h.Gate.CheckStorageAccess(); err != nil {
		return 0
	}

	keyBytes, err := readBufferFromMemory(m.Memory(), keyPtr, keyLen)
	if err != nil {
		return 0
	}
	keyStr := string(keyBytes)

	var val string
	if h.KV != nil {
		v, found, err := h.KV.Get(h.PluginID, keyStr)
		if err == nil && found {
			val = v
		}
	}

	// Fallback to manifest config
	if val == "" && h.Manifest.Config != nil {
		if keyStr == "config" || keyStr == "__config__" || keyStr == "config.json" {
			b, _ := json.Marshal(h.Manifest.Config)
			val = string(b)
		} else if keyStr == "manifest" || keyStr == "manifest.json" {
			b, _ := json.Marshal(h.Manifest)
			val = string(b)
		} else if v, ok := h.Manifest.Config[keyStr]; ok {
			if s, ok := v.(string); ok {
				val = s
			} else {
				b, _ := json.Marshal(v)
				val = string(b)
			}
		}
	}

	if val == "" {
		return 0
	}

	h.mu.Lock()
	h.LastResponse = []byte(val)
	h.mu.Unlock()

	return int32(len(h.LastResponse))
}

func hostHTTPRequest(ctx context.Context, m api.Module, reqPtr, reqLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		h.Record("ERROR", fmt.Sprintf("host_http_request: read memory failed: %v", err))
		return 0
	}

	var reqPayload HTTPRequestPayload
	if err := json.Unmarshal(reqBytes, &reqPayload); err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("invalid json payload: %v", err)})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: fmt.Sprintf("invalid json payload: %v", err), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	if err := h.Gate.CheckOutboundURLContext(ctx, reqPayload.URL); err != nil {
		h.Record("WARN", fmt.Sprintf("host_http_request blocked by gate: %s (%v)", reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 403, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	reqTimeout := httpRequestTimeout(reqPayload)
	client := sandboxedHTTPClient(h, reqTimeout)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), reqTimeout)
	defer reqCancel()

	method := reqPayload.Method
	if method == "" {
		method = http.MethodGet
	}

	bodyBytes, err := decodeHTTPRequestBody(reqPayload)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, method, reqPayload.URL, bodyReader)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 500, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	for k, v := range reqPayload.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		h.Record("WARN", fmt.Sprintf("host_http_request %s %s failed: %v", method, reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 502, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxPluginHTTPBody))
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resPayload := HTTPResponsePayload{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(respBody),
	}
	resBytes, _ := json.Marshal(resPayload)
	packed, _ := writeBufferToGuest(ctx, h, resBytes)
	return packed
}

func hostGetSecret(ctx context.Context, m api.Module, keyPtr, keyLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil || h.Secrets == nil {
		return 0
	}

	keyBytes, err := readBufferFromMemory(m.Memory(), keyPtr, keyLen)
	if err != nil {
		return 0
	}
	secretKey := string(keyBytes)

	if err := h.Gate.CheckSecretAccess(secretKey); err != nil {
		h.Record("WARN", fmt.Sprintf("secret access denied: %s", secretKey))
		return 0
	}

	val, err := h.Secrets.GetSecret(ctx, secretKey)
	if err != nil {
		return 0
	}
	h.rememberSecret(val)

	packed, _ := writeBufferToGuest(ctx, h, []byte(val))
	return packed
}

func hostKVGet(ctx context.Context, m api.Module, keyPtr, keyLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil || h.KV == nil {
		return 0
	}

	if err := h.Gate.CheckStorageAccess(); err != nil {
		return 0
	}

	keyBytes, err := readBufferFromMemory(m.Memory(), keyPtr, keyLen)
	if err != nil {
		return 0
	}

	val, found, err := h.KV.Get(h.PluginID, string(keyBytes))
	if err != nil || !found {
		return 0
	}

	packed, _ := writeBufferToGuest(ctx, h, []byte(val))
	return packed
}

func hostKVSet(ctx context.Context, m api.Module, keyPtr, keyLen, valPtr, valLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.KV == nil {
		return -1
	}

	if err := h.Gate.CheckStorageAccess(); err != nil {
		return -1
	}

	keyBytes, err := readBufferFromMemory(m.Memory(), keyPtr, keyLen)
	if err != nil {
		return -1
	}

	valBytes, err := readBufferFromMemory(m.Memory(), valPtr, valLen)
	if err != nil {
		return -1
	}

	if err := h.KV.Set(h.PluginID, string(keyBytes), string(valBytes)); err != nil {
		return -1
	}
	return 0
}

func hostKVDelete(ctx context.Context, m api.Module, keyPtr, keyLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.KV == nil {
		return -1
	}

	if err := h.Gate.CheckStorageAccess(); err != nil {
		return -1
	}

	keyBytes, err := readBufferFromMemory(m.Memory(), keyPtr, keyLen)
	if err != nil {
		return -1
	}

	if err := h.KV.Delete(h.PluginID, string(keyBytes)); err != nil {
		return -1
	}
	return 0
}

func hostEmitEvent(ctx context.Context, m api.Module, topicPtr, topicLen, payloadPtr, payloadLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.EventBus == nil {
		return -1
	}

	topicBytes, err := readBufferFromMemory(m.Memory(), topicPtr, topicLen)
	if err != nil {
		return -1
	}
	topic := string(topicBytes)

	if err := h.Gate.CheckBusEvent(topic); err != nil {
		h.Record("WARN", fmt.Sprintf("bus emit denied: %s", topic))
		return -1
	}

	payloadBytes, err := readBufferFromMemory(m.Memory(), payloadPtr, payloadLen)
	if err != nil {
		return -1
	}

	var data any
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		data = string(payloadBytes)
	}

	h.EventBus.Publish(bus.NewEvent(topic, "", data))
	return 0
}

func hostLog(ctx context.Context, m api.Module, level int32, msgPtr, msgLen uint32) {
	h := HostContextFrom(ctx)
	if h == nil {
		return
	}

	msgBytes, err := readBufferFromMemory(m.Memory(), msgPtr, msgLen)
	if err != nil {
		return
	}

	levelStr := "INFO"
	switch level {
	case 0:
		levelStr = "DEBUG"
	case 1:
		levelStr = "INFO"
	case 2:
		levelStr = "WARN"
	case 3:
		levelStr = "ERROR"
	}
	h.Record(levelStr, string(msgBytes))
}

func hostTimeNowMS() int64 {
	return time.Now().UnixMilli()
}

// SQLiteKVStore implements persistent KV storage backed by ActonOS SQLite DB.
type SQLiteKVStore struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteKVStore creates the plugin_kv table if not exists and returns the store.
func NewSQLiteKVStore(db *sql.DB) (*SQLiteKVStore, error) {
	query := `
	CREATE TABLE IF NOT EXISTS plugin_kv (
		plugin_id TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(plugin_id, key)
	);`
	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("creating plugin_kv table: %w", err)
	}
	return &SQLiteKVStore{db: db}, nil
}

func (s *SQLiteKVStore) Get(pluginID, key string) (string, bool, error) {
	var val string
	err := s.db.QueryRow("SELECT value FROM plugin_kv WHERE plugin_id = ? AND key = ?", pluginID, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (s *SQLiteKVStore) Set(pluginID, key, value string) error {
	query := `
	INSERT INTO plugin_kv (plugin_id, key, value, updated_at)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(plugin_id, key) DO UPDATE SET
		value = excluded.value,
		updated_at = CURRENT_TIMESTAMP;`
	_, err := s.db.Exec(query, pluginID, key, value)
	return err
}

func (s *SQLiteKVStore) Delete(pluginID, key string) error {
	_, err := s.db.Exec("DELETE FROM plugin_kv WHERE plugin_id = ? AND key = ?", pluginID, key)
	return err
}

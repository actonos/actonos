package plugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/workspace"
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
	Tokens       TokenBroker
	EventBus     *bus.EventBus
	Workspace    *workspace.Store
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
		if p.Timeout > 600 {
			return 600 * time.Second
		}
		return time.Duration(p.Timeout) * time.Second
	}
	if p.BodyBase64 != "" {
		return 300 * time.Second
	}
	return 300 * time.Second
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

	// 7. acton_workspace (User Workspace file persistence)
	if _, err := r.NewHostModuleBuilder("acton_workspace").
		NewFunctionBuilder().
		WithFunc(workspaceSaveFile).
		Export("save_file").
		NewFunctionBuilder().
		WithFunc(workspaceReadFile).
		Export("read_file").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_workspace: %w", err)
	}

	// 8. acton_oauth (short-lived access tokens held by the core; refresh tokens never cross)
	if _, err := r.NewHostModuleBuilder("acton_oauth").
		NewFunctionBuilder().
		WithFunc(oauthGetToken).
		Export("get_token").
		Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiating acton_oauth: %w", err)
	}

	// 9. acton_host (legacy combined host module)
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
		WithFunc(hostWorkspaceSave).
		Export("host_workspace_save").
		NewFunctionBuilder().
		WithFunc(hostWorkspaceRead).
		Export("host_workspace_read").
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

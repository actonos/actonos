package plugin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// KVStore defines the persistent key-value contract for plugins.
type KVStore interface {
	Get(pluginID, key string) (string, bool, error)
	Set(pluginID, key, value string) error
	Delete(pluginID, key string) error
}

// SecretProvider abstracts access to the Hardware Vault or configured credentials.
type SecretProvider interface {
	GetSecret(ctx context.Context, secretName string) (string, error)
}

// HostContext bundles dependencies required by Host syscalls.
type HostContext struct {
	PluginID     string
	Gate         *SecurityGate
	KV           KVStore
	Secrets      SecretProvider
	EventBus     *bus.EventBus
	HTTPClient   *http.Client
	AllocFn      api.Function
	FreeFn       api.Function
	Memory       api.Memory
	mu           sync.Mutex
}

// HTTPRequestPayload is the JSON wire format for host_http_request.
type HTTPRequestPayload struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Timeout int               `json:"timeout_seconds,omitempty"`
}

// HTTPResponsePayload is the JSON wire format returned by host_http_request.
type HTTPResponsePayload struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// RegisterHostModule creates and instantiates the 'acton_host' host module in Wazero runtime.
func RegisterHostModule(ctx context.Context, r wazero.Runtime) error {
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

func hostHTTPRequest(ctx context.Context, m api.Module, reqPtr, reqLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		slog.Error("plugin host_http_request: read memory failed", "error", err)
		return 0
	}

	var reqPayload HTTPRequestPayload
	if err := json.Unmarshal(reqBytes, &reqPayload); err != nil {
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: fmt.Sprintf("invalid json payload: %v", err)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	if err := h.Gate.CheckOutboundURL(reqPayload.URL); err != nil {
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 403, Error: err.Error()})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	reqTimeout := 15 * time.Second
	if reqPayload.Timeout > 0 && reqPayload.Timeout <= 60 {
		reqTimeout = time.Duration(reqPayload.Timeout) * time.Second
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, reqTimeout)
	defer reqCancel()

	method := reqPayload.Method
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if reqPayload.Body != "" {
		bodyReader = bytes.NewBufferString(reqPayload.Body)
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, method, reqPayload.URL, bodyReader)
	if err != nil {
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 500, Error: err.Error()})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	for k, v := range reqPayload.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 502, Error: err.Error()})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
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
		slog.Warn("plugin unauthorized secret access attempt", "plugin_id", h.PluginID, "secret", secretKey, "error", err)
		return 0
	}

	val, err := h.Secrets.GetSecret(ctx, secretKey)
	if err != nil {
		return 0
	}

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
		slog.Warn("plugin unauthorized bus event emission", "plugin_id", h.PluginID, "topic", topic, "error", err)
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
	pluginID := "wasm_plugin"
	if h != nil && h.PluginID != "" {
		pluginID = h.PluginID
	}

	msgBytes, err := readBufferFromMemory(m.Memory(), msgPtr, msgLen)
	if err != nil {
		return
	}
	msg := string(msgBytes)

	switch level {
	case 0:
		slog.Debug(msg, "plugin", pluginID)
	case 1:
		slog.Info(msg, "plugin", pluginID)
	case 2:
		slog.Warn(msg, "plugin", pluginID)
	case 3:
		slog.Error(msg, "plugin", pluginID)
	default:
		slog.Info(msg, "plugin", pluginID)
	}
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

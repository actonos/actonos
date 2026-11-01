package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/actonos/actonos/internal/bus"
	"github.com/tetratelabs/wazero/api"
)

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

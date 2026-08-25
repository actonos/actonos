package channels

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/google/uuid"
)

// QueuedInbound is a persisted inbound wake with its durable id.
type QueuedInbound struct {
	ID      string
	Message InboundMessage
}

// InboundQueue persists channel wakes so a full in-memory bus cannot drop them.
type InboundQueue struct {
	db *sql.DB
}

func NewInboundQueue(db *sql.DB) (*InboundQueue, error) {
	q := &InboundQueue{db: db}
	if db == nil {
		return q, nil
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS inbound_channel_events (
			id TEXT PRIMARY KEY,
			payload_json TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			acked INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (q *InboundQueue) Enqueue(msg InboundMessage) (string, error) {
	if q == nil || q.db == nil {
		return "", nil
	}
	id := "inb_" + uuid.NewString()
	raw, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	_, err = q.db.Exec(`INSERT INTO inbound_channel_events (id, payload_json, created_at, acked) VALUES (?, ?, ?, 0)`,
		id, string(raw), time.Now().UTC())
	return id, err
}

// PersistEvent is an EventBus persist hook: durable-write inbound messages before fan-out.
func (q *InboundQueue) PersistEvent(ev *bus.Event) error {
	if q == nil || ev == nil || ev.Type != bus.EventChannelMessage {
		return nil
	}
	msg, ok := ev.Payload.(InboundMessage)
	if !ok {
		return nil
	}
	msg.Normalize()
	id, err := q.Enqueue(msg)
	if err != nil {
		return err
	}
	if ev.Metadata == nil {
		ev.Metadata = map[string]any{}
	}
	ev.Metadata["inbound_id"] = id
	return nil
}

func (q *InboundQueue) Pending(limit int) ([]QueuedInbound, error) {
	if q == nil || q.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 32
	}
	rows, err := q.db.Query(`SELECT id, payload_json FROM inbound_channel_events WHERE acked = 0 ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedInbound
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var msg InboundMessage
		if json.Unmarshal([]byte(raw), &msg) == nil {
			msg.Normalize()
			out = append(out, QueuedInbound{ID: id, Message: msg})
		}
	}
	return out, rows.Err()
}

// Claim atomically takes unacked rows so a replay cannot double-route.
func (q *InboundQueue) Claim(limit int) ([]QueuedInbound, error) {
	pending, err := q.Pending(limit)
	if err != nil || len(pending) == 0 {
		return pending, err
	}
	for _, item := range pending {
		if ackErr := q.Ack(item.ID); ackErr != nil {
			return pending, ackErr
		}
	}
	return pending, nil
}

func (q *InboundQueue) Ack(id string) error {
	if q == nil || q.db == nil || id == "" {
		return nil
	}
	_, err := q.db.Exec(`UPDATE inbound_channel_events SET acked = 1 WHERE id = ?`, id)
	return err
}

func (q *InboundQueue) CountPending() (int, error) {
	if q == nil || q.db == nil {
		return 0, nil
	}
	var n int
	err := q.db.QueryRow(`SELECT COUNT(*) FROM inbound_channel_events WHERE acked = 0`).Scan(&n)
	return n, err
}

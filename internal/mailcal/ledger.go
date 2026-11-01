package mailcal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Ledger states.
const (
	LedgerPending = "pending" // reserved; the provider may or may not have sent it
	LedgerSent    = "sent"
)

// LedgerEntry remembers one outgoing write so a retry can never send it twice.
type LedgerEntry struct {
	Key               string     `json:"key"`
	AccountID         string     `json:"account_id"`
	DraftID           string     `json:"draft_id,omitempty"`
	PayloadHash       string     `json:"payload_hash"`
	State             string     `json:"state"`
	ProviderMessageID string     `json:"provider_message_id,omitempty"`
	ThreadID          string     `json:"thread_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
}

// Ledger is the idempotency ledger. A draft can be sent at most once, so for mail the key is
// derived from the draft id alone; calendar creates derive it from the run and the payload.
type Ledger struct {
	db    *sql.DB
	locks sync.Map // key -> *sync.Mutex, serialises concurrent attempts inside this process
}

// NewLedger creates the table.
func NewLedger(db *sql.DB) (*Ledger, error) {
	if db == nil {
		return nil, errors.New("mailcal: database is required")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mailcal_sent_ledger (
		idempotency_key TEXT PRIMARY KEY,
		account_id TEXT NOT NULL,
		draft_id TEXT NOT NULL DEFAULT '',
		payload_hash TEXT NOT NULL,
		state TEXT NOT NULL,
		provider_message_id TEXT NOT NULL DEFAULT '',
		thread_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		sent_at TIMESTAMP
	)`); err != nil {
		return nil, fmt.Errorf("creating mailcal_sent_ledger: %w", err)
	}
	return &Ledger{db: db}, nil
}

// KeyFor builds a stable key from its parts. The parts are length-prefixed so no two
// different tuples can produce the same input to the hash.
func KeyFor(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	return "idem_" + hex.EncodeToString(h.Sum(nil))[:40]
}

// Begin reserves key. It returns the existing entry and true when the key was seen before.
// The returned unlock must be called when the caller has finished (sent or given up); until
// then no other goroutine can Begin the same key.
func (l *Ledger) Begin(ctx context.Context, e LedgerEntry) (entry LedgerEntry, existed bool, unlock func(), err error) {
	if strings.TrimSpace(e.Key) == "" {
		return LedgerEntry{}, false, nil, errors.New("ledger key is required")
	}
	mu, _ := l.locks.LoadOrStore(e.Key, &sync.Mutex{})
	lock := mu.(*sync.Mutex)
	lock.Lock()
	unlock = lock.Unlock

	got, err := l.get(ctx, e.Key)
	if err == nil {
		return got, true, unlock, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		unlock()
		return LedgerEntry{}, false, nil, err
	}
	e.State, e.CreatedAt = LedgerPending, time.Now().UTC()
	if _, err := l.db.ExecContext(ctx, `INSERT INTO mailcal_sent_ledger
		(idempotency_key, account_id, draft_id, payload_hash, state, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		e.Key, e.AccountID, e.DraftID, e.PayloadHash, e.State, e.CreatedAt); err != nil {
		unlock()
		return LedgerEntry{}, false, nil, fmt.Errorf("reserving ledger key: %w", err)
	}
	return e, false, unlock, nil
}

func (l *Ledger) get(ctx context.Context, key string) (LedgerEntry, error) {
	var e LedgerEntry
	var sent sql.NullTime
	err := l.db.QueryRowContext(ctx, `SELECT idempotency_key, account_id, draft_id, payload_hash, state,
		provider_message_id, thread_id, created_at, sent_at FROM mailcal_sent_ledger WHERE idempotency_key = ?`, key).
		Scan(&e.Key, &e.AccountID, &e.DraftID, &e.PayloadHash, &e.State, &e.ProviderMessageID, &e.ThreadID, &e.CreatedAt, &sent)
	if sent.Valid {
		t := sent.Time
		e.SentAt = &t
	}
	return e, err
}

// Get returns an entry by key (sql.ErrNoRows when absent).
func (l *Ledger) Get(ctx context.Context, key string) (LedgerEntry, error) { return l.get(ctx, key) }

// Complete marks key as sent. It uses a context that survives cancellation of the request,
// because losing this write is the one way a message could later be sent twice.
func (l *Ledger) Complete(ctx context.Context, key, providerMessageID, threadID string) error {
	now := time.Now().UTC()
	_, err := l.db.ExecContext(context.WithoutCancel(ctx), `UPDATE mailcal_sent_ledger
		SET state = ?, provider_message_id = ?, thread_id = ?, sent_at = ? WHERE idempotency_key = ?`,
		LedgerSent, providerMessageID, threadID, now, key)
	return err
}

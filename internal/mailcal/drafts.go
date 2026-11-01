package mailcal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/google/uuid"
)

const (
	maxSubjectRunes = 998
	maxBodyBytes    = 256 * 1024
	defaultMaxRcpt  = 50
)

var (
	// ErrDraftNotFound is returned for an unknown draft id.
	ErrDraftNotFound = errors.New("draft not found")
	// ErrInvalidDraft is returned when a draft fails validation.
	ErrInvalidDraft = errors.New("invalid draft")
)

// DraftPayload is exactly what would be sent. It never changes after creation.
type DraftPayload struct {
	To        []abi.Address `json:"to"`
	Cc        []abi.Address `json:"cc,omitempty"`
	Bcc       []abi.Address `json:"bcc,omitempty"`
	Subject   string        `json:"subject"`
	BodyText  string        `json:"body_text"`
	InReplyTo string        `json:"in_reply_to,omitempty"` // provider id of the message being answered
	ThreadID  string        `json:"thread_id,omitempty"`
}

// ReplyContext records, at draft time, what the provider said about the message being answered.
// The policy engine (Phase 2) trusts this instead of asking the provider again at send time.
type ReplyContext struct {
	MessageID    string   `json:"message_id"`
	ThreadID     string   `json:"thread_id"`
	Verified     bool     `json:"verified"`
	Participants []string `json:"participants"` // normalized emails seen in the thread
	SenderOfLast string   `json:"sender_of_last,omitempty"`
}

// Draft is an immutable outgoing message created by an agent. Approvals bind to its id,
// so changing the text means creating a new draft.
type Draft struct {
	ID          string        `json:"id"`
	AccountID   string        `json:"account_id"`
	CreatedBy   string        `json:"created_by"` // agent id
	TraceID     string        `json:"trace_id,omitempty"`
	Payload     DraftPayload  `json:"payload"`
	PayloadHash string        `json:"payload_hash"`
	Reply       *ReplyContext `json:"reply,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

// NewDraft is the input to DraftStore.Create. Recipients and text are validated and normalized there.
type NewDraft struct {
	AccountID string
	CreatedBy string
	TraceID   string
	Payload   DraftPayload
	Reply     *ReplyContext
	MaxRcpt   int // provider limit; 0 uses the default
}

// DraftStore persists drafts. There is deliberately no update method, and a trigger
// rejects UPDATE statements as a second line of defence.
type DraftStore struct{ db *sql.DB }

// NewDraftStore creates the table and the immutability trigger.
func NewDraftStore(db *sql.DB) (*DraftStore, error) {
	if db == nil {
		return nil, errors.New("mailcal: database is required")
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS mailcal_drafts (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			created_by TEXT NOT NULL,
			trace_id TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL,
			payload_hash TEXT NOT NULL,
			reply_json TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mailcal_drafts_account ON mailcal_drafts(account_id, created_at)`,
		`CREATE TRIGGER IF NOT EXISTS mailcal_drafts_immutable BEFORE UPDATE ON mailcal_drafts
			BEGIN SELECT RAISE(ABORT, 'mailcal drafts are immutable'); END`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("creating mailcal_drafts: %w", err)
		}
	}
	return &DraftStore{db: db}, nil
}

// hasControl reports control characters other than newline and tab.
func hasControl(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func normalizeRecipients(label string, in []abi.Address) ([]abi.Address, error) {
	out, err := abi.NormalizeAddresses(in)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidDraft, label, err)
	}
	return out, nil
}

// Validate normalizes and checks a payload, returning the canonical form that is stored.
func (p DraftPayload) Validate(maxRcpt int) (DraftPayload, error) {
	if maxRcpt <= 0 {
		maxRcpt = defaultMaxRcpt
	}
	var err error
	out := p
	if out.To, err = normalizeRecipients("to", p.To); err != nil {
		return DraftPayload{}, err
	}
	if out.Cc, err = normalizeRecipients("cc", p.Cc); err != nil {
		return DraftPayload{}, err
	}
	if out.Bcc, err = normalizeRecipients("bcc", p.Bcc); err != nil {
		return DraftPayload{}, err
	}
	if len(out.To) == 0 {
		return DraftPayload{}, fmt.Errorf("%w: at least one 'to' recipient is required", ErrInvalidDraft)
	}
	if n := len(out.To) + len(out.Cc) + len(out.Bcc); n > maxRcpt {
		return DraftPayload{}, fmt.Errorf("%w: %d recipients exceeds the limit of %d", ErrInvalidDraft, n, maxRcpt)
	}
	out.Subject = strings.TrimSpace(p.Subject)
	if out.Subject == "" {
		return DraftPayload{}, fmt.Errorf("%w: subject is required", ErrInvalidDraft)
	}
	if strings.ContainsAny(out.Subject, "\r\n") || hasControl(out.Subject) {
		return DraftPayload{}, fmt.Errorf("%w: subject must be a single line without control characters", ErrInvalidDraft)
	}
	if len([]rune(out.Subject)) > maxSubjectRunes {
		return DraftPayload{}, fmt.Errorf("%w: subject longer than %d characters", ErrInvalidDraft, maxSubjectRunes)
	}
	if strings.TrimSpace(p.BodyText) == "" {
		return DraftPayload{}, fmt.Errorf("%w: body is required", ErrInvalidDraft)
	}
	if len(p.BodyText) > maxBodyBytes {
		return DraftPayload{}, fmt.Errorf("%w: body larger than %d bytes", ErrInvalidDraft, maxBodyBytes)
	}
	if hasControl(p.BodyText) {
		return DraftPayload{}, fmt.Errorf("%w: body contains control characters", ErrInvalidDraft)
	}
	out.InReplyTo = strings.TrimSpace(p.InReplyTo)
	out.ThreadID = strings.TrimSpace(p.ThreadID)
	return out, nil
}

// Hash returns the SHA-256 of the canonical JSON of the payload.
func (p DraftPayload) Hash() string {
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Create validates and stores a draft.
func (s *DraftStore) Create(ctx context.Context, in NewDraft) (Draft, error) {
	if strings.TrimSpace(in.AccountID) == "" || strings.TrimSpace(in.CreatedBy) == "" {
		return Draft{}, fmt.Errorf("%w: account and creator are required", ErrInvalidDraft)
	}
	payload, err := in.Payload.Validate(in.MaxRcpt)
	if err != nil {
		return Draft{}, err
	}
	d := Draft{
		ID: "draft_" + uuid.NewString(), AccountID: in.AccountID, CreatedBy: in.CreatedBy, TraceID: in.TraceID,
		Payload: payload, PayloadHash: payload.Hash(), Reply: in.Reply, CreatedAt: time.Now().UTC(),
	}
	rawPayload, _ := json.Marshal(payload)
	rawReply := ""
	if in.Reply != nil {
		b, _ := json.Marshal(in.Reply)
		rawReply = string(b)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO mailcal_drafts (id, account_id, created_by, trace_id, payload_json, payload_hash, reply_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.AccountID, d.CreatedBy, d.TraceID, string(rawPayload), d.PayloadHash, rawReply, d.CreatedAt); err != nil {
		return Draft{}, fmt.Errorf("storing draft: %w", err)
	}
	return d, nil
}

// List returns an account's drafts, newest first. An empty accountID lists every account.
func (s *DraftStore) List(ctx context.Context, accountID string, limit int) ([]Draft, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id FROM mailcal_drafts`
	var args []any
	if accountID != "" {
		query += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]Draft, 0, len(ids))
	for _, id := range ids {
		d, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// Get returns a draft by id.
func (s *DraftStore) Get(ctx context.Context, id string) (Draft, error) {
	var d Draft
	var payload, reply string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, created_by, trace_id, payload_json, payload_hash, reply_json, created_at
		FROM mailcal_drafts WHERE id = ?`, id).
		Scan(&d.ID, &d.AccountID, &d.CreatedBy, &d.TraceID, &payload, &d.PayloadHash, &reply, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrDraftNotFound
	}
	if err != nil {
		return Draft{}, err
	}
	if err := json.Unmarshal([]byte(payload), &d.Payload); err != nil {
		return Draft{}, fmt.Errorf("decoding draft %s: %w", id, err)
	}
	if reply != "" {
		d.Reply = &ReplyContext{}
		if err := json.Unmarshal([]byte(reply), d.Reply); err != nil {
			return Draft{}, fmt.Errorf("decoding reply context of %s: %w", id, err)
		}
	}
	// Defence in depth: a draft whose stored text no longer matches its hash was tampered with.
	if d.Payload.Hash() != d.PayloadHash {
		return Draft{}, fmt.Errorf("draft %s failed its integrity check", id)
	}
	return d, nil
}

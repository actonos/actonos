package mailcal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/actonos/actonos/internal/mailcal/policy"
	"github.com/google/uuid"
)

// ErrRuleNotFound is returned for an unknown rule or proposal id.
var ErrRuleNotFound = errors.New("rule not found")

// StoredRule is a rule plus its bookkeeping.
type StoredRule struct {
	policy.Rule
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	PausedReason string     `json:"paused_reason,omitempty"`
	PausedAt     *time.Time `json:"paused_at,omitempty"`
}

// RuleStore persists the rule book. Only code reachable from an authenticated person
// (the HTTP API) may call Create; the agent tool surface never receives a RuleStore.
type RuleStore struct{ db *sql.DB }

// NewRuleStore creates the table.
func NewRuleStore(db *sql.DB) (*RuleStore, error) {
	if db == nil {
		return nil, errors.New("mailcal: database is required")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mailcal_rules (
		id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL,
		rule_json TEXT NOT NULL,
		enabled INTEGER NOT NULL,
		paused_reason TEXT NOT NULL DEFAULT '',
		paused_at TIMESTAMP,
		created_by TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("creating mailcal_rules: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_mailcal_rules_account ON mailcal_rules(account_id)`); err != nil {
		return nil, err
	}
	return &RuleStore{db: db}, nil
}

// Create validates, normalizes and stores a rule, assigning its id.
func (s *RuleStore) Create(ctx context.Context, r policy.Rule, actor string) (StoredRule, error) {
	if actor == "" {
		return StoredRule{}, errors.New("a rule needs an author")
	}
	if err := r.Validate(); err != nil {
		return StoredRule{}, err
	}
	r.ID = "rule_" + uuid.NewString()
	raw, err := json.Marshal(r)
	if err != nil {
		return StoredRule{}, err
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO mailcal_rules (id, account_id, rule_json, enabled, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, r.ID, r.AccountID, string(raw), boolInt(r.Enabled), actor, now); err != nil {
		return StoredRule{}, fmt.Errorf("storing rule: %w", err)
	}
	return StoredRule{Rule: r, CreatedBy: actor, CreatedAt: now}, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanRule(row interface{ Scan(...any) error }) (StoredRule, error) {
	var raw string
	var enabled int
	var out StoredRule
	var pausedAt sql.NullTime
	if err := row.Scan(&raw, &enabled, &out.PausedReason, &pausedAt, &out.CreatedBy, &out.CreatedAt); err != nil {
		return StoredRule{}, err
	}
	if err := json.Unmarshal([]byte(raw), &out.Rule); err != nil {
		return StoredRule{}, fmt.Errorf("decoding rule: %w", err)
	}
	out.Enabled = enabled == 1 // the column is authoritative; pausing never rewrites the JSON
	if pausedAt.Valid {
		t := pausedAt.Time
		out.PausedAt = &t
	}
	return out, nil
}

const ruleColumns = `rule_json, enabled, paused_reason, paused_at, created_by, created_at`

// Get returns one rule.
func (s *RuleStore) Get(ctx context.Context, id string) (StoredRule, error) {
	r, err := scanRule(s.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM mailcal_rules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return StoredRule{}, ErrRuleNotFound
	}
	return r, err
}

// List returns rules, newest first. An empty accountID lists every account.
func (s *RuleStore) List(ctx context.Context, accountID string) ([]StoredRule, error) {
	q, args := `SELECT `+ruleColumns+` FROM mailcal_rules`, []any{}
	if accountID != "" {
		q += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetEnabled turns a rule on or off. Turning it off records why (for example an automatic pause).
func (s *RuleStore) SetEnabled(ctx context.Context, id string, on bool, reason string) error {
	var pausedAt any
	if !on {
		pausedAt = time.Now().UTC()
	} else {
		reason = ""
	}
	res, err := s.db.ExecContext(ctx, `UPDATE mailcal_rules SET enabled = ?, paused_reason = ?, paused_at = ? WHERE id = ?`,
		boolInt(on), reason, pausedAt, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// Delete removes a rule.
func (s *RuleStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mailcal_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// PauseAccount disables every enabled rule of an account and returns how many changed.
func (s *RuleStore) PauseAccount(ctx context.Context, accountID, reason string) (int, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE mailcal_rules SET enabled = 0, paused_reason = ?, paused_at = ?
		WHERE account_id = ? AND enabled = 1`, reason, time.Now().UTC(), accountID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

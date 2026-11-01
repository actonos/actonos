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

// Decision phases.
const (
	PhaseGate     = "gate"     // the engine's verdict before the tool ran
	PhaseExecuted = "executed" // the write happened
)

// DecisionRecord is one audit line. It never holds message text, only hashes and ids.
type DecisionRecord struct {
	ID          string         `json:"id"`
	At          time.Time      `json:"at"`
	Phase       string         `json:"phase"`
	AccountID   string         `json:"account_id"`
	AgentID     string         `json:"agent_id"`
	Tool        string         `json:"tool"`
	DraftID     string         `json:"draft_id,omitempty"`
	PayloadHash string         `json:"payload_hash,omitempty"`
	Verdict     policy.Verdict `json:"verdict"`
	RuleID      string         `json:"rule_id,omitempty"`
	Reasons     []string       `json:"reasons,omitempty"`
	Approved    bool           `json:"approved,omitempty"` // a person approved this exact action
	Outcome     string         `json:"outcome,omitempty"`
}

// DecisionStore is the audit trail and the source of rule usage counts.
type DecisionStore struct{ db *sql.DB }

// NewDecisionStore creates the table.
func NewDecisionStore(db *sql.DB) (*DecisionStore, error) {
	if db == nil {
		return nil, errors.New("mailcal: database is required")
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS mailcal_decisions (
			id TEXT PRIMARY KEY,
			at TIMESTAMP NOT NULL,
			phase TEXT NOT NULL,
			account_id TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			tool TEXT NOT NULL,
			draft_id TEXT NOT NULL DEFAULT '',
			payload_hash TEXT NOT NULL DEFAULT '',
			verdict TEXT NOT NULL,
			rule_id TEXT NOT NULL DEFAULT '',
			reasons_json TEXT NOT NULL DEFAULT '[]',
			approved INTEGER NOT NULL DEFAULT 0,
			outcome TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mailcal_decisions_rule ON mailcal_decisions(rule_id, phase, at)`,
		`CREATE INDEX IF NOT EXISTS idx_mailcal_decisions_account ON mailcal_decisions(account_id, at)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("creating mailcal_decisions: %w", err)
		}
	}
	return &DecisionStore{db: db}, nil
}

// Record appends a line.
func (s *DecisionStore) Record(ctx context.Context, d DecisionRecord) error {
	if d.ID == "" {
		d.ID = "dec_" + uuid.NewString()
	}
	if d.At.IsZero() {
		d.At = time.Now().UTC()
	}
	reasons, _ := json.Marshal(d.Reasons)
	_, err := s.db.ExecContext(ctx, `INSERT INTO mailcal_decisions
		(id, at, phase, account_id, agent_id, tool, draft_id, payload_hash, verdict, rule_id, reasons_json, approved, outcome)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.At, d.Phase, d.AccountID, d.AgentID, d.Tool, d.DraftID, d.PayloadHash, string(d.Verdict), d.RuleID,
		string(reasons), boolInt(d.Approved), d.Outcome)
	return err
}

// CountAutoExecutions counts writes that a rule allowed on its own (no human approval)
// within the trailing window. These are the only writes charged against a rule's limits.
func (s *DecisionStore) CountAutoExecutions(ctx context.Context, ruleID string, window time.Duration) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mailcal_decisions
		WHERE rule_id = ? AND phase = ? AND approved = 0 AND verdict = ? AND at >= ?`,
		ruleID, PhaseExecuted, string(policy.VerdictAllow), time.Now().UTC().Add(-window)).Scan(&n)
	return n, err
}

// DecisionFilter narrows List.
type DecisionFilter struct {
	AccountID string
	Verdict   string
	Phase     string
	Limit     int
}

// List returns audit lines, newest first.
func (s *DecisionStore) List(ctx context.Context, f DecisionFilter) ([]DecisionRecord, error) {
	q := `SELECT id, at, phase, account_id, agent_id, tool, draft_id, payload_hash, verdict, rule_id, reasons_json, approved, outcome
		FROM mailcal_decisions WHERE 1=1`
	var args []any
	if f.AccountID != "" {
		q += ` AND account_id = ?`
		args = append(args, f.AccountID)
	}
	if f.Verdict != "" {
		q += ` AND verdict = ?`
		args = append(args, f.Verdict)
	}
	if f.Phase != "" {
		q += ` AND phase = ?`
		args = append(args, f.Phase)
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY at DESC, id DESC LIMIT ?`, append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DecisionRecord
	for rows.Next() {
		var d DecisionRecord
		var verdict, reasons string
		var approved int
		if err := rows.Scan(&d.ID, &d.At, &d.Phase, &d.AccountID, &d.AgentID, &d.Tool, &d.DraftID, &d.PayloadHash,
			&verdict, &d.RuleID, &reasons, &approved, &d.Outcome); err != nil {
			return nil, err
		}
		d.Verdict, d.Approved = policy.Verdict(verdict), approved == 1
		_ = json.Unmarshal([]byte(reasons), &d.Reasons)
		out = append(out, d)
	}
	return out, rows.Err()
}

// HasBounce reports whether a returned message was already counted.
func (s *DecisionStore) HasBounce(ctx context.Context, accountID, messageID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mailcal_decisions WHERE phase = ? AND account_id = ? AND draft_id = ? AND tool = 'inbox'`,
		PhaseBounce, accountID, messageID).Scan(&n)
	return n > 0, err
}

// CountSince counts audit lines of a phase for an account within the trailing window.
func (s *DecisionStore) CountSince(ctx context.Context, accountID, phase string, window time.Duration) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mailcal_decisions WHERE account_id = ? AND phase = ? AND tool = 'inbox' AND at >= ?`,
		accountID, phase, time.Now().UTC().Add(-window)).Scan(&n)
	return n, err
}

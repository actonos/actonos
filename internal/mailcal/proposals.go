package mailcal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/policy"
	"github.com/google/uuid"
)

// Proposal statuses.
const (
	ProposalPending  = "pending"
	ProposalApproved = "approved"
	ProposalRejected = "rejected"
)

// Proposal sources.
const (
	SourceAgent = "agent"
	SourceUser  = "user"
)

// Proposal is a rule that has no effect until a person approves it.
type Proposal struct {
	ID        string      `json:"id"`
	Source    string      `json:"source"`
	AgentID   string      `json:"agent_id,omitempty"`
	Rule      policy.Rule `json:"rule"`
	Rationale string      `json:"rationale,omitempty"`
	Status    string      `json:"status"`
	RuleID    string      `json:"rule_id,omitempty"` // set when approved
	CreatedAt time.Time   `json:"created_at"`
	DecidedAt *time.Time  `json:"decided_at,omitempty"`
}

// ProposalStore holds suggested rules. Agents may only Propose; Approve lives behind the
// authenticated HTTP API.
type ProposalStore struct {
	db    *sql.DB
	rules *RuleStore
}

// NewProposalStore creates the table.
func NewProposalStore(db *sql.DB, rules *RuleStore) (*ProposalStore, error) {
	if db == nil || rules == nil {
		return nil, errors.New("mailcal: database and rule store are required")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mailcal_proposals (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		agent_id TEXT NOT NULL DEFAULT '',
		rule_json TEXT NOT NULL,
		rationale TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		rule_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		decided_at TIMESTAMP
	)`); err != nil {
		return nil, fmt.Errorf("creating mailcal_proposals: %w", err)
	}
	return &ProposalStore{db: db, rules: rules}, nil
}

// Propose validates the rule shape and stores it as pending. The stored rule is forced to be
// disabled, so even a bug downstream could not turn a proposal into a working rule.
func (s *ProposalStore) Propose(ctx context.Context, source, agentID string, r policy.Rule, rationale string) (Proposal, error) {
	if source != SourceAgent && source != SourceUser {
		return Proposal{}, fmt.Errorf("unknown proposal source %q", source)
	}
	r.Enabled = false
	r.ID = ""
	if err := r.Validate(); err != nil {
		return Proposal{}, err
	}
	raw, _ := json.Marshal(r)
	p := Proposal{ID: "prop_" + uuid.NewString(), Source: source, AgentID: agentID, Rule: r,
		Rationale: truncateRunes(rationale, 500), Status: ProposalPending, CreatedAt: time.Now().UTC()}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO mailcal_proposals (id, source, agent_id, rule_json, rationale, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, p.ID, p.Source, p.AgentID, string(raw), p.Rationale, p.Status, p.CreatedAt); err != nil {
		return Proposal{}, fmt.Errorf("storing proposal: %w", err)
	}
	return p, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func scanProposal(row interface{ Scan(...any) error }) (Proposal, error) {
	var p Proposal
	var raw string
	var decided sql.NullTime
	if err := row.Scan(&p.ID, &p.Source, &p.AgentID, &raw, &p.Rationale, &p.Status, &p.RuleID, &p.CreatedAt, &decided); err != nil {
		return Proposal{}, err
	}
	if err := json.Unmarshal([]byte(raw), &p.Rule); err != nil {
		return Proposal{}, err
	}
	if decided.Valid {
		t := decided.Time
		p.DecidedAt = &t
	}
	return p, nil
}

const proposalColumns = `id, source, agent_id, rule_json, rationale, status, rule_id, created_at, decided_at`

// Get returns a proposal.
func (s *ProposalStore) Get(ctx context.Context, id string) (Proposal, error) {
	p, err := scanProposal(s.db.QueryRowContext(ctx, `SELECT `+proposalColumns+` FROM mailcal_proposals WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrRuleNotFound
	}
	return p, err
}

// List returns proposals with the given status ("" for all), newest first.
func (s *ProposalStore) List(ctx context.Context, status string) ([]Proposal, error) {
	q, args := `SELECT `+proposalColumns+` FROM mailcal_proposals`, []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at DESC, id DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ErrProposalDecided is returned when a proposal was already approved or rejected.
var ErrProposalDecided = errors.New("proposal was already decided")

// Approve turns a pending proposal into an enabled rule created by actor. The caller must
// have authenticated a person; this method does not (and cannot) check that.
func (s *ProposalStore) Approve(ctx context.Context, id, actor string) (StoredRule, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return StoredRule{}, err
	}
	if p.Status != ProposalPending {
		return StoredRule{}, ErrProposalDecided
	}
	rule := p.Rule
	rule.Enabled = true
	stored, err := s.rules.Create(ctx, rule, actor)
	if err != nil {
		return StoredRule{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE mailcal_proposals SET status = ?, rule_id = ?, decided_at = ? WHERE id = ? AND status = ?`,
		ProposalApproved, stored.ID, time.Now().UTC(), id, ProposalPending)
	if err != nil {
		return StoredRule{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 { // lost a race with another decision
		_ = s.rules.Delete(ctx, stored.ID)
		return StoredRule{}, ErrProposalDecided
	}
	return stored, nil
}

// Reject closes a pending proposal.
func (s *ProposalStore) Reject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE mailcal_proposals SET status = ?, decided_at = ? WHERE id = ? AND status = ?`,
		ProposalRejected, time.Now().UTC(), id, ProposalPending)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := s.Get(ctx, id); gerr != nil {
			return gerr
		}
		return ErrProposalDecided
	}
	return nil
}

// ProposalFromDraft builds the narrowest rule that would have allowed this draft: its exact
// recipients, reply-only when it is a verified reply, and its exact text as the template. It
// is stored as a pending proposal for a person to review; nothing becomes active here.
func (s *Service) ProposalFromDraft(ctx context.Context, draftID string) (Proposal, error) {
	d, err := s.drafts.Get(ctx, draftID)
	if err != nil {
		return Proposal{}, err
	}
	if len(d.Payload.Bcc) > 0 {
		return Proposal{}, errors.New("a draft with Bcc recipients can never be sent by a rule")
	}
	var addrs []string
	for _, a := range append(append([]abi.Address{}, d.Payload.To...), d.Payload.Cc...) {
		addrs = append(addrs, a.Email)
	}
	action := policy.ActionSendNew
	if d.Reply != nil {
		action = policy.ActionReply
	}
	rule := policy.Rule{
		AccountID: d.AccountID, Action: action, Effect: policy.EffectAllow,
		Match: policy.Match{
			Recipients: policy.Recipients{Addresses: addrs},
			ReplyOnly:  d.Reply != nil,
			Body:       policy.BodyMatch{Mode: policy.BodyTemplate, Template: d.Payload.BodyText},
		},
		Limits: policy.Limits{PerHour: 3, PerDay: 10},
	}
	return s.proposals.Propose(ctx, SourceUser, "", rule, "created from an approved draft")
}

// DryRun answers "would this rule handle this draft on its own, and why not?" without
// touching real rules or usage counters.
func (s *Service) DryRun(ctx context.Context, rule policy.Rule, draftID string) (policy.Decision, error) {
	d, err := s.drafts.Get(ctx, draftID)
	if err != nil {
		return policy.Decision{}, err
	}
	rule.AccountID, rule.Enabled, rule.ID = d.AccountID, true, "dry-run"
	if err := rule.Validate(); err != nil {
		return policy.Decision{}, err
	}
	zero := func(string, time.Duration) (int, error) { return 0, nil }
	return policy.Evaluate([]policy.Rule{rule}, requestForDraft(d), time.Now().UTC(), zero), nil
}

// EvaluateDraft returns the engine's current verdict for a draft, for the approval card.
func (s *Service) EvaluateDraft(ctx context.Context, draftID string) (policy.Decision, error) {
	d, err := s.drafts.Get(ctx, draftID)
	if err != nil {
		return policy.Decision{}, err
	}
	return s.Decide(ctx, requestForDraft(d)), nil
}

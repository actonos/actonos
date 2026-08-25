package tools

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

	"github.com/google/uuid"
)

var (
	// ErrApprovalRequired indicates that execution is paused pending a human decision.
	ErrApprovalRequired = errors.New("human approval required")
	// ErrApprovalInvalid indicates an expired, rejected, or mismatched approval.
	ErrApprovalInvalid = errors.New("approval is invalid for this action")
	// ErrApprovalNotPending indicates the record was already decided or has expired.
	ErrApprovalNotPending = errors.New("approval is no longer pending")
)

// ApprovalRequest is a durable human-in-the-loop decision record.
type ApprovalRequest struct {
	ID          string          `json:"id"`
	TraceID     string          `json:"trace_id"`
	AgentID     string          `json:"agent_id"`
	ToolName    string          `json:"tool_name"`
	RiskLevel   string          `json:"risk_level"`
	ActionHash  string          `json:"action_hash"`
	Input       json.RawMessage `json:"input"`
	Status      string          `json:"status"`
	Reason      string          `json:"reason,omitempty"`
	RequestedAt time.Time       `json:"requested_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	DecidedAt   *time.Time      `json:"decided_at,omitempty"`
	DecidedBy   string          `json:"decided_by,omitempty"`
	TaskID      string          `json:"task_id,omitempty"`

	// created is true only when Request() actually inserted a brand-new row,
	// as opposed to returning an already-pending approval for the same exact
	// action. Callers use IsNew() to avoid re-publishing "approval:required"
	// bus events (and therefore duplicate web notifications) for a request
	// the operator has already been asked about.
	created bool
}

// IsNew reports whether this ApprovalRequest was just created by Request(),
// as opposed to being an existing pending approval that was reused.
func (a ApprovalRequest) IsNew() bool {
	return a.created
}

// ApprovalRequiredError carries the durable approval identifier to callers.
type ApprovalRequiredError struct {
	Approval ApprovalRequest
}

func (e *ApprovalRequiredError) Error() string {
	return fmt.Sprintf("%s: approval_id=%s tool=%s risk=%s", ErrApprovalRequired, e.Approval.ID, e.Approval.ToolName, e.Approval.RiskLevel)
}

func (e *ApprovalRequiredError) Unwrap() error { return ErrApprovalRequired }

const (
	// DontAskTask skips later approvals for the same agent+tool while the
	// originating mission (task_id) is still the active scope.
	DontAskTask = "task"
	// DontAskToday skips later approvals for the same agent+tool until the
	// end of the current UTC day (at least one hour).
	DontAskToday = "today"
)

// ApprovalGrant is a temporary operator waiver for a single agent tool.
type ApprovalGrant struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	ToolName         string    `json:"tool_name"`
	Scope            string    `json:"scope"`
	TaskID           string    `json:"task_id,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedBy        string    `json:"created_by,omitempty"`
	SourceApprovalID string    `json:"source_approval_id,omitempty"`
}

// GrantEligibleTool reports whether a tool may receive a don't-ask-again waiver.
// Administrative mutations always require an exact-action decision.
func GrantEligibleTool(name string) bool {
	if name == "" || strings.HasPrefix(name, "admin_") || name == "system_mcp_connect" {
		return false
	}
	return true
}

func grantExpiry(scope string, now time.Time) (time.Time, error) {
	now = now.UTC()
	switch scope {
	case DontAskToday:
		end := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		if end.Sub(now) < time.Hour {
			end = end.Add(24 * time.Hour)
		}
		return end, nil
	case DontAskTask:
		return now.Add(7 * 24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("dont_ask_again must be %q or %q", DontAskTask, DontAskToday)
	}
}

// ApprovalManager persists and validates exact-action approvals.
type ApprovalManager struct {
	db  *sql.DB
	ttl time.Duration
}

// NewApprovalManager creates an approval store backed by SQLite.
func NewApprovalManager(db *sql.DB) *ApprovalManager {
	m := &ApprovalManager{db: db, ttl: 30 * time.Minute}
	m.ensureSchema()
	return m
}

func (m *ApprovalManager) ensureSchema() {
	if m == nil || m.db == nil {
		return
	}
	_, _ = m.db.Exec(`ALTER TABLE approvals ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`)
	_, _ = m.db.Exec(`
		CREATE TABLE IF NOT EXISTS approval_grants (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			scope TEXT NOT NULL,
			task_id TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			source_approval_id TEXT NOT NULL DEFAULT ''
		)
	`)
	_, _ = m.db.Exec(`CREATE INDEX IF NOT EXISTS idx_approval_grants_lookup ON approval_grants(agent_id, tool_name, scope, expires_at)`)
}

// canonicalizeInput produces a byte-stable representation of a JSON payload.
//
// Approval records survive a json.Marshal/Unmarshal round trip through the run
// checkpoint, which compacts whitespace, applies HTML escaping, and reorders
// object keys. Hashing the raw bytes therefore produces a different digest for
// semantically identical input, which would reject a legitimate approval. This
// decodes and re-encodes so only the JSON *value* contributes to the hash.
func canonicalizeInput(input json.RawMessage) []byte {
	var decoded any
	if err := json.Unmarshal(input, &decoded); err != nil {
		// Not valid JSON: fall back to the raw bytes so hashing stays total.
		return input
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return input
	}
	return canonical
}

// ActionHash binds an approval to the exact agent, tool, and normalized input.
func ActionHash(agentID, toolName string, input json.RawMessage) string {
	sum := sha256.Sum256(append([]byte(agentID+"\x00"+toolName+"\x00"), canonicalizeInput(input)...))
	return hex.EncodeToString(sum[:])
}

// Request creates or returns a pending approval for an exact action.
func (m *ApprovalManager) Request(ctx context.Context, traceID, agentID, toolName, riskLevel string, input json.RawMessage) (*ApprovalRequest, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("approval store is unavailable")
	}
	hash := ActionHash(agentID, toolName, input)
	if traceID == "" {
		traceID = newTraceID()
	}
	taskID := TaskIDFromContext(ctx)
	var existing ApprovalRequest
	var inputText string
	err := m.db.QueryRowContext(ctx, `
		SELECT id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
		       status, COALESCE(reason, ''), requested_at, expires_at, COALESCE(task_id, '')
		FROM approvals
		WHERE action_hash = ? AND status = 'pending' AND expires_at > ?
		ORDER BY requested_at DESC LIMIT 1
	`, hash, time.Now().UTC()).Scan(
		&existing.ID, &existing.TraceID, &existing.AgentID, &existing.ToolName,
		&existing.RiskLevel, &existing.ActionHash, &inputText, &existing.Status,
		&existing.Reason, &existing.RequestedAt, &existing.ExpiresAt, &existing.TaskID,
	)
	if err == nil {
		existing.Input = json.RawMessage(inputText)
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("querying pending approval: %w", err)
	}

	now := time.Now().UTC()
	request := &ApprovalRequest{
		ID:          "apr_" + uuid.NewString(),
		TraceID:     traceID,
		AgentID:     agentID,
		ToolName:    toolName,
		RiskLevel:   riskLevel,
		ActionHash:  hash,
		Input:       append(json.RawMessage(nil), input...),
		Status:      "pending",
		RequestedAt: now,
		ExpiresAt:   now.Add(m.ttl),
		TaskID:      taskID,
		created:     true,
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO approvals (
			id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
			status, requested_at, expires_at, task_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, request.ID, request.TraceID, request.AgentID, request.ToolName, request.RiskLevel,
		request.ActionHash, string(request.Input), request.Status, request.RequestedAt, request.ExpiresAt, request.TaskID)
	if err != nil {
		return nil, fmt.Errorf("creating approval request: %w", err)
	}
	return request, nil
}

// List returns approval requests, optionally filtered by status.
func (m *ApprovalManager) List(ctx context.Context, status string, limit int) ([]ApprovalRequest, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if _, err := m.db.ExecContext(ctx, `
		UPDATE approvals SET status = 'expired'
		WHERE status = 'pending' AND expires_at <= ?
	`, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("expiring stale approvals: %w", err)
	}
	query := `
		SELECT id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
		       status, COALESCE(reason, ''), requested_at, expires_at, decided_at, COALESCE(decided_by, ''),
		       COALESCE(task_id, '')
		FROM approvals`
	args := []any{}
	if status != "" && status != "all" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY requested_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing approvals: %w", err)
	}
	defer rows.Close()

	var approvals []ApprovalRequest
	for rows.Next() {
		var item ApprovalRequest
		var inputText string
		var decidedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.TraceID, &item.AgentID, &item.ToolName, &item.RiskLevel,
			&item.ActionHash, &inputText, &item.Status, &item.Reason,
			&item.RequestedAt, &item.ExpiresAt, &decidedAt, &item.DecidedBy, &item.TaskID,
		); err != nil {
			return nil, fmt.Errorf("scanning approval: %w", err)
		}
		item.Input = json.RawMessage(inputText)
		if decidedAt.Valid {
			item.DecidedAt = &decidedAt.Time
		}
		approvals = append(approvals, item)
	}
	if approvals == nil {
		approvals = []ApprovalRequest{}
	}
	return approvals, rows.Err()
}

// Decide approves or rejects a pending request.
func (m *ApprovalManager) Decide(ctx context.Context, id, decision, actor, reason string) (*ApprovalRequest, error) {
	if decision != "approved" && decision != "rejected" {
		return nil, errors.New("decision must be approved or rejected")
	}
	now := time.Now().UTC()
	result, err := m.db.ExecContext(ctx, `
		UPDATE approvals SET status = ?, reason = ?, decided_at = ?, decided_by = ?
		WHERE id = ? AND status = 'pending' AND expires_at > ?
	`, decision, reason, now, actor, id, now)
	if err != nil {
		return nil, fmt.Errorf("updating approval: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking approval update: %w", err)
	}
	if affected != 1 {
		return nil, m.explainNotPending(ctx, id)
	}
	return m.Get(ctx, id)
}

// explainNotPending turns a failed decision into an actionable reason.
func (m *ApprovalManager) explainNotPending(ctx context.Context, id string) error {
	current, err := m.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: no approval found with id %s", ErrApprovalNotPending, id)
	}
	if current.Status != "pending" {
		return fmt.Errorf("%w: id=%s was already %s", ErrApprovalNotPending, id, current.Status)
	}
	return fmt.Errorf("%w: id=%s expired at %s", ErrApprovalNotPending, id, current.ExpiresAt.Format(time.RFC3339))
}

// Reopen returns a decided approval to the pending state.
//
// An approved action whose execution then fails would otherwise be stranded:
// the record is consumed, so the operator can neither retry nor reject it. This
// restores the pending state (extending the TTL) so the decision stays live.
func (m *ApprovalManager) Reopen(ctx context.Context, id string) error {
	if m == nil || m.db == nil {
		return errors.New("approval store is unavailable")
	}
	_, err := m.db.ExecContext(ctx, `
		UPDATE approvals SET status = 'pending', reason = NULL, decided_at = NULL,
		       decided_by = NULL, expires_at = ?
		WHERE id = ? AND status IN ('approved', 'rejected')
	`, time.Now().UTC().Add(m.ttl), id)
	if err != nil {
		return fmt.Errorf("reopening approval: %w", err)
	}
	return nil
}

// Get loads a single approval.
func (m *ApprovalManager) Get(ctx context.Context, id string) (*ApprovalRequest, error) {
	var item ApprovalRequest
	var inputText string
	var decidedAt sql.NullTime
	err := m.db.QueryRowContext(ctx, `
		SELECT id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
		       status, COALESCE(reason, ''), requested_at, expires_at, decided_at, COALESCE(decided_by, ''),
		       COALESCE(task_id, '')
		FROM approvals WHERE id = ?
	`, id).Scan(
		&item.ID, &item.TraceID, &item.AgentID, &item.ToolName, &item.RiskLevel,
		&item.ActionHash, &inputText, &item.Status, &item.Reason,
		&item.RequestedAt, &item.ExpiresAt, &decidedAt, &item.DecidedBy, &item.TaskID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading approval: %w", err)
	}
	item.Input = json.RawMessage(inputText)
	if decidedAt.Valid {
		item.DecidedAt = &decidedAt.Time
	}
	return &item, nil
}

// ValidateApproved verifies that an approved record matches the exact action.
func (m *ApprovalManager) ValidateApproved(ctx context.Context, id, agentID, toolName string, input json.RawMessage) error {
	item, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != "approved" {
		return fmt.Errorf("%w: id=%s has status %q, expected approved", ErrApprovalInvalid, id, item.Status)
	}
	if time.Now().UTC().After(item.ExpiresAt) {
		return fmt.Errorf("%w: id=%s expired at %s", ErrApprovalInvalid, id, item.ExpiresAt.Format(time.RFC3339))
	}
	if item.ActionHash != ActionHash(agentID, toolName, input) {
		return fmt.Errorf(
			"%w: id=%s was approved for a different action (approved tool=%s agent=%s, attempted tool=%s agent=%s)",
			ErrApprovalInvalid, id, item.ToolName, item.AgentID, toolName, agentID,
		)
	}
	return nil
}

// CreateGrant records a temporary don't-ask-again waiver from an approved decision.
func (m *ApprovalManager) CreateGrant(ctx context.Context, approval *ApprovalRequest, scope, actor string) (*ApprovalGrant, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("approval store is unavailable")
	}
	if approval == nil {
		return nil, errors.New("approval is required")
	}
	if !GrantEligibleTool(approval.ToolName) {
		return nil, fmt.Errorf("dont-ask-again is not available for %s", approval.ToolName)
	}
	expiresAt, err := grantExpiry(scope, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	taskID := ""
	if scope == DontAskTask {
		taskID = strings.TrimSpace(approval.TaskID)
		if taskID == "" {
			return nil, errors.New("dont-ask-again for this task requires a mission task id")
		}
	}
	now := time.Now().UTC()
	grant := &ApprovalGrant{
		ID:               "grn_" + uuid.NewString(),
		AgentID:          approval.AgentID,
		ToolName:         approval.ToolName,
		Scope:            scope,
		TaskID:           taskID,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
		CreatedBy:        actor,
		SourceApprovalID: approval.ID,
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO approval_grants (
			id, agent_id, tool_name, scope, task_id, expires_at, created_at, created_by, source_approval_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, grant.ID, grant.AgentID, grant.ToolName, grant.Scope, grant.TaskID, grant.ExpiresAt, grant.CreatedAt, grant.CreatedBy, grant.SourceApprovalID)
	if err != nil {
		return nil, fmt.Errorf("creating approval grant: %w", err)
	}
	return grant, nil
}

// ActiveGrant returns a still-valid waiver for this agent, tool, and optional task.
func (m *ApprovalManager) ActiveGrant(ctx context.Context, agentID, toolName, taskID string) (*ApprovalGrant, error) {
	if m == nil || m.db == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	_, _ = m.db.ExecContext(ctx, `DELETE FROM approval_grants WHERE expires_at <= ?`, now)

	if taskID != "" {
		if grant, err := m.lookupGrant(ctx, `
			SELECT id, agent_id, tool_name, scope, task_id, expires_at, created_at, created_by, source_approval_id
			FROM approval_grants
			WHERE agent_id = ? AND tool_name = ? AND scope = ? AND task_id = ? AND expires_at > ?
			ORDER BY expires_at DESC LIMIT 1
		`, agentID, toolName, DontAskTask, taskID, now); err != nil {
			return nil, err
		} else if grant != nil {
			return grant, nil
		}
	}
	return m.lookupGrant(ctx, `
		SELECT id, agent_id, tool_name, scope, task_id, expires_at, created_at, created_by, source_approval_id
		FROM approval_grants
		WHERE agent_id = ? AND tool_name = ? AND scope = ? AND expires_at > ?
		ORDER BY expires_at DESC LIMIT 1
	`, agentID, toolName, DontAskToday, now)
}

func (m *ApprovalManager) lookupGrant(ctx context.Context, query string, args ...any) (*ApprovalGrant, error) {
	var grant ApprovalGrant
	err := m.db.QueryRowContext(ctx, query, args...).Scan(
		&grant.ID, &grant.AgentID, &grant.ToolName, &grant.Scope, &grant.TaskID,
		&grant.ExpiresAt, &grant.CreatedAt, &grant.CreatedBy, &grant.SourceApprovalID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up approval grant: %w", err)
	}
	return &grant, nil
}

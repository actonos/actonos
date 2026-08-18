package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrApprovalRequired indicates that execution is paused pending a human decision.
	ErrApprovalRequired = errors.New("human approval required")
	// ErrApprovalInvalid indicates an expired, rejected, or mismatched approval.
	ErrApprovalInvalid = errors.New("approval is invalid for this action")
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
}

// ApprovalRequiredError carries the durable approval identifier to callers.
type ApprovalRequiredError struct {
	Approval ApprovalRequest
}

func (e *ApprovalRequiredError) Error() string {
	return fmt.Sprintf("%s: approval_id=%s tool=%s risk=%s", ErrApprovalRequired, e.Approval.ID, e.Approval.ToolName, e.Approval.RiskLevel)
}

func (e *ApprovalRequiredError) Unwrap() error { return ErrApprovalRequired }

// ApprovalManager persists and validates exact-action approvals.
type ApprovalManager struct {
	db  *sql.DB
	ttl time.Duration
}

// NewApprovalManager creates an approval store backed by SQLite.
func NewApprovalManager(db *sql.DB) *ApprovalManager {
	return &ApprovalManager{db: db, ttl: 30 * time.Minute}
}

// ActionHash binds an approval to the exact agent, tool, and normalized input.
func ActionHash(agentID, toolName string, input json.RawMessage) string {
	sum := sha256.Sum256(append([]byte(agentID+"\x00"+toolName+"\x00"), input...))
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
	var existing ApprovalRequest
	var inputText string
	err := m.db.QueryRowContext(ctx, `
		SELECT id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
		       status, COALESCE(reason, ''), requested_at, expires_at
		FROM approvals
		WHERE action_hash = ? AND status = 'pending' AND expires_at > ?
		ORDER BY requested_at DESC LIMIT 1
	`, hash, time.Now().UTC()).Scan(
		&existing.ID, &existing.TraceID, &existing.AgentID, &existing.ToolName,
		&existing.RiskLevel, &existing.ActionHash, &inputText, &existing.Status,
		&existing.Reason, &existing.RequestedAt, &existing.ExpiresAt,
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
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO approvals (
			id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
			status, requested_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, request.ID, request.TraceID, request.AgentID, request.ToolName, request.RiskLevel,
		request.ActionHash, string(request.Input), request.Status, request.RequestedAt, request.ExpiresAt)
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
		       status, COALESCE(reason, ''), requested_at, expires_at, decided_at, COALESCE(decided_by, '')
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
			&item.RequestedAt, &item.ExpiresAt, &decidedAt, &item.DecidedBy,
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
		return nil, ErrApprovalInvalid
	}
	return m.Get(ctx, id)
}

// Get loads a single approval.
func (m *ApprovalManager) Get(ctx context.Context, id string) (*ApprovalRequest, error) {
	var item ApprovalRequest
	var inputText string
	var decidedAt sql.NullTime
	err := m.db.QueryRowContext(ctx, `
		SELECT id, trace_id, agent_id, tool_name, risk_level, action_hash, input_json,
		       status, COALESCE(reason, ''), requested_at, expires_at, decided_at, COALESCE(decided_by, '')
		FROM approvals WHERE id = ?
	`, id).Scan(
		&item.ID, &item.TraceID, &item.AgentID, &item.ToolName, &item.RiskLevel,
		&item.ActionHash, &inputText, &item.Status, &item.Reason,
		&item.RequestedAt, &item.ExpiresAt, &decidedAt, &item.DecidedBy,
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
	if item.Status != "approved" || time.Now().UTC().After(item.ExpiresAt) ||
		item.ActionHash != ActionHash(agentID, toolName, input) {
		return ErrApprovalInvalid
	}
	return nil
}

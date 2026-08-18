package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/google/uuid"
)

// RunStatus represents durable autonomous execution state.
type RunStatus string

const (
	RunRunning         RunStatus = "running"
	RunCompleted       RunStatus = "completed"
	RunFailed          RunStatus = "failed"
	RunApprovalPending RunStatus = "approval_pending"
	RunBlocked         RunStatus = "blocked"
	RunCancelled       RunStatus = "cancelled"
)

// AgentRun is the durable parent record for an end-to-end agent execution.
type AgentRun struct {
	ID                string     `json:"id"`
	TraceID           string     `json:"trace_id"`
	AgentID           string     `json:"agent_id"`
	Goal              string     `json:"goal"`
	Source            string     `json:"source"`
	Status            RunStatus  `json:"status"`
	TerminationReason string     `json:"termination_reason,omitempty"`
	Iterations        int        `json:"iterations"`
	PromptTokens      int        `json:"prompt_tokens"`
	CompletionTokens  int        `json:"completion_tokens"`
	TotalTokens       int        `json:"total_tokens"`
	StartedAt         time.Time  `json:"started_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

// RunEvent is an append-only execution observation.
type RunEvent struct {
	ID         string         `json:"id"`
	RunID      string         `json:"run_id"`
	TraceID    string         `json:"trace_id"`
	Step       int            `json:"step"`
	Type       string         `json:"type"`
	Status     string         `json:"status"`
	ToolName   string         `json:"tool_name,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// RunCheckpoint contains sufficient state to resume an approval-paused ReAct loop.
type RunCheckpoint struct {
	RunID       string        `json:"run_id"`
	TraceID     string        `json:"trace_id"`
	AgentID     string        `json:"agent_id"`
	Goal        string        `json:"goal"`
	Source      string        `json:"source"`
	Messages    []llm.Message `json:"messages"`
	Iteration   int           `json:"iteration"`
	Usage       llm.Usage     `json:"usage"`
	PendingTool llm.ToolCall  `json:"pending_tool"`
}

// RunStore persists checkpoints and append-only run events.
type RunStore struct {
	db *sql.DB
}

// NewRunStore creates a durable run store.
func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

// Start creates a running execution.
func (s *RunStore) Start(ctx context.Context, traceID, agentID, goal, source string) (*AgentRun, error) {
	now := time.Now().UTC()
	run := &AgentRun{
		ID:        "run_" + uuid.NewString(),
		TraceID:   traceID,
		AgentID:   agentID,
		Goal:      goal,
		Source:    source,
		Status:    RunRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_runs (
			id, trace_id, agent_id, goal, source, status, termination_reason,
			iterations, prompt_tokens, completion_tokens, total_tokens, started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, '', 0, 0, 0, 0, ?, ?)
	`, run.ID, run.TraceID, run.AgentID, run.Goal, run.Source, run.Status, now, now)
	if err != nil {
		return nil, fmt.Errorf("starting agent run: %w", err)
	}
	return run, nil
}

// AppendEvent appends a structured observation to a run.
func (s *RunStore) AppendEvent(ctx context.Context, event RunEvent) error {
	if event.ID == "" {
		event.ID = "evt_" + uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("marshalling run event data: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO run_events (
			id, run_id, trace_id, step, type, status, tool_name, data_json, duration_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.RunID, event.TraceID, event.Step, event.Type, event.Status,
		event.ToolName, string(data), event.DurationMS, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("appending run event: %w", err)
	}
	return nil
}

// Finish checkpoints aggregate metrics and a terminal state.
func (s *RunStore) Finish(ctx context.Context, run *AgentRun) error {
	now := time.Now().UTC()
	run.UpdatedAt = now
	if run.Status != RunRunning && run.Status != RunApprovalPending {
		run.CompletedAt = &now
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_runs SET status = ?, termination_reason = ?, iterations = ?,
			prompt_tokens = ?, completion_tokens = ?, total_tokens = ?,
			updated_at = ?, completed_at = ?
		WHERE id = ?
	`, run.Status, run.TerminationReason, run.Iterations, run.PromptTokens,
		run.CompletionTokens, run.TotalTokens, run.UpdatedAt, run.CompletedAt, run.ID)
	if err != nil {
		return fmt.Errorf("finishing agent run: %w", err)
	}
	return nil
}

// SaveCheckpoint persists resumable execution state before an approval pause.
func (s *RunStore) SaveCheckpoint(ctx context.Context, checkpoint RunCheckpoint) error {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshalling run checkpoint: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE agent_runs SET checkpoint_json = ?, updated_at = ?
		WHERE id = ?
	`, string(data), time.Now().UTC(), checkpoint.RunID)
	if err != nil {
		return fmt.Errorf("saving run checkpoint: %w", err)
	}
	return nil
}

// LoadCheckpointByTrace loads the latest approval-pending checkpoint for a trace.
func (s *RunStore) LoadCheckpointByTrace(ctx context.Context, traceID string) (*RunCheckpoint, *AgentRun, error) {
	var checkpointJSON string
	var run AgentRun
	var completed sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, trace_id, agent_id, goal, source, status, termination_reason,
		       iterations, prompt_tokens, completion_tokens, total_tokens,
		       started_at, updated_at, completed_at, COALESCE(checkpoint_json, '')
		FROM agent_runs
		WHERE trace_id = ? AND status = ?
		ORDER BY started_at DESC LIMIT 1
	`, traceID, RunApprovalPending).Scan(
		&run.ID, &run.TraceID, &run.AgentID, &run.Goal, &run.Source, &run.Status,
		&run.TerminationReason, &run.Iterations, &run.PromptTokens,
		&run.CompletionTokens, &run.TotalTokens, &run.StartedAt, &run.UpdatedAt,
		&completed, &checkpointJSON,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("loading approval checkpoint: %w", err)
	}
	if completed.Valid {
		run.CompletedAt = &completed.Time
	}
	var checkpoint RunCheckpoint
	if checkpointJSON == "" {
		return nil, nil, errors.New("approval-pending run has no checkpoint")
	}
	if err := json.Unmarshal([]byte(checkpointJSON), &checkpoint); err != nil {
		return nil, nil, fmt.Errorf("decoding run checkpoint: %w", err)
	}
	return &checkpoint, &run, nil
}

// List returns recent durable executions.
func (s *RunStore) List(ctx context.Context, limit int) ([]AgentRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trace_id, agent_id, goal, source, status, termination_reason,
		       iterations, prompt_tokens, completion_tokens, total_tokens,
		       started_at, updated_at, completed_at
		FROM agent_runs ORDER BY started_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing agent runs: %w", err)
	}
	defer rows.Close()
	var runs []AgentRun
	for rows.Next() {
		var run AgentRun
		var completed sql.NullTime
		if err := rows.Scan(
			&run.ID, &run.TraceID, &run.AgentID, &run.Goal, &run.Source, &run.Status,
			&run.TerminationReason, &run.Iterations, &run.PromptTokens,
			&run.CompletionTokens, &run.TotalTokens, &run.StartedAt, &run.UpdatedAt, &completed,
		); err != nil {
			return nil, fmt.Errorf("scanning agent run: %w", err)
		}
		if completed.Valid {
			run.CompletedAt = &completed.Time
		}
		runs = append(runs, run)
	}
	if runs == nil {
		runs = []AgentRun{}
	}
	return runs, rows.Err()
}

// Events returns the append-only event sequence for a run.
func (s *RunStore) Events(ctx context.Context, runID string) ([]RunEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, trace_id, step, type, status, COALESCE(tool_name, ''),
		       data_json, duration_ms, created_at
		FROM run_events WHERE run_id = ? ORDER BY created_at ASC, rowid ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("listing run events: %w", err)
	}
	defer rows.Close()
	var events []RunEvent
	for rows.Next() {
		var event RunEvent
		var data string
		if err := rows.Scan(
			&event.ID, &event.RunID, &event.TraceID, &event.Step, &event.Type,
			&event.Status, &event.ToolName, &data, &event.DurationMS, &event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning run event: %w", err)
		}
		_ = json.Unmarshal([]byte(data), &event.Data)
		events = append(events, event)
	}
	if events == nil {
		events = []RunEvent{}
	}
	return events, rows.Err()
}

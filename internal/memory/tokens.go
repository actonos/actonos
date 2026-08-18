package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/llm"
)

// TokenUsageRecord represents a single token usage ledger event.
type TokenUsageRecord struct {
	ID               string    `json:"id"`
	Timestamp        time.Time `json:"timestamp"`
	AgentID          string    `json:"agent_id"`
	Model            string    `json:"model"`
	Provider         string    `json:"provider"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	Source           string    `json:"source"` // "chat", "stream", "cron", "heartbeat", "swarm", "channel"
	ConversationID   string    `json:"conversation_id,omitempty"`
}

// TokenUsageSummary aggregates token usage over specific timeframes.
type TokenUsageSummary struct {
	TotalPromptTokens     int64             `json:"total_prompt_tokens"`
	TotalCompletionTokens int64             `json:"total_completion_tokens"`
	TotalTokens           int64             `json:"total_tokens"`
	TotalCostUSD          float64           `json:"total_cost_usd"`
	TodayTokens           int64             `json:"today_tokens"`
	TodayCostUSD          float64           `json:"today_cost_usd"`
	MonthTokens           int64             `json:"month_tokens"`
	MonthCostUSD          float64           `json:"month_cost_usd"`
	ByModel               []ModelUsageStat  `json:"by_model"`
	ByAgent               []AgentUsageStat  `json:"by_agent"`
	DailyTrend            []DailyUsagePoint `json:"daily_trend"`
}

// ModelUsageStat represents aggregated usage for a specific model.
type ModelUsageStat struct {
	Model       string  `json:"model"`
	TotalTokens int64   `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
	Percentage  float64 `json:"percentage"`
}

// AgentUsageStat represents aggregated usage for a specific agent.
type AgentUsageStat struct {
	AgentID     string  `json:"agent_id"`
	TotalTokens int64   `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
	Percentage  float64 `json:"percentage"`
}

// DailyUsagePoint represents token usage on a single day.
type DailyUsagePoint struct {
	Date             string  `json:"date"` // YYYY-MM-DD
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// CalculateEstimatedCost estimates the cost of a request in USD using the canonical pricing catalog.
func CalculateEstimatedCost(model string, promptTokens, completionTokens int) float64 {
	prompt1M, compl1M := llm.GetModelPricing(model)
	cost := (float64(promptTokens) / 1000000.0 * prompt1M) + (float64(completionTokens) / 1000000.0 * compl1M)
	return cost
}

// TokenTracker manages recording and querying token metrics in SQLite.
type TokenTracker struct {
	mu sync.RWMutex
	db *sql.DB
}

// NewTokenTracker initializes a new TokenTracker.
func NewTokenTracker(db *sql.DB) *TokenTracker {
	return &TokenTracker{db: db}
}

// Record inserts a new token usage record asynchronously or synchronously.
func (t *TokenTracker) Record(ctx context.Context, rec TokenUsageRecord) error {
	if t.db == nil {
		return nil
	}

	if rec.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		rec.ID = "tok_" + hex.EncodeToString(b)
	}

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}

	if rec.TotalTokens == 0 {
		rec.TotalTokens = rec.PromptTokens + rec.CompletionTokens
	}

	if rec.EstimatedCostUSD == 0 {
		rec.EstimatedCostUSD = CalculateEstimatedCost(rec.Model, rec.PromptTokens, rec.CompletionTokens)
	}

	query := `
	INSERT INTO token_usage (
		id, timestamp, agent_id, model, provider,
		prompt_tokens, completion_tokens, total_tokens,
		estimated_cost_usd, source, conversation_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := t.db.ExecContext(
		ctx,
		query,
		rec.ID,
		rec.Timestamp,
		rec.AgentID,
		rec.Model,
		rec.Provider,
		rec.PromptTokens,
		rec.CompletionTokens,
		rec.TotalTokens,
		rec.EstimatedCostUSD,
		rec.Source,
		rec.ConversationID,
	)
	if err != nil {
		slog.Warn("failed to record token usage", "error", err, "agent_id", rec.AgentID)
		return err
	}
	return nil
}

// GetSummary calculates aggregated statistics for today, this month, and lifetime.
func (t *TokenTracker) GetSummary(ctx context.Context) (*TokenUsageSummary, error) {
	if t.db == nil {
		return &TokenUsageSummary{}, nil
	}

	summary := &TokenUsageSummary{
		ByModel:    []ModelUsageStat{},
		ByAgent:    []AgentUsageStat{},
		DailyTrend: []DailyUsagePoint{},
	}

	// 1. Overall Lifetime Aggregates
	row := t.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(estimated_cost_usd), 0)
		FROM token_usage
	`)
	_ = row.Scan(&summary.TotalPromptTokens, &summary.TotalCompletionTokens, &summary.TotalTokens, &summary.TotalCostUSD)

	// 2. Today's Aggregates (UTC)
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	rowToday := t.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(estimated_cost_usd), 0)
		FROM token_usage
		WHERE timestamp >= ?
	`, todayStart)
	_ = rowToday.Scan(&summary.TodayTokens, &summary.TodayCostUSD)

	// 3. This Month's Aggregates
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rowMonth := t.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(estimated_cost_usd), 0)
		FROM token_usage
		WHERE timestamp >= ?
	`, monthStart)
	_ = rowMonth.Scan(&summary.MonthTokens, &summary.MonthCostUSD)

	// 4. By Model
	rowsModel, err := t.db.QueryContext(ctx, `
		SELECT
			model,
			COALESCE(SUM(total_tokens), 0) as tokens,
			COALESCE(SUM(estimated_cost_usd), 0) as cost
		FROM token_usage
		GROUP BY model
		ORDER BY tokens DESC
		LIMIT 10
	`)
	if err == nil {
		defer rowsModel.Close()
		for rowsModel.Next() {
			var st ModelUsageStat
			if err := rowsModel.Scan(&st.Model, &st.TotalTokens, &st.CostUSD); err == nil {
				if summary.TotalTokens > 0 {
					st.Percentage = float64(st.TotalTokens) / float64(summary.TotalTokens) * 100.0
				}
				summary.ByModel = append(summary.ByModel, st)
			}
		}
	}

	// 5. By Agent
	rowsAgent, err := t.db.QueryContext(ctx, `
		SELECT
			agent_id,
			COALESCE(SUM(total_tokens), 0) as tokens,
			COALESCE(SUM(estimated_cost_usd), 0) as cost
		FROM token_usage
		GROUP BY agent_id
		ORDER BY tokens DESC
		LIMIT 10
	`)
	if err == nil {
		defer rowsAgent.Close()
		for rowsAgent.Next() {
			var st AgentUsageStat
			if err := rowsAgent.Scan(&st.AgentID, &st.TotalTokens, &st.CostUSD); err == nil {
				if summary.TotalTokens > 0 {
					st.Percentage = float64(st.TotalTokens) / float64(summary.TotalTokens) * 100.0
				}
				summary.ByAgent = append(summary.ByAgent, st)
			}
		}
	}

	// 6. Daily Trend (last 14 days)
	fourteenDaysAgo := now.AddDate(0, 0, -14).Truncate(24 * time.Hour)
	rowsDaily, err := t.db.QueryContext(ctx, `
		SELECT
			strftime('%Y-%m-%d', timestamp) as day,
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(estimated_cost_usd), 0)
		FROM token_usage
		WHERE timestamp >= ?
		GROUP BY day
		ORDER BY day ASC
	`, fourteenDaysAgo)
	if err == nil {
		defer rowsDaily.Close()
		for rowsDaily.Next() {
			var dp DailyUsagePoint
			if err := rowsDaily.Scan(&dp.Date, &dp.PromptTokens, &dp.CompletionTokens, &dp.TotalTokens, &dp.CostUSD); err == nil {
				summary.DailyTrend = append(summary.DailyTrend, dp)
			}
		}
	}

	return summary, nil
}

// GetAgentMonthlyCost returns the total cost incurred by a specific agent during the current month.
func (t *TokenTracker) GetAgentMonthlyCost(ctx context.Context, agentID string) (float64, error) {
	if t.db == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	row := t.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(estimated_cost_usd), 0)
		FROM token_usage
		WHERE agent_id = ? AND timestamp >= ?
	`, agentID, monthStart)

	var cost float64
	err := row.Scan(&cost)
	return cost, err
}

// GetHistory returns recent token usage ledger records with optional filtering.
func (t *TokenTracker) GetHistory(ctx context.Context, limit int, agentID, source string) ([]TokenUsageRecord, error) {
	if t.db == nil {
		return []TokenUsageRecord{}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `
		SELECT id, timestamp, agent_id, model, provider, prompt_tokens, completion_tokens, total_tokens, estimated_cost_usd, source, COALESCE(conversation_id, '')
		FROM token_usage
		WHERE 1=1
	`
	var args []any

	if agentID != "" && agentID != "all" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if source != "" && source != "all" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TokenUsageRecord
	for rows.Next() {
		var r TokenUsageRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.AgentID, &r.Model, &r.Provider, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.EstimatedCostUSD, &r.Source, &r.ConversationID); err == nil {
			records = append(records, r)
		}
	}
	if records == nil {
		records = []TokenUsageRecord{}
	}
	return records, nil
}

package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var nonAlphaNumRegex = regexp.MustCompile(`[^\w\s]`)

// FTSResult represents a lexical match from SQLite FTS5.
type FTSResult struct {
	ID        string  `json:"id"`
	AgentID   string  `json:"agent_id"`
	Layer     string  `json:"layer"`
	Content   string  `json:"content"`
	BM25Rank  float64 `json:"bm25_rank"` // Negative number in SQLite FTS5 (lower/more negative = more relevant)
	NormScore float64 `json:"norm_score"` // Normalized 0.0 - 1.0 (higher = more relevant)
}

// IndexMemoryFTS inserts or updates a memory record in the FTS5 index.
func (d *DB) IndexMemoryFTS(ctx context.Context, id, agentID, layer, content string) error {
	// First delete existing if any
	_ = d.DeleteMemoryFTS(ctx, id)

	query := `INSERT INTO memories_fts (id, agent_id, layer, content) VALUES (?, ?, ?, ?)`
	_, err := d.db.ExecContext(ctx, query, id, agentID, layer, content)
	return err
}

// DeleteMemoryFTS removes a record from the FTS5 index.
func (d *DB) DeleteMemoryFTS(ctx context.Context, id string) error {
	query := `DELETE FROM memories_fts WHERE id = ?`
	_, err := d.db.ExecContext(ctx, query, id)
	return err
}

// sanitizeFTSQuery cleans user query string for FTS5 boolean/phrase syntax safety.
func sanitizeFTSQuery(query string) string {
	cleaned := nonAlphaNumRegex.ReplaceAllString(query, " ")
	tokens := strings.Fields(cleaned)
	if len(tokens) == 0 {
		return ""
	}
	// Join words with OR and prefix wildcards
	var parts []string
	for _, t := range tokens {
		if len(t) > 0 {
			parts = append(parts, fmt.Sprintf(`"%s"*`, t))
		}
	}
	return strings.Join(parts, " OR ")
}

// SearchFTS queries SQLite FTS5 using BM25 ranking.
func (d *DB) SearchFTS(ctx context.Context, agentID, layer, queryStr string, limit int) ([]FTSResult, error) {
	if limit <= 0 {
		limit = 10
	}

	sanitized := sanitizeFTSQuery(queryStr)
	if sanitized == "" {
		return nil, nil
	}

	var query string
	var args []any

	if agentID != "" && layer != "" {
		query = `
			SELECT id, agent_id, layer, content, rank
			FROM memories_fts
			WHERE memories_fts MATCH ? AND agent_id = ? AND layer = ?
			ORDER BY rank
			LIMIT ?
		`
		args = []any{sanitized, agentID, layer, limit}
	} else if agentID != "" {
		query = `
			SELECT id, agent_id, layer, content, rank
			FROM memories_fts
			WHERE memories_fts MATCH ? AND agent_id = ?
			ORDER BY rank
			LIMIT ?
		`
		args = []any{sanitized, agentID, limit}
	} else {
		query = `
			SELECT id, agent_id, layer, content, rank
			FROM memories_fts
			WHERE memories_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		`
		args = []any{sanitized, limit}
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying fts5: %w", err)
	}
	defer rows.Close()

	var results []FTSResult
	for rows.Next() {
		var item FTSResult
		if err := rows.Scan(&item.ID, &item.AgentID, &item.Layer, &item.Content, &item.BM25Rank); err != nil {
			return nil, fmt.Errorf("scanning fts5 row: %w", err)
		}
		// In SQLite FTS5, rank is negative; smaller (more negative) is better.
		// Normalize to [0, 1] range: score = 1 / (1 + abs(rank))
		rankAbs := item.BM25Rank
		if rankAbs < 0 {
			rankAbs = -rankAbs
		}
		item.NormScore = 1.0 / (1.0 + rankAbs)

		results = append(results, item)
	}

	return results, rows.Err()
}

package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// EmbedQueryVector returns a single E5 query embedding for hybrid retrieval.
func (s *EmbeddingService) EmbedQueryVector(ctx context.Context, query string) ([]float32, error) {
	if s == nil || s.embedder == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	vectors, err := s.embedder.EmbedQuery(ctx, []string{strings.TrimSpace(query)})
	if err != nil || len(vectors) == 0 {
		return nil, err
	}
	return vectors[0], nil
}

func (s *EmbeddingService) Search(ctx context.Context, query string, scopes []string, limit int) ([]SemanticRecord, error) {
	if s == nil || s.embedder == nil || s.vectorStore == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 6
	}
	vectors, err := s.embedder.EmbedQuery(ctx, []string{strings.TrimSpace(query)})
	if err != nil || len(vectors) != 1 {
		return nil, err
	}
	allowed := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		allowed[scope] = true
	}
	var results []VectorResult
	if len(allowed) == 0 {
		results, err = s.vectorStore.SearchByEmbedding(ctx, semanticCollection, vectors[0], limit)
		if err != nil {
			return nil, err
		}
	} else {
		byID := make(map[string]VectorResult)
		for scope := range allowed {
			scopeResults, searchErr := s.vectorStore.SearchByEmbeddingFiltered(
				ctx, semanticCollection, vectors[0], limit, map[string]string{"scope": scope},
			)
			if searchErr != nil {
				return nil, searchErr
			}
			for _, result := range scopeResults {
				if current, ok := byID[result.ID]; !ok || result.Similarity > current.Similarity {
					byID[result.ID] = result
				}
			}
		}
		results = make([]VectorResult, 0, len(byID))
		for _, result := range byID {
			results = append(results, result)
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Similarity > results[j].Similarity })
	}
	var records []SemanticRecord
	for _, result := range results {
		var record SemanticRecord
		var metadataJSON string
		err := s.db.QueryRowContext(ctx, `
			SELECT c.id, c.content, s.source_type, s.source_ref, s.agent_id, s.scope,
			       s.conversation_id, c.metadata_json
			FROM semantic_chunks c JOIN semantic_sources s ON s.id = c.source_id
			WHERE c.id = ? AND c.active = 1 AND s.state = 'active'
		`, result.ID).Scan(&record.ID, &record.Content, &record.SourceType, &record.SourceRef,
			&record.AgentID, &record.Scope, &record.ConversationID, &metadataJSON)
		if err != nil || (len(allowed) > 0 && !allowed[record.Scope]) {
			continue
		}
		_ = json.Unmarshal([]byte(metadataJSON), &record.Metadata)
		record.Similarity = float64(result.Similarity)
		records = append(records, record)
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

func (s *EmbeddingService) Status(ctx context.Context) (EmbeddingStatus, error) {
	status := EmbeddingStatus{ModelID: EmbeddingModelID, ModelRevision: EmbeddingModelRevision, Dimension: EmbeddingDimension}
	if s == nil || s.db == nil {
		return status, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM embedding_jobs GROUP BY status`)
	if err != nil {
		return status, err
	}
	for rows.Next() {
		var state string
		var count int
		_ = rows.Scan(&state, &count)
		switch state {
		case "pending":
			status.Pending = count
		case "running":
			status.Running = count
		case "dead":
			status.Dead = count
		}
	}
	rows.Close()
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_sources WHERE state = 'active'`).Scan(&status.IndexedSources)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_chunks c
		JOIN semantic_sources s ON s.id = c.source_id WHERE c.active = 1 AND s.state = 'active'`).Scan(&status.ActiveChunks)
	var oldest sql.NullTime
	_ = s.db.QueryRowContext(ctx, `SELECT MIN(due_at) FROM embedding_jobs WHERE status = 'pending'`).Scan(&oldest)
	if oldest.Valid {
		status.OldestDueAt = &oldest.Time
	}
	if s.embedder != nil {
		healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		status.ServiceReady = s.embedder.Health(healthCtx) == nil
	}
	return status, nil
}

func chunkText(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	paragraphs := strings.Split(content, "\n\n")
	const targetRunes = 800
	const overlapRunes = 120
	var chunks []string
	var current strings.Builder
	flush := func() {
		value := strings.TrimSpace(current.String())
		if value == "" {
			current.Reset()
			return
		}
		for len([]rune(value)) > targetRunes {
			runes := []rune(value)
			chunk := strings.TrimSpace(string(runes[:targetRunes]))
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
			value = strings.TrimSpace(string(runes[targetRunes-overlapRunes:]))
		}
		if value != "" {
			chunks = append(chunks, value)
		}
		current.Reset()
	}
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if len([]rune(current.String()))+len([]rune(paragraph))+2 > targetRunes {
			flush()
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(paragraph)
	}
	flush()
	return chunks
}

func stableID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:])
}

func hashText(value string) string { return stableID(value) }

func approximateTokenCount(value string) int {
	count := len([]rune(value)) / 3
	if count < 1 {
		return 1
	}
	return count
}

func cloneMetadata(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+8)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 2000 {
		return value[:2000]
	}
	return value
}

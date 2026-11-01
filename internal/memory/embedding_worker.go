package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

func (s *EmbeddingService) run(ctx context.Context) {
	s.recoverOrphanedJobs(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
		for {
			job, err := s.claim(ctx)
			if err != nil || job == nil {
				break
			}
			start := time.Now()
			slog.Info("processing embedding job",
				"job_id", job.ID,
				"source_type", job.SourceType,
				"source_ref", job.SourceRef,
				"operation", job.Operation,
				"agent_id", job.AgentID,
				"scope", job.Scope,
				"generation", job.Generation,
			)
			err = s.process(ctx, *job)
			if err != nil {
				slog.Error("embedding job failed",
					"job_id", job.ID,
					"source_type", job.SourceType,
					"source_ref", job.SourceRef,
					"operation", job.Operation,
					"error", err,
					"attempts", job.Attempts+1,
					"duration", time.Since(start).Round(time.Millisecond),
				)
				s.fail(ctx, *job, err)
			} else {
				slog.Info("embedding job completed",
					"job_id", job.ID,
					"source_type", job.SourceType,
					"source_ref", job.SourceRef,
					"operation", job.Operation,
					"duration", time.Since(start).Round(time.Millisecond),
				)
				s.complete(ctx, *job)
			}
		}
	}
}

func (s *EmbeddingService) reviveDeadJobs(ctx context.Context) {
	if s.db == nil || s.embedder == nil {
		return
	}
	if err := s.embedder.Health(ctx); err != nil {
		return
	}
	result, err := s.db.ExecContext(ctx, `UPDATE embedding_jobs SET status = 'pending', due_at = ?, last_error = '', updated_at = ? WHERE status = 'dead'`, s.now(), s.now())
	if err == nil {
		if rows, _ := result.RowsAffected(); rows > 0 {
			slog.Info("revived dead embedding jobs after helper recovery", "count", rows)
		}
	}
}

func (s *EmbeddingService) recoverOrphanedJobs(ctx context.Context) {
	if s.db == nil {
		return
	}
	result, err := s.db.ExecContext(ctx, `UPDATE embedding_jobs SET status = 'pending', lease_until = NULL WHERE status = 'running'`)
	if err == nil {
		if rows, _ := result.RowsAffected(); rows > 0 {
			slog.Info("recovered orphaned running embedding jobs on startup", "count", rows)
		}
	}
}

func (s *EmbeddingService) claim(ctx context.Context) (*EmbeddingJob, error) {
	if s.embedder == nil || s.vectorStore == nil {
		return nil, nil
	}
	if err := s.embedder.Health(ctx); err != nil {
		return nil, nil
	}
	s.reviveDeadJobs(ctx)
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(ctx, `UPDATE embedding_jobs SET status = 'pending', lease_until = NULL
		WHERE status = 'running' AND lease_until < ?`, now)
	var job EmbeddingJob
	err = tx.QueryRowContext(ctx, `
		SELECT id, source_type, source_key, source_ref, operation, agent_id, scope,
		       conversation_id, attempts, generation
		FROM embedding_jobs
		WHERE status = 'pending' AND due_at <= ?
		ORDER BY due_at, created_at LIMIT 1
	`, now).Scan(&job.ID, &job.SourceType, &job.SourceKey, &job.SourceRef, &job.Operation,
		&job.AgentID, &job.Scope, &job.ConversationID, &job.Attempts, &job.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE embedding_jobs
		SET status = 'running', lease_until = ?, updated_at = ?
		WHERE id = ? AND generation = ? AND status = 'pending'`, now.Add(embeddingLeaseDuration), now, job.ID, job.Generation)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *EmbeddingService) process(ctx context.Context, job EmbeddingJob) error {
	if job.Operation == EmbeddingDelete {
		return s.deleteSource(ctx, job)
	}
	content, metadata, err := s.loadContent(ctx, job)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
			return s.deleteSource(ctx, job)
		}
		if errors.Is(err, errUnsupportedEmbeddingSource) {
			return s.markUnsupported(ctx, job, err)
		}
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return s.deleteSource(ctx, job)
	}
	contentHash := hashText(content)
	sourceID := stableID("source", job.SourceType, job.SourceKey)
	var existingHash string
	var existingAgentID, existingScope, existingConversationID, existingModelID, existingModelRevision, existingChunkerVersion string
	var existingDimension int
	err = s.db.QueryRowContext(ctx, `SELECT content_hash, agent_id, scope, conversation_id,
		model_id, model_revision, dimension, chunker_version FROM semantic_sources
		WHERE id = ? AND state = 'active'`, sourceID).Scan(&existingHash, &existingAgentID,
		&existingScope, &existingConversationID, &existingModelID, &existingModelRevision,
		&existingDimension, &existingChunkerVersion)
	if err == nil && existingHash == contentHash && existingAgentID == job.AgentID &&
		existingScope == job.Scope && existingConversationID == job.ConversationID &&
		existingModelID == EmbeddingModelID && existingModelRevision == EmbeddingModelRevision &&
		existingDimension == EmbeddingDimension && existingChunkerVersion == chunkerVersion {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking semantic source: %w", err)
	}

	chunks := chunkText(content)
	if len(chunks) == 0 {
		return s.deleteSource(ctx, job)
	}
	slog.Info("chunked content for embedding",
		"job_id", job.ID,
		"source_ref", job.SourceRef,
		"chunks", len(chunks),
		"content_len", len(content),
	)
	passages := make([]string, len(chunks))
	for index, chunk := range chunks {
		passages[index] = chunk
	}
	embeddings, err := s.embedder.EmbedPassages(ctx, passages)
	if err != nil {
		return fmt.Errorf("embedding source %s: %w", job.SourceKey, err)
	}
	if len(embeddings) != len(chunks) {
		return fmt.Errorf("embedding count mismatch for source %s", job.SourceKey)
	}

	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO semantic_sources (
			id, source_type, source_key, source_ref, agent_id, scope, conversation_id,
			content_hash, active_generation, model_id, model_revision, dimension,
			chunker_version, state, size_bytes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, 'indexing', ?, ?, ?)
		ON CONFLICT(source_type, source_key) DO UPDATE SET
			source_ref = excluded.source_ref, size_bytes = excluded.size_bytes,
			deleted_at = NULL, updated_at = excluded.updated_at
	`, sourceID, job.SourceType, job.SourceKey, job.SourceRef, job.AgentID, job.Scope,
		job.ConversationID, contentHash, EmbeddingModelID, EmbeddingModelRevision,
		EmbeddingDimension, chunkerVersion, len(content), now, now)
	if err != nil {
		return fmt.Errorf("upserting semantic source: %w", err)
	}

	vectorDocs := make([]VectorDocument, 0, len(chunks))
	for index, chunk := range chunks {
		chunkID := stableID(sourceID, fmt.Sprint(job.Generation), fmt.Sprint(index), hashText(chunk))
		meta := cloneMetadata(metadata)
		meta["source_type"] = job.SourceType
		meta["source_ref"] = job.SourceRef
		meta["scope"] = job.Scope
		meta["agent_id"] = job.AgentID
		meta["conversation_id"] = job.ConversationID
		meta["ordinal"] = index
		metaJSON, _ := json.Marshal(meta)
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO semantic_chunks
			(id, source_id, generation, ordinal, content, content_hash, token_count, metadata_json, active, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`, chunkID, sourceID, job.Generation,
			index, chunk, hashText(chunk), approximateTokenCount(chunk), string(metaJSON), now)
		if err != nil {
			return fmt.Errorf("staging semantic chunk: %w", err)
		}
		strMeta := make(map[string]string, len(meta)+2)
		for key, value := range meta {
			strMeta[key] = fmt.Sprint(value)
		}
		strMeta["source_id"] = sourceID
		strMeta["generation"] = fmt.Sprint(job.Generation)
		vectorDocs = append(vectorDocs, VectorDocument{ID: chunkID, Content: chunk, Embedding: embeddings[index], Metadata: strMeta})
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if err := s.vectorStore.IndexDocuments(ctx, semanticCollection, vectorDocs); err != nil {
		return fmt.Errorf("indexing semantic vectors: %w", err)
	}

	tx, err = s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentGeneration int
	var currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT generation, status FROM embedding_jobs WHERE id = ?`, job.ID).Scan(&currentGeneration, &currentStatus); err != nil {
		return fmt.Errorf("checking embedding job generation: %w", err)
	}
	if currentGeneration != job.Generation || currentStatus != "running" {
		orphanIDs := make([]string, 0, len(vectorDocs))
		for _, document := range vectorDocs {
			orphanIDs = append(orphanIDs, document.ID)
		}
		_ = tx.Rollback()
		_ = s.vectorStore.DeleteDocuments(context.Background(), semanticCollection, orphanIDs)
		_, _ = s.db.ExecContext(context.Background(), `DELETE FROM semantic_chunks WHERE source_id = ? AND generation = ? AND active = 0`, sourceID, job.Generation)
		return nil
	}
	var oldIDs []string
	rows, err := tx.QueryContext(ctx, `SELECT id FROM semantic_chunks
		WHERE source_id = ? AND active = 1 AND generation <> ?`, sourceID, job.Generation)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				oldIDs = append(oldIDs, id)
			}
		}
		rows.Close()
	}
	_, err = tx.ExecContext(ctx, `UPDATE semantic_chunks SET active = CASE WHEN generation = ? THEN 1 ELSE 0 END
		WHERE source_id = ?`, job.Generation, sourceID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE semantic_sources SET source_ref = ?, agent_id = ?, scope = ?,
		conversation_id = ?, content_hash = ?, active_generation = ?, model_id = ?,
		model_revision = ?, dimension = ?, chunker_version = ?, state = 'active', size_bytes = ?,
		indexed_at = ?, deleted_at = NULL, updated_at = ? WHERE id = ?`, job.SourceRef, job.AgentID,
		job.Scope, job.ConversationID, contentHash, job.Generation, EmbeddingModelID,
		EmbeddingModelRevision, EmbeddingDimension, chunkerVersion, len(content), now, now, sourceID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = s.vectorStore.DeleteDocuments(context.Background(), semanticCollection, oldIDs)
	slog.Info("indexed semantic chunks", "job_id", job.ID, "source_ref", job.SourceRef, "chunks", len(chunks))
	return nil
}

func (s *EmbeddingService) loadContent(ctx context.Context, job EmbeddingJob) (string, map[string]any, error) {
	switch job.SourceType {
	case "message":
		var role, content string
		err := s.db.QueryRowContext(ctx, `SELECT role, content FROM messages WHERE id = ?`, job.SourceRef).Scan(&role, &content)
		return content, map[string]any{"role": role}, err
	case "memory":
		var content string
		err := s.db.QueryRowContext(ctx, `SELECT content FROM memories WHERE id = ?`, job.SourceRef).Scan(&content)
		return content, map[string]any{"layer": "episodic"}, err
	case "file":
		info, err := os.Stat(job.SourceRef)
		if err != nil {
			return "", nil, err
		}
		if info.IsDir() {
			return "", nil, fmt.Errorf("%w: source is a directory", errUnsupportedEmbeddingSource)
		}
		if info.Size() > maxEmbeddingFileSize {
			return "", nil, fmt.Errorf("%w: file exceeds limit of %d bytes", errUnsupportedEmbeddingSource, maxEmbeddingFileSize)
		}
		slog.Info("extracting document text for embedding", "job_id", job.ID, "file", job.SourceRef, "size_bytes", info.Size())
		text, err := extractDocumentText(job.SourceRef)
		if err != nil {
			return "", nil, err
		}
		slog.Info("extracted document text successfully", "job_id", job.ID, "file", job.SourceRef, "chars", len(text))
		return text, map[string]any{
			"path": filepath.ToSlash(job.SourceRef), "filename": filepath.Base(job.SourceRef),
		}, nil
	case "workspace_file":
		if s.workspaceStore == nil {
			return "", nil, errors.New("workspace store is unavailable")
		}
		node, data, err := s.workspaceStore.Read(ctx, job.SourceRef, 0, 0)
		if errors.Is(err, workspacepkg.ErrNotFound) {
			return "", nil, os.ErrNotExist
		}
		if err != nil {
			return "", nil, fmt.Errorf("reading workspace file %s: %w", job.SourceRef, err)
		}
		if node.SizeBytes > maxEmbeddingFileSize {
			return "", nil, fmt.Errorf("%w: file exceeds limit of %d bytes", errUnsupportedEmbeddingSource, maxEmbeddingFileSize)
		}
		text, err := extractDocumentBytes(node.Name, node.MIMEType, data)
		if err != nil {
			return "", nil, err
		}
		return text, map[string]any{
			"file_id": node.ID, "virtual_path": node.VirtualPath, "filename": node.Name,
			"mime_type": node.MIMEType, "version": node.Version,
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported embedding source type %q", job.SourceType)
	}
}

func (s *EmbeddingService) deleteSource(ctx context.Context, job EmbeddingJob) error {
	sourceID := stableID("source", job.SourceType, job.SourceKey)
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM semantic_chunks WHERE source_id = ?`, sourceID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	now := s.now()
	_, err = s.db.ExecContext(ctx, `UPDATE semantic_chunks SET active = 0 WHERE source_id = ?`, sourceID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE semantic_sources SET state = 'deleted', deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, sourceID)
	if err != nil {
		return err
	}
	slog.Info("deleted semantic source", "job_id", job.ID, "source_type", job.SourceType, "source_ref", job.SourceRef, "chunks_deleted", len(ids))
	return s.vectorStore.DeleteDocuments(ctx, semanticCollection, ids)
}

func (s *EmbeddingService) tombstoneSource(ctx context.Context, job EmbeddingJob) error {
	sourceID := stableID("source", job.SourceType, job.SourceKey)
	now := s.now()
	_, err := s.db.ExecContext(ctx, `UPDATE semantic_sources SET state = 'deleted', deleted_at = ?,
		updated_at = ? WHERE id = ?`, now, now, sourceID)
	if err != nil {
		return fmt.Errorf("tombstoning semantic source: %w", err)
	}
	return nil
}

func (s *EmbeddingService) markUnsupported(ctx context.Context, job EmbeddingJob, cause error) error {
	sourceID := stableID("source", job.SourceType, job.SourceKey)
	if err := s.deleteSource(ctx, job); err != nil {
		return err
	}
	now := s.now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO semantic_sources (
		id, source_type, source_key, source_ref, agent_id, scope, conversation_id,
		state, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 'unsupported', ?, ?)
	ON CONFLICT(source_type, source_key) DO UPDATE SET source_ref = excluded.source_ref,
		agent_id = excluded.agent_id, scope = excluded.scope,
		conversation_id = excluded.conversation_id, state = 'unsupported',
		content_hash = '', active_generation = 0, indexed_at = NULL,
		deleted_at = NULL, updated_at = excluded.updated_at`, sourceID, job.SourceType,
		job.SourceKey, job.SourceRef, job.AgentID, job.Scope, job.ConversationID, now, now)
	if err != nil {
		return fmt.Errorf("marking unsupported semantic source (%s): %w", truncateError(cause), err)
	}
	slog.Warn("embedding source marked unsupported", "job_id", job.ID, "source_type", job.SourceType, "source_ref", job.SourceRef, "reason", truncateError(cause))
	return nil
}

func (s *EmbeddingService) complete(ctx context.Context, job EmbeddingJob) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM embedding_jobs WHERE id = ? AND generation = ?`, job.ID, job.Generation)
}

func (s *EmbeddingService) fail(ctx context.Context, job EmbeddingJob, cause error) {
	attempts := job.Attempts + 1
	status := "pending"
	if attempts >= 8 {
		status = "dead"
	}
	backoff := time.Duration(1<<min(attempts, 6)) * time.Minute
	_, _ = s.db.ExecContext(ctx, `UPDATE embedding_jobs SET status = ?, attempts = ?, due_at = ?,
		lease_until = NULL, last_error = ?, updated_at = ? WHERE id = ? AND generation = ?`,
		status, attempts, s.now().Add(backoff), truncateError(cause), s.now(), job.ID, job.Generation)
}

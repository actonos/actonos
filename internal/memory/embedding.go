package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	EmbeddingModelID       = "intfloat/multilingual-e5-small"
	EmbeddingModelRevision = "614241f622f53c4eeff9890bdc4f31cfecc418b3"
	EmbeddingDimension     = 384
	semanticCollection     = "semantic_documents"
	chunkerVersion         = "paragraph-v2"
	defaultEmbeddingDelay  = time.Minute
	embeddingLeaseDuration = 5 * time.Minute
	maxEmbeddingFileSize   = 10 << 20
	maxEmbeddingBatchSize  = 32
)

var errUnsupportedEmbeddingSource = errors.New("unsupported embedding source")

type EmbeddingOperation string

const (
	EmbeddingUpsert EmbeddingOperation = "upsert"
	EmbeddingDelete EmbeddingOperation = "delete"
)

// Embedder creates normalized E5 query and passage embeddings.
type Embedder interface {
	EmbedQuery(ctx context.Context, texts []string) ([][]float32, error)
	EmbedPassages(ctx context.Context, texts []string) ([][]float32, error)
	Health(ctx context.Context) error
}

// HTTPEmbedder calls the local embeddingd helper without exposing it externally.
type HTTPEmbedder struct {
	baseURL string
	client  *http.Client
}

func NewHTTPEmbedder(baseURL string) *HTTPEmbedder {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:8091"
	}
	return &HTTPEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 2 * time.Minute},
	}
}

func (e *HTTPEmbedder) EmbedQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, "query", texts)
}

func (e *HTTPEmbedder) EmbedPassages(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, "passage", texts)
}

func (e *HTTPEmbedder) embed(ctx context.Context, kind string, texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxEmbeddingBatchSize {
		end := min(start+maxEmbeddingBatchSize, len(texts))
		batch, err := e.embedBatch(ctx, kind, texts[start:end])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (e *HTTPEmbedder) embedBatch(ctx context.Context, kind string, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{"kind": kind, "texts": texts})
	if err != nil {
		return nil, fmt.Errorf("marshalling embedding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling embedding service: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("reading embedding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decoding embedding response: %w", err)
	}
	if len(response.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding service returned %d vectors for %d texts", len(response.Embeddings), len(texts))
	}
	for _, vector := range response.Embeddings {
		if len(vector) != EmbeddingDimension {
			return nil, fmt.Errorf("embedding dimension is %d, expected %d", len(vector), EmbeddingDimension)
		}
	}
	return response.Embeddings, nil
}

func (e *HTTPEmbedder) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embedding service health returned %s", resp.Status)
	}
	return nil
}

type EmbeddingJob struct {
	ID             string
	SourceType     string
	SourceKey      string
	SourceRef      string
	Operation      EmbeddingOperation
	AgentID        string
	Scope          string
	ConversationID string
	Attempts       int
	Generation     int
}

type EmbeddingStatus struct {
	Pending        int        `json:"pending"`
	Running        int        `json:"running"`
	Dead           int        `json:"dead"`
	IndexedSources int        `json:"indexed_sources"`
	ActiveChunks   int        `json:"active_chunks"`
	OldestDueAt    *time.Time `json:"oldest_due_at,omitempty"`
	ModelID        string     `json:"model_id"`
	ModelRevision  string     `json:"model_revision"`
	Dimension      int        `json:"dimension"`
	ServiceReady   bool       `json:"service_ready"`
}

type SemanticRecord struct {
	ID             string         `json:"id"`
	Content        string         `json:"content"`
	SourceType     string         `json:"source_type"`
	SourceRef      string         `json:"source_ref"`
	AgentID        string         `json:"agent_id,omitempty"`
	Scope          string         `json:"scope"`
	ConversationID string         `json:"conversation_id,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Similarity     float64        `json:"similarity"`
}

// EmbeddingService owns the durable queue and semantic document index.
type EmbeddingService struct {
	db           *sql.DB
	vectorStore  *VectorStore
	embedder     Embedder
	workspaceDir string
	delay        time.Duration
	now          func() time.Time
	wake         chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
	cancel       context.CancelFunc
}

func NewEmbeddingService(db *sql.DB, vectorStore *VectorStore, embedder Embedder) *EmbeddingService {
	return &EmbeddingService{
		db:          db,
		vectorStore: vectorStore,
		embedder:    embedder,
		delay:       defaultEmbeddingDelay,
		now:         func() time.Time { return time.Now().UTC() },
		wake:        make(chan struct{}, 1),
	}
}

func (s *EmbeddingService) SetWorkspaceDir(workspaceDir string) {
	absPath, err := filepath.Abs(workspaceDir)
	if err == nil {
		s.workspaceDir = filepath.Clean(absPath)
	}
}

func (s *EmbeddingService) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		workerCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		go s.run(workerCtx)
	})
}

func (s *EmbeddingService) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
}

func (s *EmbeddingService) EnqueueMessage(ctx context.Context, messageID, agentID, conversationID string) error {
	return s.enqueue(ctx, EmbeddingJob{
		SourceType: "message", SourceKey: messageID, SourceRef: messageID,
		Operation: EmbeddingUpsert, AgentID: agentID,
		Scope: "conversation:" + conversationID, ConversationID: conversationID,
	})
}

func (s *EmbeddingService) EnqueueMemory(ctx context.Context, memoryID, agentID string) error {
	return s.enqueue(ctx, EmbeddingJob{
		SourceType: "memory", SourceKey: memoryID, SourceRef: memoryID,
		Operation: EmbeddingUpsert, AgentID: agentID, Scope: "agent:" + agentID,
	})
}

func (s *EmbeddingService) EnqueueFile(ctx context.Context, absolutePath, agentID, scope string, operation EmbeddingOperation) error {
	absPath, err := filepath.Abs(absolutePath)
	if err != nil {
		return fmt.Errorf("resolving embedding file path: %w", err)
	}
	if scope == "" {
		scope = "shared"
	}
	if agentID == "" && scope == "shared" && s.workspaceDir != "" {
		if rel, relErr := filepath.Rel(s.workspaceDir, absPath); relErr == nil {
			parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
			if len(parts) > 1 && strings.HasPrefix(parts[0], "agent_") {
				agentID = parts[0]
				scope = "agent:" + agentID
			}
		}
	}
	if operation == EmbeddingDelete {
		type sourceMatch struct {
			key, ref, agentID, scope string
		}
		var matches []sourceMatch
		rows, queryErr := s.db.QueryContext(ctx, `SELECT source_key, source_ref, agent_id, scope
			FROM semantic_sources WHERE source_type = 'file'`)
		if queryErr == nil {
			for rows.Next() {
				var match sourceMatch
				if rows.Scan(&match.key, &match.ref, &match.agentID, &match.scope) == nil {
					rel, relErr := filepath.Rel(filepath.Clean(absPath), filepath.Clean(match.ref))
					if relErr == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
						matches = append(matches, match)
					}
				}
			}
			rows.Close()
			for _, match := range matches {
				job := EmbeddingJob{SourceType: "file", SourceKey: match.key, SourceRef: match.ref,
					Operation: EmbeddingDelete, AgentID: match.agentID, Scope: match.scope}
				if err := s.enqueue(ctx, job); err != nil {
					return err
				}
				if err := s.tombstoneSource(ctx, job); err != nil {
					return err
				}
			}
			if len(matches) > 0 {
				return nil
			}
		}
	}
	job := EmbeddingJob{
		SourceType: "file", SourceKey: filepath.Clean(absPath), SourceRef: filepath.Clean(absPath),
		Operation: operation, AgentID: agentID, Scope: scope,
	}
	if err := s.enqueue(ctx, job); err != nil {
		return err
	}
	if operation == EmbeddingDelete {
		return s.tombstoneSource(ctx, job)
	}
	return nil
}

// NotifyFileMutation implements the mutation sink used by native file tools.
func (s *EmbeddingService) NotifyFileMutation(ctx context.Context, absolutePath, agentID string, deleted bool) error {
	absPath, err := filepath.Abs(absolutePath)
	if err != nil {
		return err
	}
	if s.workspaceDir != "" {
		rel, relErr := filepath.Rel(s.workspaceDir, absPath)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	scope := "shared"
	if agentID != "" {
		scope = "agent:" + agentID
	}
	operation := EmbeddingUpsert
	if deleted {
		operation = EmbeddingDelete
	}
	return s.EnqueueFile(ctx, absPath, agentID, scope, operation)
}

func (s *EmbeddingService) enqueue(ctx context.Context, job EmbeddingJob) error {
	if s == nil || s.db == nil {
		return nil
	}
	now := s.now()
	dueAt := now.Add(s.delay)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO embedding_jobs (
			id, source_type, source_key, source_ref, operation, agent_id, scope,
			conversation_id, due_at, status, attempts, generation, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, 1, ?, ?)
		ON CONFLICT(source_type, source_key) DO UPDATE SET
			source_ref = excluded.source_ref,
			operation = excluded.operation,
			agent_id = excluded.agent_id,
			scope = excluded.scope,
			conversation_id = excluded.conversation_id,
			due_at = excluded.due_at,
			status = 'pending',
			attempts = 0,
			generation = embedding_jobs.generation + 1,
			lease_until = NULL,
			last_error = '',
			updated_at = excluded.updated_at
	`, uuid.NewString(), job.SourceType, job.SourceKey, job.SourceRef, string(job.Operation),
		job.AgentID, job.Scope, job.ConversationID, dueAt, now, now)
	if err != nil {
		return fmt.Errorf("enqueueing embedding job: %w", err)
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *EmbeddingService) run(ctx context.Context) {
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

func (s *EmbeddingService) claim(ctx context.Context) (*EmbeddingJob, error) {
	if s.embedder == nil || s.vectorStore == nil {
		return nil, nil
	}
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
		text, err := extractDocumentText(job.SourceRef)
		if err != nil {
			return "", nil, err
		}
		return text, map[string]any{
			"path": filepath.ToSlash(job.SourceRef), "filename": filepath.Base(job.SourceRef),
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

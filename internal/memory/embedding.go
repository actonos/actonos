package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	workspacepkg "github.com/actonos/actonos/internal/workspace"
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
	db             *sql.DB
	vectorStore    *VectorStore
	embedder       Embedder
	workspaceDir   string
	workspaceStore *workspacepkg.Store
	delay          time.Duration
	now            func() time.Time
	wake           chan struct{}
	startOnce      sync.Once
	stopOnce       sync.Once
	cancel         context.CancelFunc
	writeGuard     func() bool
}

func (s *EmbeddingService) SetWriteGuard(fn func() bool) {
	s.writeGuard = fn
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

// SetWorkspaceStore configures the metadata-backed user workspace source.
// Workspace embedding jobs use opaque file IDs and never persist host paths.
func (s *EmbeddingService) SetWorkspaceStore(store *workspacepkg.Store) {
	s.workspaceStore = store
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

func (s *EmbeddingService) EnqueueWorkspaceFile(ctx context.Context, fileID, actorID string, operation EmbeddingOperation) error {
	if fileID == "" {
		return errors.New("workspace file ID is required")
	}
	job := EmbeddingJob{
		SourceType: "workspace_file", SourceKey: fileID, SourceRef: fileID,
		Operation: operation, AgentID: actorID, Scope: "shared",
	}
	if err := s.enqueue(ctx, job); err != nil {
		return err
	}
	if operation == EmbeddingDelete {
		return s.tombstoneSource(ctx, job)
	}
	return nil
}

// NotifyWorkspaceMutation implements tools.WorkspaceMutationSink.
func (s *EmbeddingService) NotifyWorkspaceMutation(ctx context.Context, fileID, agentID string, deleted bool) error {
	operation := EmbeddingUpsert
	if deleted {
		operation = EmbeddingDelete
	}
	return s.EnqueueWorkspaceFile(ctx, fileID, agentID, operation)
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
	if s.writeGuard != nil && s.writeGuard() {
		return errors.New("embedding writes frozen: disk exhausted")
	}
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

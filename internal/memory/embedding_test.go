package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEmbedder struct {
	mu       sync.Mutex
	fail     error
	passages []string
	queries  []string
}

func (f *fakeEmbedder) EmbedQuery(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	f.queries = append(f.queries, texts...)
	return fakeVectors(texts), nil
}

func (f *fakeEmbedder) EmbedPassages(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	f.passages = append(f.passages, texts...)
	return fakeVectors(texts), nil
}

func (f *fakeEmbedder) Health(context.Context) error { return f.fail }

func fakeVectors(texts []string) [][]float32 {
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		vector := make([]float32, EmbeddingDimension)
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "alpha"):
			vector[0] = 1
		case strings.Contains(lower, "beta"):
			vector[1] = 1
		default:
			vector[2] = 1
		}
		vectors[index] = vector
	}
	return vectors
}

func newEmbeddingTestService(t *testing.T) (*DB, *EmbeddingService, *fakeEmbedder) {
	t.Helper()
	root := t.TempDir()
	db, err := Open(filepath.Join(root, "embedding.db"))
	if err != nil {
		t.Fatalf("opening embedding test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	vectorStore, err := NewVectorStore(filepath.Join(root, "vectors"))
	if err != nil {
		t.Fatalf("opening embedding test vector store: %v", err)
	}
	embedder := &fakeEmbedder{}
	service := NewEmbeddingService(db.SQLDB(), vectorStore, embedder)
	service.SetWorkspaceDir(filepath.Join(root, "workspace"))
	return db, service, embedder
}

func insertTestMessage(t *testing.T, db *sql.DB, id, conversationID, agentID, content string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`INSERT INTO messages (id, conversation_id, agent_id, role, content, created_at)
		VALUES (?, ?, ?, 'user', ?, ?)`, id, conversationID, agentID, content, now)
	if err != nil {
		t.Fatalf("inserting test message: %v", err)
	}
}

func claimAndProcess(t *testing.T, service *EmbeddingService) EmbeddingJob {
	t.Helper()
	job, err := service.claim(context.Background())
	if err != nil {
		t.Fatalf("claiming embedding job: %v", err)
	}
	if job == nil {
		t.Fatal("expected a due embedding job")
	}
	if err := service.process(context.Background(), *job); err != nil {
		t.Fatalf("processing embedding job: %v", err)
	}
	service.complete(context.Background(), *job)
	return *job
}

func TestEmbeddingQueueDebouncesAndCoalesces(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	current := base
	service.now = func() time.Time { return current }

	if err := service.EnqueueMessage(context.Background(), "msg-1", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	current = current.Add(30 * time.Second)
	if err := service.EnqueueMessage(context.Background(), "msg-1", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}

	var count, generation, attempts int
	var dueAt time.Time
	var status string
	err := db.SQLDB().QueryRow(`SELECT COUNT(*), generation, attempts, due_at, status
		FROM embedding_jobs WHERE source_type = 'message' AND source_key = 'msg-1'`).
		Scan(&count, &generation, &attempts, &dueAt, &status)
	if err != nil {
		t.Fatalf("reading coalesced job: %v", err)
	}
	if count != 1 || generation != 2 || attempts != 0 || status != "pending" {
		t.Fatalf("unexpected coalesced job: count=%d generation=%d attempts=%d status=%s", count, generation, attempts, status)
	}
	wantDue := base.Add(90 * time.Second)
	if !dueAt.Equal(wantDue) {
		t.Fatalf("debounce due_at = %s, want %s", dueAt, wantDue)
	}
}

func TestEmbeddingClaimRecoversExpiredLease(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if err := service.EnqueueMessage(context.Background(), "msg-lease", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	_, err := db.SQLDB().Exec(`UPDATE embedding_jobs SET status = 'running', due_at = ?, lease_until = ?`,
		now.Add(-time.Minute), now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.claim(context.Background())
	if err != nil || job == nil {
		t.Fatalf("recovering expired lease: job=%v err=%v", job, err)
	}
	var status string
	var leaseUntil time.Time
	if err := db.SQLDB().QueryRow(`SELECT status, lease_until FROM embedding_jobs WHERE id = ?`, job.ID).Scan(&status, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if status != "running" || !leaseUntil.Equal(now.Add(embeddingLeaseDuration)) {
		t.Fatalf("unexpected recovered lease: status=%s lease=%s", status, leaseUntil)
	}
}

func TestEmbeddingFailureBecomesDeadAfterEightAttempts(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if err := service.EnqueueMessage(context.Background(), "msg-dead", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	var job EmbeddingJob
	if err := db.SQLDB().QueryRow(`SELECT id, generation FROM embedding_jobs WHERE source_key = 'msg-dead'`).Scan(&job.ID, &job.Generation); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 8; attempt++ {
		job.Attempts = attempt
		service.fail(context.Background(), job, errors.New("model unavailable"))
	}
	var status string
	var attempts int
	if err := db.SQLDB().QueryRow(`SELECT status, attempts FROM embedding_jobs WHERE id = ?`, job.ID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "dead" || attempts != 8 {
		t.Fatalf("job status=%s attempts=%d, want dead/8", status, attempts)
	}
}

func TestEmbeddingProcessActivatesAndFiltersScopes(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	service.delay = 0
	service.now = func() time.Time { return now }
	insertTestMessage(t, db.SQLDB(), "msg-shared", "conv-shared", "agent-1", "alpha shared reference")
	insertTestMessage(t, db.SQLDB(), "msg-private", "conv-private", "agent-1", "alpha private reference")
	if err := service.enqueue(context.Background(), EmbeddingJob{SourceType: "message", SourceKey: "msg-shared",
		SourceRef: "msg-shared", Operation: EmbeddingUpsert, AgentID: "agent-1", Scope: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueMessage(context.Background(), "msg-private", "agent-1", "conv-private"); err != nil {
		t.Fatal(err)
	}
	claimAndProcess(t, service)
	claimAndProcess(t, service)

	records, err := service.Search(context.Background(), "alpha", []string{"conversation:conv-private"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SourceRef != "msg-private" {
		t.Fatalf("scope filtering returned %+v", records)
	}
	records, err = service.Search(context.Background(), "alpha", []string{"shared", "conversation:conv-private"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Similarity < records[1].Similarity {
		t.Fatalf("merged semantic results are not complete and sorted: %+v", records)
	}
}

func TestEmbeddingReindexKeepsActiveGenerationOnFailure(t *testing.T) {
	db, service, embedder := newEmbeddingTestService(t)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	service.delay = 0
	service.now = func() time.Time { return now }
	insertTestMessage(t, db.SQLDB(), "msg-reindex", "conv-1", "agent-1", "alpha original")
	if err := service.EnqueueMessage(context.Background(), "msg-reindex", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	first := claimAndProcess(t, service)

	_, err := db.SQLDB().Exec(`UPDATE messages SET content = 'beta replacement' WHERE id = 'msg-reindex'`)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err := service.EnqueueMessage(context.Background(), "msg-reindex", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	job, err := service.claim(context.Background())
	if err != nil || job == nil {
		t.Fatalf("claiming reindex job: job=%v err=%v", job, err)
	}
	embedder.fail = errors.New("inference failed")
	if err := service.process(context.Background(), *job); err == nil {
		t.Fatal("expected reindex inference failure")
	}
	var state, contentHash string
	var activeGeneration int
	if err := db.SQLDB().QueryRow(`SELECT state, content_hash, active_generation FROM semantic_sources
		WHERE source_key = 'msg-reindex'`).Scan(&state, &contentHash, &activeGeneration); err != nil {
		t.Fatal(err)
	}
	if state != "active" || activeGeneration != first.Generation || contentHash != hashText("alpha original") {
		t.Fatalf("failed reindex changed active source: state=%s generation=%d hash=%s", state, activeGeneration, contentHash)
	}
}

func TestEmbeddingDeleteTombstonesImmediatelyAndCleansVectors(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	service.delay = 0
	service.now = func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }
	path := filepath.Join(service.workspaceDir, "notes.txt")
	if err := osWriteFile(path, "alpha file content"); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueFile(context.Background(), path, "", "shared", EmbeddingUpsert); err != nil {
		t.Fatal(err)
	}
	claimAndProcess(t, service)
	if err := service.EnqueueFile(context.Background(), path, "", "shared", EmbeddingDelete); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.SQLDB().QueryRow(`SELECT state FROM semantic_sources WHERE source_key = ?`, filepath.Clean(path)).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "deleted" {
		t.Fatalf("source state=%s immediately after delete enqueue", state)
	}
	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.IndexedSources != 0 || status.ActiveChunks != 0 {
		t.Fatalf("tombstoned source remains in active status counts: %+v", status)
	}
	claimAndProcess(t, service)
	records, err := service.Search(context.Background(), "alpha", []string{"shared"}, 5)
	if err != nil || len(records) != 0 {
		t.Fatalf("deleted source remains searchable: records=%+v err=%v", records, err)
	}
}

func TestEmbeddingDirectoryDeleteMatchesUnicodePaths(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	service.delay = 0
	service.now = func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }
	directory := filepath.Join(service.workspaceDir, "tài liệu")
	path := filepath.Join(directory, "ghi chú.txt")
	if err := osWriteFile(path, "alpha unicode path"); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueFile(context.Background(), path, "", "shared", EmbeddingUpsert); err != nil {
		t.Fatal(err)
	}
	claimAndProcess(t, service)
	if err := service.EnqueueFile(context.Background(), directory, "", "shared", EmbeddingDelete); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.SQLDB().QueryRow(`SELECT state FROM semantic_sources WHERE source_ref = ?`, filepath.Clean(path)).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "deleted" {
		t.Fatalf("unicode descendant state=%s, want deleted", state)
	}
}

func TestChunkTextBoundsAndOverlap(t *testing.T) {
	content := strings.Repeat("x", 1100)
	chunks := chunkText(content)
	if len(chunks) != 2 {
		t.Fatalf("chunk count=%d, want 2", len(chunks))
	}
	if len([]rune(chunks[0])) != 800 || len([]rune(chunks[1])) != 420 {
		t.Fatalf("unexpected chunk lengths: %d, %d", len([]rune(chunks[0])), len([]rune(chunks[1])))
	}
	if chunks[0][680:] != chunks[1][:120] {
		t.Fatal("expected 120-rune overlap between chunks")
	}
}

func TestHTTPEmbedderBatchesRequests(t *testing.T) {
	var mu sync.Mutex
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var request struct {
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		batchSizes = append(batchSizes, len(request.Texts))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": fakeVectors(request.Texts)})
	}))
	defer server.Close()

	texts := make([]string, 70)
	for index := range texts {
		texts[index] = fmt.Sprintf("alpha %d", index)
	}
	vectors, err := NewHTTPEmbedder(server.URL).EmbedPassages(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != len(texts) || fmt.Sprint(batchSizes) != "[32 32 6]" {
		t.Fatalf("vectors=%d batches=%v", len(vectors), batchSizes)
	}
	for _, vector := range vectors {
		var norm float64
		for _, value := range vector {
			norm += float64(value * value)
		}
		if math.Abs(math.Sqrt(norm)-1) > 1e-6 {
			t.Fatalf("fake vector is not normalized")
		}
	}
}

func osWriteFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func TestEmbeddingDeletedMessageCleansUp(t *testing.T) {
	_, service, _ := newEmbeddingTestService(t)
	service.delay = 0
	// Enqueue a message that doesn't exist in the database (or was deleted)
	if err := service.EnqueueMessage(context.Background(), "msg-nonexistent", "agent-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	job := claimAndProcess(t, service)
	if job.SourceKey != "msg-nonexistent" {
		t.Fatalf("unexpected processed job: %+v", job)
	}
}

func TestChunkTextEdgeCases(t *testing.T) {
	if chunks := chunkText(""); len(chunks) != 0 {
		t.Fatalf("chunkText empty string returned %d chunks", len(chunks))
	}
	if chunks := chunkText("   \n\n\t  \n  "); len(chunks) != 0 {
		t.Fatalf("chunkText whitespace only returned %d chunks", len(chunks))
	}
	// Very long single line without any newlines
	longText := strings.Repeat("a", 2000)
	chunks := chunkText(longText)
	if len(chunks) < 2 {
		t.Fatalf("chunkText long single line produced %d chunks, want >= 2", len(chunks))
	}
	for i, chunk := range chunks {
		if len([]rune(chunk)) > 800 {
			t.Fatalf("chunk %d exceeded target runes: %d", i, len([]rune(chunk)))
		}
	}
}

func TestIgnoredEmbeddingPath(t *testing.T) {
	cases := []struct {
		path   string
		ignore bool
	}{
		{"workspace/docs/readme.md", false},
		{"workspace/.git/config", true},
		{"workspace/.hidden/file.txt", true},
		{"workspace/node_modules/pkg/index.js", true},
		{"workspace/sub/vectors/index.bin", true},
		{"workspace/sub/models/model.onnx", true},
		{"workspace/file.tmp", true},
		{"workspace/file.swp", true},
		{"workspace/file.part", true},
		{"workspace/file.txt~", true},
	}
	for _, tc := range cases {
		if got := ignoredEmbeddingPath(tc.path); got != tc.ignore {
			t.Errorf("ignoredEmbeddingPath(%q) = %v, want %v", tc.path, got, tc.ignore)
		}
	}
}

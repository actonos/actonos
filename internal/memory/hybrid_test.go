package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestHybridEngine_StoreAndRetrieve(t *testing.T) {
	tempDir := t.TempDir()
	db, err := Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	defer db.Close()

	vs, err := NewVectorStore(filepath.Join(tempDir, "vectors"))
	if err != nil {
		t.Fatalf("opening vector store: %v", err)
	}

	engine := NewHybridEngine(db, vs, nil)
	ctx := context.Background()

	agentID := "agent_architect_01"

	// Mock vector generator (dim 4)
	vec1 := []float32{0.9, 0.1, 0.0, 0.0}
	vec2 := []float32{0.0, 0.8, 0.6, 0.0}

	// 1. Store memories
	doc1, err := engine.StoreMemory(
		ctx, agentID, LayerEpisodic,
		"ActonOS runs as a single static binary with CGO_ENABLED=0",
		vec1,
		map[string]any{"source": "docs"},
		1.5,
	)
	if err != nil {
		t.Fatalf("storing memory 1: %v", err)
	}

	doc2, err := engine.StoreMemory(
		ctx, agentID, LayerEpisodic,
		"Docker container deployment on Alpine Linux with minimal image",
		vec2,
		map[string]any{"source": "deploy"},
		1.0,
	)
	if err != nil {
		t.Fatalf("storing memory 2: %v", err)
	}

	if doc1.ID == "" || doc2.ID == "" {
		t.Fatalf("expected valid document IDs")
	}

	// 2. Search by Lexical Keyword (FTS5)
	resultsFTS, err := engine.Search(ctx, agentID, LayerEpisodic, "binary", nil, 5)
	if err != nil {
		t.Fatalf("FTS search failed: %v", err)
	}
	if len(resultsFTS) == 0 {
		t.Fatalf("expected at least 1 FTS match for 'binary'")
	}
	if resultsFTS[0].ID != doc1.ID {
		t.Fatalf("expected doc1 as top match for 'binary', got %s", resultsFTS[0].ID)
	}

	// 3. Search by Vector Embedding
	queryVec := []float32{0.0, 0.85, 0.55, 0.0}
	resultsVec, err := engine.Search(ctx, agentID, LayerEpisodic, "", queryVec, 5)
	if err != nil {
		t.Fatalf("vector search failed: %v", err)
	}
	if len(resultsVec) == 0 {
		t.Fatalf("expected at least 1 vector match")
	}
	if resultsVec[0].ID != doc2.ID {
		t.Fatalf("expected doc2 as top vector match, got %s", resultsVec[0].ID)
	}

	// 4. Hybrid combined search
	resultsHybrid, err := engine.Search(ctx, agentID, LayerEpisodic, "static binary", vec1, 5)
	if err != nil {
		t.Fatalf("hybrid search failed: %v", err)
	}
	if len(resultsHybrid) == 0 {
		t.Fatalf("expected hybrid search results")
	}
	if resultsHybrid[0].ID != doc1.ID {
		t.Fatalf("expected doc1 as top hybrid match, got %s", resultsHybrid[0].ID)
	}
	if resultsHybrid[0].Score <= 0 {
		t.Fatalf("expected positive score, got %f", resultsHybrid[0].Score)
	}

	// Wait for async touch routine
	time.Sleep(50 * time.Millisecond)
}

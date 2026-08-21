package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryLayer identifies the tier of memory.
type MemoryLayer string

const (
	LayerWorking     MemoryLayer = "working"
	LayerUserProfile MemoryLayer = "user_profile"
	LayerProcedural  MemoryLayer = "procedural"
	LayerEpisodic    MemoryLayer = "episodic"
)

// MemoryRecord represents a single stored memory fragment.
type MemoryRecord struct {
	ID               string         `json:"id"`
	AgentID          string         `json:"agent_id"`
	Layer            MemoryLayer    `json:"layer"`
	Content          string         `json:"content"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	ImportanceWeight float64        `json:"importance_weight"`
	LastAccessedAt   time.Time      `json:"last_accessed_at"`
	AccessCount      int            `json:"access_count"`
	CreatedAt        time.Time      `json:"created_at"`
	Score            float64        `json:"score,omitempty"`
}

// HybridEngine combines SQLite FTS5 lexical search, Chromem-go vector search, and Ebbinghaus decay.
type HybridEngine struct {
	db               *DB
	vectorStore      *VectorStore
	embeddingService *EmbeddingService
	decayCfg         DecayConfig
}

func (h *HybridEngine) SetEmbeddingService(service *EmbeddingService) {
	h.embeddingService = service
}

// NewHybridEngine creates a new hybrid memory engine.
func NewHybridEngine(db *DB, vectorStore *VectorStore, decayCfg *DecayConfig) *HybridEngine {
	cfg := DefaultDecayConfig()
	if decayCfg != nil {
		cfg = *decayCfg
	}
	return &HybridEngine{
		db:          db,
		vectorStore: vectorStore,
		decayCfg:    cfg,
	}
}

// StoreMemory stores a memory fragment into both relational/FTS5 and vector indices.
func (h *HybridEngine) StoreMemory(
	ctx context.Context,
	agentID string,
	layer MemoryLayer,
	content string,
	embedding []float32,
	metadata map[string]any,
	importanceWeight float64,
) (*MemoryRecord, error) {
	if importanceWeight <= 0 {
		importanceWeight = 1.0
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshalling metadata: %w", err)
	}

	// 1. Insert into SQLite memories table
	query := `
		INSERT INTO memories (
			id, agent_id, layer, content, metadata_json,
			importance_weight, last_accessed_at, access_count, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = h.db.db.ExecContext(
		ctx, query,
		id, agentID, string(layer), content, string(metaJSON),
		importanceWeight, now, 1, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting memory to sqlite: %w", err)
	}

	// 2. Index in FTS5
	if err := h.db.IndexMemoryFTS(ctx, id, agentID, string(layer), content); err != nil {
		return nil, fmt.Errorf("indexing memory fts5: %w", err)
	}

	// 3. Index in Chromem-go VectorStore if embedding is provided
	if len(embedding) > 0 && h.vectorStore != nil {
		strMeta := make(map[string]string)
		strMeta["agent_id"] = agentID
		strMeta["layer"] = string(layer)
		for k, v := range metadata {
			strMeta[k] = fmt.Sprintf("%v", v)
		}

		colName := fmt.Sprintf("%s_%s", agentID, layer)
		if err := h.vectorStore.IndexDocument(ctx, colName, id, content, embedding, strMeta); err != nil {
			return nil, fmt.Errorf("indexing document in vector store: %w", err)
		}
	}

	record := &MemoryRecord{
		ID:               id,
		AgentID:          agentID,
		Layer:            layer,
		Content:          content,
		Metadata:         metadata,
		ImportanceWeight: importanceWeight,
		LastAccessedAt:   now,
		AccessCount:      1,
		CreatedAt:        now,
	}
	if layer == LayerEpisodic && len(embedding) == 0 && h.embeddingService != nil {
		_ = h.embeddingService.EnqueueMemory(context.Background(), id, agentID)
	}
	return record, nil
}

// sigmoid normalizes an arbitrary value to (0, 1).
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// Search retrieves memories by combining FTS5 lexical scores, vector similarities, and Ebbinghaus decay.
func (h *HybridEngine) Search(
	ctx context.Context,
	agentID string,
	layer MemoryLayer,
	queryStr string,
	queryVector []float32,
	limit int,
) ([]MemoryRecord, error) {
	if limit <= 0 {
		limit = 10
	}

	type candidateScore struct {
		ftsNormScore float64
		vecSim       float64
	}

	candidateMap := make(map[string]*candidateScore)
	var mu sync.Mutex

	var wg sync.WaitGroup

	// Branch 1: FTS5 Lexical Search
	if queryStr != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ftsResults, err := h.db.SearchFTS(ctx, agentID, string(layer), queryStr, limit*2)
			if err == nil {
				mu.Lock()
				for _, r := range ftsResults {
					if _, exists := candidateMap[r.ID]; !exists {
						candidateMap[r.ID] = &candidateScore{}
					}
					candidateMap[r.ID].ftsNormScore = r.NormScore
				}
				mu.Unlock()
			}
		}()
	}

	// Branch 2: Dense Vector Search
	if len(queryVector) > 0 && h.vectorStore != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			colName := fmt.Sprintf("%s_%s", agentID, layer)
			vecResults, err := h.vectorStore.SearchByEmbedding(ctx, colName, queryVector, limit*2)
			if err == nil {
				mu.Lock()
				for _, r := range vecResults {
					if _, exists := candidateMap[r.ID]; !exists {
						candidateMap[r.ID] = &candidateScore{}
					}
					candidateMap[r.ID].vecSim = float64(r.Similarity)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if len(candidateMap) == 0 {
		return nil, nil
	}

	// Fetch full records from SQLite
	ids := make([]string, 0, len(candidateMap))
	for id := range candidateMap {
		ids = append(ids, id)
	}

	records, err := h.getMemoriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var scoredRecords []MemoryRecord

	for _, rec := range records {
		cand := candidateMap[rec.ID]
		if cand == nil {
			continue
		}

		// Combined lexical + semantic similarity (Sigmoid fusion)
		lexicalComponent := cand.ftsNormScore
		semanticComponent := cand.vecSim

		var combinedSim float64
		if lexicalComponent > 0 && semanticComponent > 0 {
			// Sigmoid fusion when both match
			combinedSim = 0.5*lexicalComponent + 0.5*semanticComponent
		} else if semanticComponent > 0 {
			combinedSim = semanticComponent
		} else {
			combinedSim = lexicalComponent
		}

		elapsed := now.Sub(rec.LastAccessedAt)
		finalScore := CalculateRetrievalScore(
			elapsed,
			rec.ImportanceWeight,
			rec.AccessCount,
			combinedSim,
			h.decayCfg,
		)

		rec.Score = finalScore
		scoredRecords = append(scoredRecords, rec)
	}

	// Sort by final score descending
	sort.Slice(scoredRecords, func(i, j int) bool {
		return scoredRecords[i].Score > scoredRecords[j].Score
	})

	if len(scoredRecords) > limit {
		scoredRecords = scoredRecords[:limit]
	}

	// Touch top returned memories asynchronously to reinforce their stability
	go func(top []MemoryRecord) {
		for _, m := range top {
			_ = h.TouchMemory(context.Background(), m.ID)
		}
	}(scoredRecords)

	return scoredRecords, nil
}

// TouchMemory updates last_accessed_at to current time and increments access_count.
func (h *HybridEngine) TouchMemory(ctx context.Context, id string) error {
	query := `
		UPDATE memories
		SET last_accessed_at = ?, access_count = access_count + 1
		WHERE id = ?
	`
	_, err := h.db.db.ExecContext(ctx, query, time.Now().UTC(), id)
	return err
}

func (h *HybridEngine) getMemoriesByIDs(ctx context.Context, ids []string) ([]MemoryRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var placeholders string
	var args []any
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT id, agent_id, layer, content, metadata_json,
		       importance_weight, last_accessed_at, access_count, created_at
		FROM memories
		WHERE id IN (%s)
	`, placeholders)

	rows, err := h.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetching memories: %w", err)
	}
	defer rows.Close()

	var records []MemoryRecord
	for rows.Next() {
		var rec MemoryRecord
		var metaJSON sql.NullString
		var layerStr string

		if err := rows.Scan(
			&rec.ID, &rec.AgentID, &layerStr, &rec.Content, &metaJSON,
			&rec.ImportanceWeight, &rec.LastAccessedAt, &rec.AccessCount, &rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning memory: %w", err)
		}

		rec.Layer = MemoryLayer(layerStr)
		if metaJSON.Valid && metaJSON.String != "" {
			_ = json.Unmarshal([]byte(metaJSON.String), &rec.Metadata)
		}

		records = append(records, rec)
	}

	return records, rows.Err()
}

// DB returns the underlying relational database pointer.
func (h *HybridEngine) DB() *DB {
	return h.db
}

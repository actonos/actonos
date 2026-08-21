package memory

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/philippgille/chromem-go"
)

// VectorResult represents a semantic search match.
type VectorResult struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Metadata   map[string]string `json:"metadata"`
	Similarity float32           `json:"similarity"`
}

// VectorDocument is a pre-embedded document ready for batch indexing.
type VectorDocument struct {
	ID        string
	Content   string
	Embedding []float32
	Metadata  map[string]string
}

// VectorStore wraps Chromem-go for in-memory or persistent vector search.
type VectorStore struct {
	mu  sync.RWMutex
	db  *chromem.DB
	dir string
}

// NewVectorStore initializes Chromem-go vector storage.
func NewVectorStore(storageDir string) (*VectorStore, error) {
	var db *chromem.DB
	var err error

	if storageDir != "" {
		if err := os.MkdirAll(storageDir, 0755); err != nil {
			return nil, fmt.Errorf("creating vector store directory: %w", err)
		}
		db, err = chromem.NewPersistentDB(storageDir, false)
		if err != nil {
			return nil, fmt.Errorf("initializing persistent chromem db: %w", err)
		}
	} else {
		db = chromem.NewDB()
	}

	return &VectorStore{
		db:  db,
		dir: storageDir,
	}, nil
}

// getOrCreateCollection gets or creates a named collection in Chromem-go.
func (vs *VectorStore) getOrCreateCollection(name string) (*chromem.Collection, error) {
	// Custom embedding function nil since we pass pre-computed vectors or rely on query embedding
	col, err := vs.db.GetOrCreateCollection(name, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get or create collection %s: %w", name, err)
	}
	return col, nil
}

// IndexDocument inserts or updates a document vector in a specific collection.
func (vs *VectorStore) IndexDocument(
	ctx context.Context,
	collectionName string,
	docID string,
	content string,
	embedding []float32,
	metadata map[string]string,
) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	col, err := vs.getOrCreateCollection(collectionName)
	if err != nil {
		return err
	}

	doc := chromem.Document{
		ID:        docID,
		Content:   content,
		Embedding: embedding,
		Metadata:  metadata,
	}

	return col.AddDocuments(ctx, []chromem.Document{doc}, 1)
}

// IndexDocuments inserts or replaces a batch of pre-embedded documents.
func (vs *VectorStore) IndexDocuments(ctx context.Context, collectionName string, documents []VectorDocument) error {
	if len(documents) == 0 {
		return nil
	}

	vs.mu.Lock()
	defer vs.mu.Unlock()

	col, err := vs.getOrCreateCollection(collectionName)
	if err != nil {
		return err
	}

	docs := make([]chromem.Document, 0, len(documents))
	for _, document := range documents {
		docs = append(docs, chromem.Document{
			ID:        document.ID,
			Content:   document.Content,
			Embedding: document.Embedding,
			Metadata:  document.Metadata,
		})
	}

	return col.AddDocuments(ctx, docs, 1)
}

// SearchByEmbedding queries the vector store using a pre-calculated query vector.
func (vs *VectorStore) SearchByEmbedding(
	ctx context.Context,
	collectionName string,
	queryVector []float32,
	limit int,
) ([]VectorResult, error) {
	return vs.SearchByEmbeddingFiltered(ctx, collectionName, queryVector, limit, nil)
}

// SearchByEmbeddingFiltered queries the vector store and applies exact metadata filters before ranking.
func (vs *VectorStore) SearchByEmbeddingFiltered(
	ctx context.Context,
	collectionName string,
	queryVector []float32,
	limit int,
	where map[string]string,
) ([]VectorResult, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	col, err := vs.getOrCreateCollection(collectionName)
	if err != nil {
		return nil, err
	}

	docCount := col.Count()
	if docCount == 0 {
		return nil, nil
	}

	if limit <= 0 || limit > docCount {
		limit = docCount
	}

	results, err := col.QueryEmbedding(ctx, queryVector, limit, where, nil)
	if err != nil {
		return nil, fmt.Errorf("querying chromem: %w", err)
	}

	out := make([]VectorResult, len(results))
	for i, res := range results {
		out[i] = VectorResult{
			ID:         res.ID,
			Content:    res.Content,
			Metadata:   res.Metadata,
			Similarity: res.Similarity,
		}
	}
	return out, nil
}

// DeleteDocument removes a document from the collection.
func (vs *VectorStore) DeleteDocument(ctx context.Context, collectionName, docID string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	col, err := vs.getOrCreateCollection(collectionName)
	if err != nil {
		return err
	}

	return col.Delete(ctx, nil, nil, docID)
}

// DeleteDocuments removes multiple document IDs from a collection.
func (vs *VectorStore) DeleteDocuments(ctx context.Context, collectionName string, docIDs []string) error {
	if len(docIDs) == 0 {
		return nil
	}

	vs.mu.Lock()
	defer vs.mu.Unlock()

	col, err := vs.getOrCreateCollection(collectionName)
	if err != nil {
		return err
	}

	return col.Delete(ctx, nil, nil, docIDs...)
}

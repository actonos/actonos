package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
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
	slog.Info("dispatching embedding batch to embeddingd", "kind", kind, "batch_size", len(texts))
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

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

func TestHealthIsNotHealthyWithoutRealLLM(t *testing.T) {
	srv := NewServer(Config{
		LLMRouter: llm.NewModelCascadeRouter(),
		DataDir:   t.TempDir(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	var resp struct {
		Data struct {
			Status     string            `json:"status"`
			Components map[string]string `json:"components"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status == "healthy" {
		t.Fatalf("expected degraded/unhealthy without a real LLM, got %+v", resp.Data)
	}
	if resp.Data.Components["llm"] == "healthy" {
		t.Fatalf("llm component should not be healthy: %+v", resp.Data.Components)
	}

	ready := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	rw := httptest.NewRecorder()
	srv.Router().ServeHTTP(rw, ready)
	if rw.Code == http.StatusOK {
		t.Fatal("ready must not be 200 when health is degraded")
	}
}

func TestHealthDegradesWhenEmbeddingHelperIsDown(t *testing.T) {
	dir := t.TempDir()
	db, err := memory.Open(filepath.Join(dir, "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vectors, err := memory.NewVectorStore(filepath.Join(dir, "vectors"))
	if err != nil {
		t.Fatal(err)
	}
	svc := memory.NewEmbeddingService(db.SQLDB(), vectors, memory.NewHTTPEmbedder("http://127.0.0.1:1"))
	router := llm.NewModelCascadeRouter()
	router.RegisterProvider("test-model", llm.NewMockProvider("test-model", "ok"))
	srv := NewServer(Config{
		LLMRouter: router,
		Embedding: svc,
		DataDir:   dir,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	var resp struct {
		Data struct {
			Status     string            `json:"status"`
			Components map[string]string `json:"components"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Components["embedding"] == "healthy" {
		t.Fatalf("embedding must degrade when helper Health() fails, got %+v", resp.Data)
	}
	if resp.Data.Status == "healthy" {
		t.Fatalf("overall health must not be healthy when embedding helper is down, got %q", resp.Data.Status)
	}
}

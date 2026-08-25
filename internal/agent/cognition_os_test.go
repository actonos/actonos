package agent

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
)

type capturingEmbedder struct {
	queries []string
}

func (c *capturingEmbedder) EmbedQuery(_ context.Context, texts []string) ([][]float32, error) {
	c.queries = append(c.queries, texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, memory.EmbeddingDimension)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}
func (c *capturingEmbedder) EmbedPassages(context.Context, []string) ([][]float32, error) {
	return nil, nil
}
func (c *capturingEmbedder) Health(context.Context) error { return nil }

func TestCognitivePromptInvokesQueryEmbedding(t *testing.T) {
	db, eventBus := setupTestDB(t)
	vectorStore, err := memory.NewVectorStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hybrid := memory.NewHybridEngine(db, vectorStore, nil)
	embedder := &capturingEmbedder{}
	svc := memory.NewEmbeddingService(db.SQLDB(), vectorStore, embedder)
	hybrid.SetEmbeddingService(svc)
	_, _ = hybrid.StoreMemory(context.Background(), "agent_a", memory.LayerEpisodic, "the user likes dark mode", nil, nil, 1)

	prompt, _ := BuildCognitiveSystemPrompt(context.Background(), "agent_a", &AgentManifest{Name: "A", AgentID: "agent_a"}, t.TempDir(), t.TempDir(), nil, hybrid, svc, "what theme does the user like")
	if len(embedder.queries) == 0 {
		t.Fatal("expected query embedding to be invoked from the cognitive-prompt path")
	}
	_ = prompt
	_ = eventBus
}

func TestReflectionStoresHeartbeatMission(t *testing.T) {
	db, eventBus := setupTestDB(t)
	vectorStore, err := memory.NewVectorStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hybrid := memory.NewHybridEngine(db, vectorStore, nil)
	router := llm.NewModelCascadeRouter()
	provider := llm.NewMockProvider("reflect-model", `{"preference_key":"","preference_value":"","episodic_memory":"mission learned to retry the deploy"}`)
	router.RegisterProvider("reflect-model", provider)
	engine := NewReflectionEngine(nil, hybrid, router, eventBus)
	engine.ReflectOnConversation(context.Background(), "agent_system_core",
		"<autonomous_mission_cycle> deploy the service",
		"advanced the deploy to 40%")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mems, _ := hybrid.Search(context.Background(), "agent_system_core", memory.LayerEpisodic, "deploy", nil, 5)
		if len(mems) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected redacted mission reflection to be stored")
}

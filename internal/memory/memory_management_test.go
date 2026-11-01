package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func newManagementEngine(t *testing.T) *HybridEngine {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	vs, err := NewVectorStore(filepath.Join(dir, "vectors"))
	if err != nil {
		t.Fatalf("opening vector store: %v", err)
	}
	return NewHybridEngine(db, vs, nil)
}

func findRecord(t *testing.T, recs []MemoryRecord, id string) MemoryRecord {
	t.Helper()
	for _, r := range recs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("memory %s not found in %d records", id, len(recs))
	return MemoryRecord{}
}

func TestPinAndUnpinMemory(t *testing.T) {
	engine := newManagementEngine(t)
	ctx := context.Background()
	rec, err := engine.StoreMemory(ctx, "agent-1", LayerEpisodic, "remember the deploy window", []float32{1, 0, 0, 0}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.PinMemory(ctx, rec.ID, true); err != nil {
		t.Fatalf("PinMemory: %v", err)
	}
	recs, err := engine.ListMemories(ctx, "agent-1", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	got := findRecord(t, recs, rec.ID)
	if !got.Pinned || !got.UserPinned {
		t.Fatalf("expected user-pinned memory, got pinned=%v user_pinned=%v", got.Pinned, got.UserPinned)
	}

	if err := engine.PinMemory(ctx, rec.ID, false); err != nil {
		t.Fatal(err)
	}
	recs, _ = engine.ListMemories(ctx, "agent-1", "", 0)
	got = findRecord(t, recs, rec.ID)
	if !got.Pinned || got.UserPinned {
		t.Fatalf("expected agent-pinned (not user-pinned), got pinned=%v user_pinned=%v", got.Pinned, got.UserPinned)
	}

	if err := engine.UnpinMemory(ctx, rec.ID); err != nil {
		t.Fatalf("UnpinMemory: %v", err)
	}
	recs, _ = engine.ListMemories(ctx, "agent-1", "", 0)
	got = findRecord(t, recs, rec.ID)
	if got.Pinned || got.UserPinned {
		t.Fatalf("expected unpinned memory, got pinned=%v user_pinned=%v", got.Pinned, got.UserPinned)
	}
}

func TestSetMemoryImportanceProtectsHighTiers(t *testing.T) {
	engine := newManagementEngine(t)
	ctx := context.Background()
	rec, err := engine.StoreMemory(ctx, "agent-1", LayerEpisodic, "API contract changed", []float32{0, 1, 0, 0}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.SetMemoryImportance(ctx, rec.ID, ImportanceLow); err != nil {
		t.Fatalf("SetMemoryImportance(low): %v", err)
	}
	recs, _ := engine.ListMemories(ctx, "agent-1", "", 0)
	got := findRecord(t, recs, rec.ID)
	if got.Importance != string(ImportanceLow) || got.Pinned {
		t.Fatalf("low tier must not pin: importance=%q pinned=%v", got.Importance, got.Pinned)
	}

	if err := engine.SetMemoryImportance(ctx, rec.ID, ImportanceCritical); err != nil {
		t.Fatalf("SetMemoryImportance(critical): %v", err)
	}
	recs, _ = engine.ListMemories(ctx, "agent-1", "", 0)
	got = findRecord(t, recs, rec.ID)
	if got.Importance != string(ImportanceCritical) || !got.Pinned {
		t.Fatalf("critical tier must pin: importance=%q pinned=%v", got.Importance, got.Pinned)
	}

	// Downgrading never silently unpins a protected memory.
	if err := engine.SetMemoryImportance(ctx, rec.ID, ImportanceNormal); err != nil {
		t.Fatal(err)
	}
	recs, _ = engine.ListMemories(ctx, "agent-1", "", 0)
	if got = findRecord(t, recs, rec.ID); !got.Pinned {
		t.Fatal("changing tier must not clear an existing pin")
	}
}

func TestListMemoriesFiltersByAgentLayerAndLimit(t *testing.T) {
	engine := newManagementEngine(t)
	ctx := context.Background()
	store := func(agent string, layer MemoryLayer, content string) {
		t.Helper()
		if _, err := engine.StoreMemory(ctx, agent, layer, content, []float32{0.5, 0.5, 0, 0}, nil, 1); err != nil {
			t.Fatal(err)
		}
	}
	store("agent-1", LayerEpisodic, "first episodic note")
	store("agent-1", LayerEpisodic, "second episodic note")
	store("agent-1", LayerProcedural, "how to deploy")
	store("agent-2", LayerEpisodic, "someone else's note")

	all, err := engine.ListMemories(ctx, "agent-1", "", 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("expected 3 memories for agent-1, got %d (err=%v)", len(all), err)
	}
	for _, r := range all {
		if r.AgentID != "agent-1" {
			t.Fatalf("leaked memory from %q", r.AgentID)
		}
	}

	episodic, _ := engine.ListMemories(ctx, "agent-1", LayerEpisodic, 50)
	if len(episodic) != 2 {
		t.Fatalf("expected 2 episodic memories, got %d", len(episodic))
	}

	limited, _ := engine.ListMemories(ctx, "agent-1", "", 1)
	if len(limited) != 1 {
		t.Fatalf("limit not honoured: got %d", len(limited))
	}

	none, err := engine.ListMemories(ctx, "unknown-agent", "", 0)
	if err != nil || len(none) != 0 {
		t.Fatalf("expected no memories, got %d (err=%v)", len(none), err)
	}
}

func TestImportanceTierMultiplier(t *testing.T) {
	cases := map[ImportanceTier]float64{
		ImportanceCritical:       3.0,
		ImportanceUserPreference: 2.5,
		ImportanceHigh:           1.5,
		ImportanceNormal:         1.0,
		ImportanceLow:            0.5,
		ImportanceTier("bogus"):  1.0,
		ImportanceTier(""):       1.0,
	}
	for tier, want := range cases {
		if got := ImportanceTierMultiplier(tier); got != want {
			t.Errorf("ImportanceTierMultiplier(%q) = %v, want %v", tier, got, want)
		}
	}
	if !IsProtectedTier(ImportanceCritical) || !IsProtectedTier(ImportanceUserPreference) || IsProtectedTier(ImportanceHigh) {
		t.Fatal("IsProtectedTier misclassifies tiers")
	}
}

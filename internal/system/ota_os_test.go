package system

import (
	"path/filepath"
	"testing"
)

func TestOTAPersistAndRollback(t *testing.T) {
	dir := t.TempDir()
	eng := NewOTAEngine(dir)
	eng.activeBuild = filepath.Join(dir, "releases", "v2", "actond")
	eng.previousBuild = filepath.Join(dir, "releases", "v1", "actond")
	if err := eng.persistState(); err != nil {
		t.Fatal(err)
	}
	eng2 := NewOTAEngine(dir)
	active, previous := eng2.State()
	if active != eng.activeBuild || previous != eng.previousBuild {
		t.Fatalf("persisted OTA state mismatch: active=%q previous=%q", active, previous)
	}
	if err := eng2.Rollback(); err != nil {
		t.Fatal(err)
	}
	eng3 := NewOTAEngine(dir)
	active, _ = eng3.State()
	if active != previous {
		t.Fatalf("rollback state did not restore previous binary, got %q want %q", active, previous)
	}
}

func TestWritesFrozenOverride(t *testing.T) {
	on := true
	ForceWriteFreeze(&on)
	t.Cleanup(func() { ForceWriteFreeze(nil) })
	if !WritesFrozen(t.TempDir()) {
		t.Fatal("expected forced freeze")
	}
}

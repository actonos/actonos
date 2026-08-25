package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackagePercentsRespectsFloors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.out")
	// One statement uncovered in memory would be 0%; two covered statements in agent = 100%.
	content := "mode: set\n" +
		"github.com/actonos/actonos/internal/memory/db.go:1.1,2.2 2 1\n" +
		"github.com/actonos/actonos/internal/memory/db.go:3.1,4.2 8 0\n" +
		"github.com/actonos/actonos/internal/agent/engine.go:1.1,2.2 10 1\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	pct, err := packagePercents(path)
	if err != nil {
		t.Fatal(err)
	}
	if pct["github.com/actonos/actonos/internal/memory"] >= 80 {
		t.Fatalf("memory fixture must sit below 80%%, got %.1f", pct["github.com/actonos/actonos/internal/memory"])
	}
	if pct["github.com/actonos/actonos/internal/agent"] < 80 {
		t.Fatalf("agent fixture should be fully covered, got %.1f", pct["github.com/actonos/actonos/internal/agent"])
	}
}

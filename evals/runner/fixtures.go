package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeFixtures places a task's input files in its workspace. Paths must stay
// inside the workspace so a task definition can never write elsewhere.
func writeFixtures(workspace string, fixtures map[string]string) error {
	root := filepath.Clean(workspace)
	for name, content := range fixtures {
		if filepath.IsAbs(name) {
			return fmt.Errorf("fixture %q must be a relative path", name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("fixture %q escapes the task workspace", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating fixture directory for %q: %w", name, err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing fixture %q: %w", name, err)
		}
	}
	return nil
}

// excerpt returns at most limit runes of s, marking truncation.
func excerpt(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

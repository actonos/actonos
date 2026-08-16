package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	sampleMD := `---
name: sample_skill
description: A test skill for verifying SKILL.md parsing
category: developer
parameters:
  type: object
  properties:
    target:
      type: string
  required:
    - target
entrypoint: run.py
---

# Instruction Section
This is the markdown instructions body.
`

	manifest, body, err := parseSkillMD([]byte(sampleMD))
	if err != nil {
		t.Fatalf("failed to parse SKILL.md: %v", err)
	}

	if manifest.Name != "sample_skill" {
		t.Errorf("expected name 'sample_skill', got %s", manifest.Name)
	}
	if manifest.Description != "A test skill for verifying SKILL.md parsing" {
		t.Errorf("unexpected description: %s", manifest.Description)
	}
	if manifest.Category != "developer" {
		t.Errorf("expected category 'developer', got %s", manifest.Category)
	}
	if manifest.Entrypoint != "run.py" {
		t.Errorf("expected entrypoint 'run.py', got %s", manifest.Entrypoint)
	}
	if body != "# Instruction Section\nThis is the markdown instructions body." {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHubManager_InstallAndUninstall(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "actonos-skills-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hm := NewHubManager(tempDir)
	catalog := hm.ListCatalog()
	if len(catalog) == 0 {
		t.Fatal("expected curated catalog to have items")
	}

	targetID := catalog[0].ID

	// 1. Install
	if err := hm.InstallSkill(targetID); err != nil {
		t.Fatalf("install skill failed: %v", err)
	}

	installedSkillMD := filepath.Join(tempDir, targetID, "SKILL.md")
	if _, err := os.Stat(installedSkillMD); err != nil {
		t.Fatalf("SKILL.md was not written: %v", err)
	}

	// Verify catalog status
	catalogAfter := hm.ListCatalog()
	var foundInstalled bool
	for _, s := range catalogAfter {
		if s.ID == targetID && s.Installed {
			foundInstalled = true
			break
		}
	}
	if !foundInstalled {
		t.Fatalf("skill %s should be marked installed", targetID)
	}

	// 2. Uninstall
	if err := hm.UninstallSkill(targetID); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, targetID)); !os.IsNotExist(err) {
		t.Fatal("skill dir should be removed after uninstall")
	}
}

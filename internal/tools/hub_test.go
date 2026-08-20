package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/actonos/actonos/internal/bus"
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

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/registry.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"version": "1.0.0",
				"totalSkills": 1,
				"skills": [
					{
						"id": "test-mock-skill-id",
						"slug": "mock_skill",
						"name": "Mock Skill",
						"description": "A dynamic mock skill for tests",
						"category": "software-dev",
						"author": "tester",
						"stars": 100,
						"files": ["SKILL.md", "scripts/test.sh"]
					}
				]
			}`))
			return
		}
		if r.URL.Path == "/skills/mock_skill/SKILL.md" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("---\nname: mock_skill\ndescription: test\n---\n# Mock Skill\nContent\n"))
			return
		}
		if r.URL.Path == "/skills/mock_skill/scripts/test.sh" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("#!/bin/sh\necho 'hello'\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()
	defer mockServer.CloseClientConnections()

	hm := NewHubManagerWithRegistry(tempDir, mockServer.URL+"/registry.json", mockServer.URL+"/skills")
	eb := bus.NewEventBus()
	defer eb.Close()
	hm.SetEventBus(eb)

	progressCh := eb.Subscribe(bus.EventSkillProgress)
	defer eb.Unsubscribe(bus.EventSkillProgress, progressCh)

	// Fetch from mock
	if err := hm.FetchRemoteCatalog(context.Background()); err != nil {
		t.Fatalf("fetching mock catalog: %v", err)
	}

	catalog := hm.ListCatalog()
	if len(catalog) != 1 {
		t.Fatalf("expected catalog to have 1 item, got %d", len(catalog))
	}

	target := catalog[0]
	targetID := target.ID
	slug := target.Slug

	// 1. Install
	if err := hm.InstallSkill(targetID); err != nil {
		t.Fatalf("install skill failed: %v", err)
	}

	installedSkillMD := filepath.Join(tempDir, slug, "SKILL.md")
	if _, err := os.Stat(installedSkillMD); err != nil {
		t.Fatalf("SKILL.md was not written at %s: %v", installedSkillMD, err)
	}

	installedScript := filepath.Join(tempDir, slug, "scripts", "test.sh")
	if _, err := os.Stat(installedScript); err != nil {
		t.Fatalf("scripts/test.sh was not written at %s: %v", installedScript, err)
	}

	// Verify catalog status
	catalogAfter := hm.ListCatalog()
	var foundInstalled bool
	for _, s := range catalogAfter {
		if (s.ID == targetID || s.Slug == slug) && s.Installed {
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
	if _, err := os.Stat(filepath.Join(tempDir, slug)); !os.IsNotExist(err) {
		t.Fatal("skill dir should be removed after uninstall")
	}
}

func TestSkillRequirements_VerificationAndGating(t *testing.T) {
	// 1. Valid requirements (current OS, empty env/bins)
	reqs := SkillRequirements{
		OS: []string{runtime.GOOS},
	}
	met, missing := VerifySkillRequirements(reqs)
	if !met || len(missing) > 0 {
		t.Errorf("expected current OS %s to pass requirements, got: %v", runtime.GOOS, missing)
	}

	// 2. Missing env var requirement
	reqsWithMissingEnv := SkillRequirements{
		Env: []string{"NON_EXISTENT_ACTONOS_TEST_ENV_VAR_12345"},
	}
	metEnv, missingEnv := VerifySkillRequirements(reqsWithMissingEnv)
	if metEnv || len(missingEnv) == 0 {
		t.Errorf("expected missing env to fail verification")
	}

	// 3. Missing CLI binary requirement
	reqsWithMissingBin := SkillRequirements{
		Bins: []string{"non_existent_binary_xyz_12345"},
	}
	metBin, missingBin := VerifySkillRequirements(reqsWithMissingBin)
	if metBin || len(missingBin) == 0 {
		t.Errorf("expected missing binary to fail verification")
	}
}

func TestSkillTool_EnableDisableToggle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "actonos-toggle-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	skillMD := `---
name: toggle_test
description: Skill toggle testing
---
# Toggle test
Instructions
`
	if err := os.WriteFile(filepath.Join(tempDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("writing skill md: %v", err)
	}

	st, err := NewSkillTool(tempDir)
	if err != nil {
		t.Fatalf("creating skill tool: %v", err)
	}

	if !st.IsEnabled() {
		t.Fatalf("skill should be enabled by default")
	}

	// Disable
	if err := st.SetEnabled(false); err != nil {
		t.Fatalf("disabling skill failed: %v", err)
	}
	if st.IsEnabled() {
		t.Fatalf("skill should be disabled")
	}

	// Re-enable
	if err := st.SetEnabled(true); err != nil {
		t.Fatalf("re-enabling skill failed: %v", err)
	}
	if !st.IsEnabled() {
		t.Fatalf("skill should be enabled")
	}
}


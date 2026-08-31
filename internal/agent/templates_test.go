package agent

import (
	"testing"
)

func TestBuiltinTemplatesCountAndIntegrity(t *testing.T) {
	templates := ListTemplates("", "")
	if len(templates) < 15 {
		t.Fatalf("expected at least 15 built-in agent templates, got %d", len(templates))
	}

	seenIDs := make(map[string]bool)
	for _, tmpl := range templates {
		if tmpl.ID == "" {
			t.Errorf("template missing ID: %+v", tmpl)
		}
		if seenIDs[tmpl.ID] {
			t.Errorf("duplicate template ID: %s", tmpl.ID)
		}
		seenIDs[tmpl.ID] = true

		if tmpl.Name == "" {
			t.Errorf("template %s missing name", tmpl.ID)
		}
		if tmpl.Category == "" {
			t.Errorf("template %s missing category", tmpl.ID)
		}
		if tmpl.Description == "" {
			t.Errorf("template %s missing description", tmpl.ID)
		}
		if tmpl.Manifest.SystemInstructions == "" {
			t.Errorf("template %s missing system instructions", tmpl.ID)
		}
		if tmpl.Manifest.ModelConfig.PrimaryModel == "" {
			t.Errorf("template %s missing primary model", tmpl.ID)
		}
	}
}

func TestListTemplatesFiltering(t *testing.T) {
	devTemplates := ListTemplates("development", "")
	if len(devTemplates) == 0 {
		t.Fatalf("expected development templates, got none")
	}
	for _, tmpl := range devTemplates {
		if tmpl.Category != "development" {
			t.Errorf("expected category development, got %s for %s", tmpl.Category, tmpl.ID)
		}
	}

	searchRes := ListTemplates("", "SEO")
	if len(searchRes) == 0 {
		t.Fatalf("expected at least one template matching query 'SEO', got 0")
	}
	if searchRes[0].ID != "seo_content_monitor" {
		t.Errorf("expected seo_content_monitor, got %s", searchRes[0].ID)
	}

	single, err := GetTemplateByID("code_reviewer")
	if err != nil || single == nil {
		t.Fatalf("failed to get code_reviewer template: %v", err)
	}
	if single.ID != "code_reviewer" {
		t.Errorf("expected code_reviewer, got %s", single.ID)
	}

	_, err = GetTemplateByID("non_existent_template")
	if err == nil {
		t.Fatalf("expected error for non-existent template ID, got nil")
	}
}

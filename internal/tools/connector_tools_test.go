package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockConnectorTokenProvider struct {
	tokens map[string]string
}

func (m *mockConnectorTokenProvider) GetConnectorToken(ctx context.Context, providerID string) (string, bool, error) {
	tok, ok := m.tokens[providerID]
	return tok, ok && tok != "", nil
}

func (m *mockConnectorTokenProvider) GetConnectedAccounts(ctx context.Context) []ConnectedAccountInfo {
	var list []ConnectedAccountInfo
	for k, v := range m.tokens {
		list = append(list, ConnectedAccountInfo{
			ID:          k,
			Name:        k,
			AccountName: "@user",
			Connected:   v != "",
		})
	}
	return list
}

func TestGitHubConnectorTool_Disconnected(t *testing.T) {
	provider := &mockConnectorTokenProvider{
		tokens: map[string]string{}, // No github token
	}
	tool := NewGitHubConnectorTool(provider)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"list_repositories"}`))
	if err != nil {
		t.Fatalf("expected no error from Execute when disconnected, got: %v", err)
	}
	if res.Error != "connector_disconnected" {
		t.Errorf("expected error code 'connector_disconnected', got %q", res.Error)
	}
}

func TestGitHubConnectorTool_ListRepositories(t *testing.T) {
	// Mock GitHub API Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-gh-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/user/repos" {
			repos := []map[string]any{
				{
					"name":             "ActonOS",
					"full_name":        "octocat/ActonOS",
					"description":      "Autonomous AI Agent OS",
					"private":          false,
					"html_url":         "https://github.com/octocat/ActonOS",
					"language":         "Go",
					"stargazers_count": 128,
					"forks_count":      12,
					"open_issues_count": 3,
					"updated_at":       "2026-08-22T12:00:00Z",
					"default_branch":   "main",
				},
			}
			json.NewEncoder(w).Encode(repos)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := &mockConnectorTokenProvider{
		tokens: map[string]string{"github": "test-gh-token"},
	}
	tool := NewGitHubConnectorTool(provider)
	tool.SetAPIBaseURL(server.URL)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"list_repositories","limit":10}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("tool returned error: %s", res.Error)
	}

	count, ok := res.Data["count"].(int)
	if !ok || count != 1 {
		t.Errorf("expected 1 repository in data, got %v", res.Data["count"])
	}
}

func TestGitHubConnectorTool_GetFileContent(t *testing.T) {
	content := "# Welcome to ActonOS\nAutonomous Kernel"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octocat/ActonOS/contents/README.md" {
			fileResp := map[string]any{
				"type":     "file",
				"name":     "README.md",
				"path":     "README.md",
				"size":     len(content),
				"encoding": "base64",
				"content":  encoded,
			}
			json.NewEncoder(w).Encode(fileResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := &mockConnectorTokenProvider{
		tokens: map[string]string{"github": "test-gh-token"},
	}
	tool := NewGitHubConnectorTool(provider)
	tool.SetAPIBaseURL(server.URL)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{
		"action": "get_file_content",
		"owner": "octocat",
		"repo": "ActonOS",
		"path": "README.md"
	}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.Content != content {
		t.Errorf("expected decoded content %q, got %q", content, res.Content)
	}
}

func TestGitHubConnectorTool_ListIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octocat/ActonOS/issues" {
			issues := []map[string]any{
				{
					"number":     42,
					"title":      "Add connector tools",
					"state":      "open",
					"comments":   5,
					"created_at": "2026-08-22T10:00:00Z",
					"html_url":   "https://github.com/octocat/ActonOS/issues/42",
					"user":       map[string]any{"login": "developer"},
					"labels":     []map[string]any{{"name": "feature"}},
				},
			}
			json.NewEncoder(w).Encode(issues)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := &mockConnectorTokenProvider{
		tokens: map[string]string{"github": "test-gh-token"},
	}
	tool := NewGitHubConnectorTool(provider)
	tool.SetAPIBaseURL(server.URL)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{
		"action": "list_issues",
		"owner": "octocat",
		"repo": "ActonOS",
		"state": "open"
	}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("tool returned error: %s", res.Error)
	}

	count, ok := res.Data["count"].(int)
	if !ok || count != 1 {
		t.Errorf("expected 1 issue, got %v", res.Data["count"])
	}
}

func TestRegisterConnectorTools(t *testing.T) {
	reg := NewToolRegistry(nil)
	provider := &mockConnectorTokenProvider{tokens: map[string]string{}}

	RegisterConnectorTools(reg, provider)

	expectedTools := []string{
		"connector_github",
		"connector_google_workspace",
		"connector_notion",
		"connector_slack",
	}

	for _, name := range expectedTools {
		tool, err := reg.Get(name)
		if err != nil || tool == nil {
			t.Errorf("expected tool %s to be registered", name)
		}
	}
}

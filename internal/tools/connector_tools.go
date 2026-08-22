package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ConnectorTokenProvider dynamically retrieves the active token and connection state for a SaaS provider.
type ConnectorTokenProvider interface {
	GetConnectorToken(ctx context.Context, providerID string) (token string, connected bool, err error)
	GetConnectedAccounts(ctx context.Context) []ConnectedAccountInfo
}

// ConnectedAccountInfo represents summary information of an active connector account.
type ConnectedAccountInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AccountName string `json:"account_name"`
	Connected   bool   `json:"connected"`
}

// Functional adapter for ConnectorTokenProvider
type ConnectorTokenProviderFunc func(ctx context.Context, providerID string) (token string, connected bool, err error)

func (f ConnectorTokenProviderFunc) GetConnectorToken(ctx context.Context, providerID string) (string, bool, error) {
	return f(ctx, providerID)
}

func (f ConnectorTokenProviderFunc) GetConnectedAccounts(ctx context.Context) []ConnectedAccountInfo {
	return nil
}

// =============================================================================
// GitHub Connector Tool
// =============================================================================

// GitHubConnectorTool enables autonomous agents to interact with GitHub.
type GitHubConnectorTool struct {
	provider   ConnectorTokenProvider
	httpClient *http.Client
	apiBaseURL string
}

// NewGitHubConnectorTool creates a new GitHubConnectorTool instance.
func NewGitHubConnectorTool(provider ConnectorTokenProvider) *GitHubConnectorTool {
	return &GitHubConnectorTool{
		provider: provider,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiBaseURL: "https://api.github.com",
	}
}

func (t *GitHubConnectorTool) SetAPIBaseURL(baseURL string) {
	t.apiBaseURL = strings.TrimRight(baseURL, "/")
}

func (t *GitHubConnectorTool) Name() string {
	return "connector_github"
}

func (t *GitHubConnectorTool) Description() string {
	return "Interact with connected GitHub account to list or search repositories, inspect issues, read file contents, check pull requests, and view profiles. Requires GitHub connector to be connected."
}

func (t *GitHubConnectorTool) Category() string {
	return "native"
}

func (t *GitHubConnectorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": [
					"list_repositories",
					"get_repository",
					"list_issues",
					"get_issue",
					"create_issue",
					"list_pull_requests",
					"get_file_content",
					"search_repositories",
					"search_code",
					"get_user"
				],
				"description": "The GitHub operation to perform."
			},
			"owner": {
				"type": "string",
				"description": "Owner or organization (e.g. 'octocat' or 'google'). Omit for authenticated user's own repos."
			},
			"repo": {
				"type": "string",
				"description": "Repository name (e.g. 'Hello-World')."
			},
			"path": {
				"type": "string",
				"description": "Path to file or directory within repository for 'get_file_content' (e.g. 'README.md' or 'src/index.ts')."
			},
			"ref": {
				"type": "string",
				"description": "Git commit branch, tag, or SHA for 'get_file_content' (default: default branch)."
			},
			"query": {
				"type": "string",
				"description": "Search keyword for 'search_repositories' or 'search_code'."
			},
			"state": {
				"type": "string",
				"enum": ["open", "closed", "all"],
				"description": "State filter for issues or pull requests (default: 'open')."
			},
			"issue_number": {
				"type": "integer",
				"description": "Issue or Pull Request number for 'get_issue'."
			},
			"title": {
				"type": "string",
				"description": "Title when creating an issue."
			},
			"body": {
				"type": "string",
				"description": "Body description when creating an issue."
			},
			"limit": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 30,
				"description": "Maximum number of items to return."
			},
			"sort": {
				"type": "string",
				"description": "Sort order ('updated', 'created', 'pushed', 'stars')."
			}
		},
		"required": ["action"]
	}`)
}

type gitHubInput struct {
	Action      string `json:"action"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Ref         string `json:"ref"`
	Query       string `json:"query"`
	State       string `json:"state"`
	IssueNumber int    `json:"issue_number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Limit       int    `json:"limit"`
	Sort        string `json:"sort"`
}

func (t *GitHubConnectorTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.provider == nil {
		return nil, errors.New("github connector token provider is not configured")
	}

	token, connected, err := t.provider.GetConnectorToken(ctx, "github")
	if err != nil {
		return nil, fmt.Errorf("resolving github credentials: %w", err)
	}
	if !connected || strings.TrimSpace(token) == "" {
		return &ToolResult{
			Content: "GitHub connector is currently disconnected or missing authentication. Please connect your GitHub account in the Connectors page (Settings > Connectors).",
			Error:   "connector_disconnected",
		}, nil
	}

	var in gitHubInput
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &in); err != nil {
		return nil, fmt.Errorf("decoding github tool parameters: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 30
	}
	if in.Limit > 100 {
		in.Limit = 100
	}
	if in.State == "" {
		in.State = "open"
	}

	switch in.Action {
	case "list_repositories":
		return t.listRepositories(ctx, token, in)
	case "get_repository":
		return t.getRepository(ctx, token, in)
	case "list_issues":
		return t.listIssues(ctx, token, in)
	case "get_issue":
		return t.getIssue(ctx, token, in)
	case "create_issue":
		return t.createIssue(ctx, token, in)
	case "list_pull_requests":
		return t.listPullRequests(ctx, token, in)
	case "get_file_content":
		return t.getFileContent(ctx, token, in)
	case "search_repositories":
		return t.searchRepositories(ctx, token, in)
	case "search_code":
		return t.searchCode(ctx, token, in)
	case "get_user":
		return t.getUser(ctx, token, in)
	default:
		return nil, fmt.Errorf("unsupported action '%s'", in.Action)
	}
}

func (t *GitHubConnectorTool) doRequest(ctx context.Context, token, method, path string, body any) (*http.Response, []byte, error) {
	reqURL := fmt.Sprintf("%s%s", t.apiBaseURL, path)
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "ActonOS-Kernel")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, respBytes, nil
}

func (t *GitHubConnectorTool) listRepositories(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	var endpoint string
	if in.Owner != "" {
		endpoint = fmt.Sprintf("/users/%s/repos?per_page=%d", url.PathEscape(in.Owner), in.Limit)
	} else {
		endpoint = fmt.Sprintf("/user/repos?per_page=%d&affiliation=owner,collaborator,organization_member", in.Limit)
	}

	if in.Sort != "" {
		endpoint += "&sort=" + url.QueryEscape(in.Sort)
	} else {
		endpoint += "&sort=updated"
	}

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	var repos []map[string]any
	if err := json.Unmarshal(respBytes, &repos); err != nil {
		return nil, err
	}

	type RepoSummary struct {
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		Private       bool   `json:"private"`
		HTMLURL       string `json:"html_url"`
		Language      string `json:"language"`
		Stargazers    int    `json:"stargazers_count"`
		Forks         int    `json:"forks_count"`
		OpenIssues    int    `json:"open_issues_count"`
		UpdatedAt     string `json:"updated_at"`
		DefaultBranch string `json:"default_branch"`
	}

	var summaries []RepoSummary
	for _, r := range repos {
		s := RepoSummary{
			Name:          getString(r, "name"),
			FullName:      getString(r, "full_name"),
			Description:   getString(r, "description"),
			Private:       getBool(r, "private"),
			HTMLURL:       getString(r, "html_url"),
			Language:      getString(r, "language"),
			Stargazers:    getInt(r, "stargazers_count"),
			Forks:         getInt(r, "forks_count"),
			OpenIssues:    getInt(r, "open_issues_count"),
			UpdatedAt:     getString(r, "updated_at"),
			DefaultBranch: getString(r, "default_branch"),
		}
		summaries = append(summaries, s)
	}

	contentBytes, _ := json.MarshalIndent(summaries, "", "  ")
	return &ToolResult{
		Content: string(contentBytes),
		Data: map[string]any{
			"count":        len(summaries),
			"repositories": summaries,
		},
	}, nil
}

func (t *GitHubConnectorTool) getRepository(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if in.Owner == "" || in.Repo == "" {
		return nil, errors.New("'owner' and 'repo' are required for get_repository")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s", url.PathEscape(in.Owner), url.PathEscape(in.Repo))
	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) listIssues(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if in.Owner == "" || in.Repo == "" {
		return nil, errors.New("'owner' and 'repo' are required for list_issues")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=%d",
		url.PathEscape(in.Owner), url.PathEscape(in.Repo), url.QueryEscape(in.State), in.Limit)

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	var rawIssues []map[string]any
	if err := json.Unmarshal(respBytes, &rawIssues); err != nil {
		return nil, err
	}

	type IssueSummary struct {
		Number    int      `json:"number"`
		Title     string   `json:"title"`
		State     string   `json:"state"`
		User      string   `json:"user"`
		Comments  int      `json:"comments"`
		CreatedAt string   `json:"created_at"`
		HTMLURL   string   `json:"html_url"`
		Labels    []string `json:"labels"`
		IsPR      bool     `json:"is_pull_request"`
	}

	var issues []IssueSummary
	for _, raw := range rawIssues {
		_, isPR := raw["pull_request"]
		userMap, _ := raw["user"].(map[string]any)
		userName := getString(userMap, "login")

		var labels []string
		if rawLabels, ok := raw["labels"].([]any); ok {
			for _, l := range rawLabels {
				if lMap, ok := l.(map[string]any); ok {
					labels = append(labels, getString(lMap, "name"))
				}
			}
		}

		issues = append(issues, IssueSummary{
			Number:    getInt(raw, "number"),
			Title:     getString(raw, "title"),
			State:     getString(raw, "state"),
			User:      userName,
			Comments:  getInt(raw, "comments"),
			CreatedAt: getString(raw, "created_at"),
			HTMLURL:   getString(raw, "html_url"),
			Labels:    labels,
			IsPR:      isPR,
		})
	}

	contentBytes, _ := json.MarshalIndent(issues, "", "  ")
	return &ToolResult{
		Content: string(contentBytes),
		Data: map[string]any{
			"count":  len(issues),
			"issues": issues,
		},
	}, nil
}

func (t *GitHubConnectorTool) getIssue(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if in.Owner == "" || in.Repo == "" || in.IssueNumber <= 0 {
		return nil, errors.New("'owner', 'repo', and 'issue_number' are required for get_issue")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d",
		url.PathEscape(in.Owner), url.PathEscape(in.Repo), in.IssueNumber)

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) createIssue(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if in.Owner == "" || in.Repo == "" || strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("'owner', 'repo', and 'title' are required for create_issue")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues", url.PathEscape(in.Owner), url.PathEscape(in.Repo))
	body := map[string]any{
		"title": in.Title,
		"body":  in.Body,
	}

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) listPullRequests(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if in.Owner == "" || in.Repo == "" {
		return nil, errors.New("'owner' and 'repo' are required for list_pull_requests")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=%d",
		url.PathEscape(in.Owner), url.PathEscape(in.Repo), url.QueryEscape(in.State), in.Limit)

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) getFileContent(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if in.Owner == "" || in.Repo == "" {
		return nil, errors.New("'owner' and 'repo' are required for get_file_content")
	}

	cleanPath := strings.TrimPrefix(in.Path, "/")
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s",
		url.PathEscape(in.Owner), url.PathEscape(in.Repo), cleanPath)

	if in.Ref != "" {
		endpoint += "?ref=" + url.QueryEscape(in.Ref)
	}

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	// Check if this is a file with Base64 content or directory array
	var fileObj struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Path     string `json:"path"`
		Size     int64  `json:"size"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}

	if err := json.Unmarshal(respBytes, &fileObj); err == nil && fileObj.Type == "file" {
		if fileObj.Encoding == "base64" {
			// Strip newlines in base64
			cleaned := strings.ReplaceAll(fileObj.Content, "\n", "")
			cleaned = strings.ReplaceAll(cleaned, "\r", "")
			decoded, decodeErr := base64.StdEncoding.DecodeString(cleaned)
			if decodeErr == nil {
				return &ToolResult{
					Content: string(decoded),
					Data: map[string]any{
						"path": fileObj.Path,
						"size": fileObj.Size,
						"name": fileObj.Name,
					},
				}, nil
			}
		}
	}

	// Default fallback (e.g. directory listing)
	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) searchRepositories(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, errors.New("'query' is required for search_repositories")
	}

	endpoint := fmt.Sprintf("/search/repositories?q=%s&per_page=%d", url.QueryEscape(in.Query), in.Limit)
	if in.Sort != "" {
		endpoint += "&sort=" + url.QueryEscape(in.Sort)
	}

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) searchCode(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, errors.New("'query' is required for search_code")
	}

	q := in.Query
	if in.Owner != "" && in.Repo != "" {
		q += fmt.Sprintf(" repo:%s/%s", in.Owner, in.Repo)
	} else if in.Owner != "" {
		q += fmt.Sprintf(" user:%s", in.Owner)
	}

	endpoint := fmt.Sprintf("/search/code?q=%s&per_page=%d", url.QueryEscape(q), in.Limit)
	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

func (t *GitHubConnectorTool) getUser(ctx context.Context, token string, in gitHubInput) (*ToolResult, error) {
	var endpoint string
	if in.Owner != "" {
		endpoint = fmt.Sprintf("/users/%s", url.PathEscape(in.Owner))
	} else {
		endpoint = "/user"
	}

	resp, respBytes, err := t.doRequest(ctx, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{
			Content: fmt.Sprintf("GitHub API returned error (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Error:   fmt.Sprintf("http_%d", resp.StatusCode),
		}, nil
	}

	return &ToolResult{
		Content: string(respBytes),
	}, nil
}

// =============================================================================
// Google Workspace Connector Tool
// =============================================================================

type GoogleWorkspaceConnectorTool struct {
	provider   ConnectorTokenProvider
	httpClient *http.Client
}

func NewGoogleWorkspaceConnectorTool(provider ConnectorTokenProvider) *GoogleWorkspaceConnectorTool {
	return &GoogleWorkspaceConnectorTool{
		provider: provider,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *GoogleWorkspaceConnectorTool) Name() string {
	return "connector_google_workspace"
}

func (t *GoogleWorkspaceConnectorTool) Description() string {
	return "Interact with connected Google Workspace account to search emails (Gmail), list upcoming Google Calendar events, and search Google Drive files."
}

func (t *GoogleWorkspaceConnectorTool) Category() string {
	return "native"
}

func (t *GoogleWorkspaceConnectorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list_emails", "read_email", "list_calendar_events", "search_drive_files"],
				"description": "The Google Workspace operation to perform."
			},
			"query": {
				"type": "string",
				"description": "Search query for Gmail or Drive."
			},
			"message_id": {
				"type": "string",
				"description": "Specific Gmail message ID for 'read_email'."
			},
			"limit": {
				"type": "integer",
				"minimum": 1,
				"maximum": 50,
				"default": 10,
				"description": "Maximum number of records to return."
			}
		},
		"required": ["action"]
	}`)
}

func (t *GoogleWorkspaceConnectorTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.provider == nil {
		return nil, errors.New("google workspace token provider is not configured")
	}

	token, connected, err := t.provider.GetConnectorToken(ctx, "google_workspace")
	if err != nil {
		return nil, fmt.Errorf("resolving google workspace credentials: %w", err)
	}
	if !connected || strings.TrimSpace(token) == "" {
		return &ToolResult{
			Content: "Google Workspace connector is currently disconnected. Please authenticate Google Workspace in Connectors page.",
			Error:   "connector_disconnected",
		}, nil
	}

	var in struct {
		Action    string `json:"action"`
		Query     string `json:"query"`
		MessageID string `json:"message_id"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &in); err != nil {
		return nil, fmt.Errorf("decoding parameters: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 10
	}

	switch in.Action {
	case "list_emails":
		reqURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages?maxResults=%d", in.Limit)
		if in.Query != "" {
			reqURL += "&q=" + url.QueryEscape(in.Query)
		}
		return t.fetchGoogleAPI(ctx, token, reqURL)

	case "read_email":
		if in.MessageID == "" {
			return nil, errors.New("'message_id' is required for read_email")
		}
		reqURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s", url.PathEscape(in.MessageID))
		return t.fetchGoogleAPI(ctx, token, reqURL)

	case "list_calendar_events":
		now := time.Now().UTC().Format(time.RFC3339)
		reqURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/primary/events?timeMin=%s&maxResults=%d&singleEvents=true&orderBy=startTime",
			url.QueryEscape(now), in.Limit)
		return t.fetchGoogleAPI(ctx, token, reqURL)

	case "search_drive_files":
		reqURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files?pageSize=%d&fields=files(id,name,mimeType,modifiedTime,size)", in.Limit)
		if in.Query != "" {
			reqURL += "&q=" + url.QueryEscape(fmt.Sprintf("name contains '%s' and trashed = false", in.Query))
		}
		return t.fetchGoogleAPI(ctx, token, reqURL)

	default:
		return nil, fmt.Errorf("unsupported action '%s'", in.Action)
	}
}

func (t *GoogleWorkspaceConnectorTool) fetchGoogleAPI(ctx context.Context, token, reqURL string) (*ToolResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Content: string(b),
	}, nil
}

// =============================================================================
// Notion Connector Tool
// =============================================================================

type NotionConnectorTool struct {
	provider   ConnectorTokenProvider
	httpClient *http.Client
}

func NewNotionConnectorTool(provider ConnectorTokenProvider) *NotionConnectorTool {
	return &NotionConnectorTool{
		provider: provider,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *NotionConnectorTool) Name() string {
	return "connector_notion"
}

func (t *NotionConnectorTool) Description() string {
	return "Interact with connected Notion workspace to search documents, query databases, read pages, and append meeting notes."
}

func (t *NotionConnectorTool) Category() string {
	return "native"
}

func (t *NotionConnectorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["search", "query_database", "get_page", "get_block_children", "create_page"],
				"description": "The Notion operation to perform."
			},
			"query": {
				"type": "string",
				"description": "Keyword search across workspace pages and databases."
			},
			"database_id": {
				"type": "string",
				"description": "Database ID for 'query_database'."
			},
			"page_id": {
				"type": "string",
				"description": "Page ID for 'get_page'."
			},
			"block_id": {
				"type": "string",
				"description": "Block ID for 'get_block_children'."
			},
			"title": {
				"type": "string",
				"description": "Title when creating a page."
			},
			"content": {
				"type": "string",
				"description": "Markdown/text content when creating a page."
			},
			"limit": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 20
			}
		},
		"required": ["action"]
	}`)
}

func (t *NotionConnectorTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.provider == nil {
		return nil, errors.New("notion token provider is not configured")
	}

	token, connected, err := t.provider.GetConnectorToken(ctx, "notion")
	if err != nil {
		return nil, fmt.Errorf("resolving notion credentials: %w", err)
	}
	if !connected || strings.TrimSpace(token) == "" {
		return &ToolResult{
			Content: "Notion connector is currently disconnected. Please connect Notion in Connectors page.",
			Error:   "connector_disconnected",
		}, nil
	}

	var in struct {
		Action     string `json:"action"`
		Query      string `json:"query"`
		DatabaseID string `json:"database_id"`
		PageID     string `json:"page_id"`
		BlockID    string `json:"block_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &in); err != nil {
		return nil, fmt.Errorf("decoding parameters: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 20
	}

	switch in.Action {
	case "search":
		body := map[string]any{"page_size": in.Limit}
		if in.Query != "" {
			body["query"] = in.Query
		}
		return t.doNotionRequest(ctx, token, http.MethodPost, "https://api.notion.com/v1/search", body)

	case "query_database":
		if in.DatabaseID == "" {
			return nil, errors.New("'database_id' is required for query_database")
		}
		reqURL := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", in.DatabaseID)
		body := map[string]any{"page_size": in.Limit}
		return t.doNotionRequest(ctx, token, http.MethodPost, reqURL, body)

	case "get_page":
		if in.PageID == "" {
			return nil, errors.New("'page_id' is required for get_page")
		}
		reqURL := fmt.Sprintf("https://api.notion.com/v1/pages/%s", in.PageID)
		return t.doNotionRequest(ctx, token, http.MethodGet, reqURL, nil)

	case "get_block_children":
		blockID := in.BlockID
		if blockID == "" {
			blockID = in.PageID
		}
		if blockID == "" {
			return nil, errors.New("'block_id' or 'page_id' is required for get_block_children")
		}
		reqURL := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children?page_size=%d", blockID, in.Limit)
		return t.doNotionRequest(ctx, token, http.MethodGet, reqURL, nil)

	default:
		return nil, fmt.Errorf("unsupported action '%s'", in.Action)
	}
}

func (t *NotionConnectorTool) doNotionRequest(ctx context.Context, token, method, reqURL string, body any) (*ToolResult, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", "2022-06-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Content: string(b),
	}, nil
}

// =============================================================================
// Slack Connector Tool
// =============================================================================

type SlackConnectorTool struct {
	provider   ConnectorTokenProvider
	httpClient *http.Client
}

func NewSlackConnectorTool(provider ConnectorTokenProvider) *SlackConnectorTool {
	return &SlackConnectorTool{
		provider: provider,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *SlackConnectorTool) Name() string {
	return "connector_slack"
}

func (t *SlackConnectorTool) Description() string {
	return "Interact with connected Slack workspace to list channels, read conversations, and post messages."
}

func (t *SlackConnectorTool) Category() string {
	return "native"
}

func (t *SlackConnectorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list_channels", "read_channel_messages", "post_message"],
				"description": "The Slack operation to perform."
			},
			"channel_id": {
				"type": "string",
				"description": "Slack channel ID (e.g. 'C0123456789')."
			},
			"message": {
				"type": "string",
				"description": "Text message to post to the channel."
			},
			"limit": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 20
			}
		},
		"required": ["action"]
	}`)
}

func (t *SlackConnectorTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if t.provider == nil {
		return nil, errors.New("slack token provider is not configured")
	}

	token, connected, err := t.provider.GetConnectorToken(ctx, "slack")
	if err != nil {
		return nil, fmt.Errorf("resolving slack credentials: %w", err)
	}
	if !connected || strings.TrimSpace(token) == "" {
		return &ToolResult{
			Content: "Slack connector is currently disconnected. Please connect Slack in Connectors page.",
			Error:   "connector_disconnected",
		}, nil
	}

	var in struct {
		Action    string `json:"action"`
		ChannelID string `json:"channel_id"`
		Message   string `json:"message"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(NormalizeToolInput(inputJSON), &in); err != nil {
		return nil, fmt.Errorf("decoding parameters: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 20
	}

	switch in.Action {
	case "list_channels":
		reqURL := fmt.Sprintf("https://slack.com/api/conversations.list?types=public_channel,private_channel&limit=%d", in.Limit)
		return t.doSlackRequest(ctx, token, http.MethodGet, reqURL, nil)

	case "read_channel_messages":
		if in.ChannelID == "" {
			return nil, errors.New("'channel_id' is required for read_channel_messages")
		}
		reqURL := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=%d", url.QueryEscape(in.ChannelID), in.Limit)
		return t.doSlackRequest(ctx, token, http.MethodGet, reqURL, nil)

	case "post_message":
		if in.ChannelID == "" || strings.TrimSpace(in.Message) == "" {
			return nil, errors.New("'channel_id' and 'message' are required for post_message")
		}
		body := map[string]any{
			"channel": in.ChannelID,
			"text":    in.Message,
		}
		return t.doSlackRequest(ctx, token, http.MethodPost, "https://slack.com/api/chat.postMessage", body)

	default:
		return nil, fmt.Errorf("unsupported action '%s'", in.Action)
	}
}

func (t *SlackConnectorTool) doSlackRequest(ctx context.Context, token, method, reqURL string, body any) (*ToolResult, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Content: string(b),
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

// RegisterConnectorTools registers all standard SaaS connector tools into a ToolRegistry.
func RegisterConnectorTools(r *ToolRegistry, provider ConnectorTokenProvider) {
	_ = r.Register(NewGitHubConnectorTool(provider))
	_ = r.Register(NewGoogleWorkspaceConnectorTool(provider))
	_ = r.Register(NewNotionConnectorTool(provider))
	_ = r.Register(NewSlackConnectorTool(provider))
}

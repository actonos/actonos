package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/channels"
	"github.com/go-chi/chi/v5"
)

// ConnectorAccount holds identity information for a connected SaaS service.
type ConnectorAccount struct {
	ID           string   `json:"id"`            // e.g. "google_workspace", "github", "notion", "slack"
	Name         string   `json:"name"`          // Display name
	Category     string   `json:"category"`      // "Productivity", "Development", "Knowledge", "Messaging"
	Icon         string   `json:"icon"`          // "mail", "github", "book-open", "message-circle"
	RiskLevel    string   `json:"risk_level"`    // "Low", "Medium", "High"
	Description  string   `json:"description"`   // Brief explanation
	Connected    bool     `json:"connected"`     // Whether currently authenticated
	AuthType     string   `json:"auth_type"`     // "oauth" or "token"
	AccountName  string   `json:"account_name"`  // e.g. "@octocat", "user@gmail.com", "Acme Team"
	AccountEmail string   `json:"account_email"` // e.g. "user@example.com"
	AvatarURL    string   `json:"avatar_url"`    // User avatar URL
	ConnectedAt  string   `json:"connected_at"`  // RFC3339 timestamp
	Scopes       []string `json:"scopes"`        // Granted scopes
	ExpiresAt    string   `json:"expires_at"`    // Token expiry if OAuth
	ClientID     string   `json:"client_id"`     // Custom OAuth Client ID (masked)
	ClientSecret string   `json:"client_secret"` // Custom OAuth Secret (masked)
	DirectToken  string   `json:"direct_token"`  // Saved direct token (masked)
}

// Default standard SaaS Connectors definitions
var defaultConnectors = []ConnectorAccount{
	{
		ID:          "google_workspace",
		Name:        "Google Workspace",
		Category:    "Productivity",
		Icon:        "mail",
		RiskLevel:   "Medium",
		Description: "Access emails, calendar events, and read/write Google Docs & Drive files.",
		Scopes: []string{
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/calendar.events",
			"https://www.googleapis.com/auth/drive.readonly",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
	},
	{
		ID:          "github",
		Name:        "GitHub",
		Category:    "Development",
		Icon:        "github",
		RiskLevel:   "High",
		Description: "Inspect repositories, file issues, create pull requests, and review code.",
		Scopes: []string{
			"repo",
			"read:user",
			"user:email",
		},
	},
	{
		ID:          "notion",
		Name:        "Notion",
		Category:    "Knowledge",
		Icon:        "book-open",
		RiskLevel:   "Medium",
		Description: "Search workspace documents, create database entries, and append meeting notes.",
		Scopes: []string{
			"read_content",
			"update_content",
			"insert_content",
		},
	},
	{
		ID:          "slack",
		Name:        "Slack",
		Category:    "Messaging",
		Icon:        "message-circle",
		RiskLevel:   "Medium",
		Description: "Send channel messages, read mentions, and dispatch bot notifications.",
		Scopes: []string{
			"chat:write",
			"channels:read",
			"channels:history",
			"users:read",
		},
	},
}

var (
	connectorsMu sync.RWMutex
)

// Helper: load stored connectors metadata from disk
func (s *Server) loadStoredConnectors() map[string]ConnectorAccount {
	configDir := filepath.Join(s.dataDir, "config")
	filePath := filepath.Join(configDir, "connectors.json")

	result := make(map[string]ConnectorAccount)
	for _, def := range defaultConnectors {
		result[def.ID] = def
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return result
	}

	var stored map[string]ConnectorAccount
	if err := json.Unmarshal(data, &stored); err == nil {
		for id, conn := range stored {
			base := result[id]
			base.Connected = conn.Connected
			base.AuthType = conn.AuthType
			base.AccountName = conn.AccountName
			base.AccountEmail = conn.AccountEmail
			base.AvatarURL = conn.AvatarURL
			base.ConnectedAt = conn.ConnectedAt
			base.ExpiresAt = conn.ExpiresAt
			base.ClientID = conn.ClientID
			base.ClientSecret = conn.ClientSecret
			base.DirectToken = conn.DirectToken
			if len(conn.Scopes) > 0 {
				base.Scopes = conn.Scopes
			}
			result[id] = base
		}
	}

	return result
}

// Helper: save connectors metadata to disk
func (s *Server) saveStoredConnectors(connectors map[string]ConnectorAccount) error {
	configDir := filepath.Join(s.dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)
	filePath := filepath.Join(configDir, "connectors.json")

	data, err := json.MarshalIndent(connectors, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0600)
}

// Helper: get OAuth configuration for a provider
func (s *Server) getOAuthProviderConfig(providerID string, customClientID, customClientSecret string) auth.OAuthProviderConfig {
	cfg := auth.OAuthProviderConfig{
		ProviderID:   providerID,
		ClientID:     customClientID,
		ClientSecret: customClientSecret,
	}

	switch providerID {
	case "google_workspace":
		cfg.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
		cfg.TokenURL = "https://oauth2.googleapis.com/token"
		cfg.Scopes = []string{
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/calendar.events",
			"https://www.googleapis.com/auth/drive.readonly",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		}
		if cfg.ClientID == "" {
			cfg.ClientID = "actonos-google-client.apps.googleusercontent.com"
		}
	case "github":
		cfg.AuthURL = "https://github.com/login/oauth/authorize"
		cfg.TokenURL = "https://github.com/login/oauth/access_token"
		cfg.Scopes = []string{"repo", "read:user", "user:email"}
		if cfg.ClientID == "" {
			cfg.ClientID = "Ov23liActonOSAppID"
		}
	case "notion":
		cfg.AuthURL = "https://api.notion.com/v1/oauth/authorize"
		cfg.TokenURL = "https://api.notion.com/v1/oauth/token"
		cfg.Scopes = []string{"read_content", "update_content", "insert_content"}
		if cfg.ClientID == "" {
			cfg.ClientID = "acton-notion-client-id"
		}
	case "slack":
		cfg.AuthURL = "https://slack.com/oauth/v2/authorize"
		cfg.TokenURL = "https://slack.com/api/oauth.v2.access"
		cfg.Scopes = []string{"chat:write", "channels:read", "channels:history", "users:read"}
		if cfg.ClientID == "" {
			cfg.ClientID = "acton-slack-client-id"
		}
	}

	return cfg
}

// Validation function: test token against real provider API
type ProviderIdentity struct {
	AccountName  string
	AccountEmail string
	AvatarURL    string
}

func validateTokenWithProvider(ctx context.Context, providerID, token string) (*ProviderIdentity, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	switch providerID {
	case "github":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "ActonOS-Kernel")
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github api request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("github auth failed (HTTP %d): %s", resp.StatusCode, string(b))
		}

		var user struct {
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
			return nil, err
		}

		displayName := user.Login
		if user.Name != "" {
			displayName = fmt.Sprintf("%s (@%s)", user.Name, user.Login)
		}
		return &ProviderIdentity{
			AccountName:  displayName,
			AccountEmail: user.Email,
			AvatarURL:    user.AvatarURL,
		}, nil

	case "google_workspace":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("google userinfo request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("google auth failed (HTTP %d): %s", resp.StatusCode, string(b))
		}

		var u struct {
			Name    string `json:"name"`
			Email   string `json:"email"`
			Picture string `json:"picture"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
			return nil, err
		}

		return &ProviderIdentity{
			AccountName:  u.Name,
			AccountEmail: u.Email,
			AvatarURL:    u.Picture,
		}, nil

	case "notion":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.notion.com/v1/users/me", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Notion-Version", "2022-06-28")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("notion api request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("notion auth failed (HTTP %d): %s", resp.StatusCode, string(b))
		}

		var u struct {
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
			Person    struct {
				Email string `json:"email"`
			} `json:"person"`
			Bot struct {
				WorkspaceName string `json:"workspace_name"`
			} `json:"bot"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
			return nil, err
		}

		name := u.Name
		if u.Bot.WorkspaceName != "" {
			name = fmt.Sprintf("%s (Workspace: %s)", u.Name, u.Bot.WorkspaceName)
		}
		return &ProviderIdentity{
			AccountName:  name,
			AccountEmail: u.Person.Email,
			AvatarURL:    u.AvatarURL,
		}, nil

	case "slack":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/auth.test", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("slack api request: %w", err)
		}
		defer resp.Body.Close()

		var res struct {
			OK    bool   `json:"ok"`
			User  string `json:"user"`
			Team  string `json:"team"`
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}

		if !res.OK {
			return nil, fmt.Errorf("slack auth failed: %s", res.Error)
		}

		return &ProviderIdentity{
			AccountName: fmt.Sprintf("%s @ %s", res.User, res.Team),
		}, nil

	default:
		return &ProviderIdentity{AccountName: providerID}, nil
	}
}

// GET /api/integrations
func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	connectorsMu.RLock()
	stored := s.loadStoredConnectors()
	connectorsMu.RUnlock()

	var list []ConnectorAccount
	for _, def := range defaultConnectors {
		if c, ok := stored[def.ID]; ok {
			// Mask sensitive fields for response
			c.ClientID = maskKey(c.ClientID)
			c.ClientSecret = maskKey(c.ClientSecret)
			c.DirectToken = maskKey(c.DirectToken)
			list = append(list, c)
		} else {
			list = append(list, def)
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"integrations": list,
		"count":        len(list),
	})
}

// POST /api/integrations/{provider}/auth-url
func (s *Server) handleGetAuthURL(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	var req struct {
		ClientID     string `json:"client_id,omitempty"`
		ClientSecret string `json:"client_secret,omitempty"`
		RedirectURI  string `json:"redirect_uri,omitempty"`
	}
	_ = s.decodeJSON(r, &req)

	connectorsMu.RLock()
	stored := s.loadStoredConnectors()
	connectorsMu.RUnlock()

	existing := stored[provider]
	clientID := req.ClientID
	if clientID == "" {
		clientID = existing.ClientID
	}
	clientSecret := req.ClientSecret
	if clientSecret == "" {
		clientSecret = existing.ClientSecret
	}

	cfg := s.getOAuthProviderConfig(provider, clientID, clientSecret)

	redirectURI := req.RedirectURI
	if redirectURI == "" {
		host := r.Host
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		redirectURI = fmt.Sprintf("%s://%s/api/integrations/oauth/callback", scheme, host)
	}

	if s.oauthEngine == nil {
		s.respondError(w, http.StatusInternalServerError, "OAUTH_NOT_INITIALIZED", "OAuth engine is not initialized")
		return
	}

	authURL, state, _, err := s.oauthEngine.BuildAuthURL(cfg, redirectURI, cfg.Scopes)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "AUTH_URL_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"provider":     provider,
		"auth_url":     authURL,
		"state":        state,
		"redirect_uri": redirectURI,
	})
}

// GET /api/integrations/oauth/callback (and /api/auth/callback)
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errQuery := r.URL.Query().Get("error")
	errDesc := r.URL.Query().Get("error_description")

	if errQuery != "" {
		targetURL := fmt.Sprintf("/connectors?error=%s&error_description=%s", url.QueryEscape(errQuery), url.QueryEscape(errDesc))
		http.Redirect(w, r, targetURL, http.StatusFound)
		return
	}

	if code == "" || state == "" {
		http.Redirect(w, r, "/connectors?error=missing_code_or_state", http.StatusFound)
		return
	}

	if s.stateStore == nil || s.oauthEngine == nil {
		http.Redirect(w, r, "/connectors?error=oauth_not_initialized", http.StatusFound)
		return
	}

	session, err := s.stateStore.Consume(state)
	if err != nil {
		http.Redirect(w, r, "/connectors?error=invalid_or_expired_state", http.StatusFound)
		return
	}

	provider := session.Provider

	connectorsMu.RLock()
	stored := s.loadStoredConnectors()
	connectorsMu.RUnlock()

	existing := stored[provider]
	cfg := s.getOAuthProviderConfig(provider, existing.ClientID, existing.ClientSecret)

	tokenResp, err := s.oauthEngine.ExchangeCode(r.Context(), cfg, code, session.RedirectURI, session.CodeVerifier)
	if err != nil {
		targetURL := fmt.Sprintf("/connectors?error=exchange_failed&details=%s", url.QueryEscape(err.Error()))
		http.Redirect(w, r, targetURL, http.StatusFound)
		return
	}

	// Validate token with provider to fetch user identity
	identity, _ := validateTokenWithProvider(r.Context(), provider, tokenResp.AccessToken)

	// Save token in token daemon / vault
	if s.tokenDaemon != nil {
		_ = s.tokenDaemon.SaveToken(r.Context(), provider, tokenResp)
	}

	// Update connector state
	connectorsMu.Lock()
	conn := stored[provider]
	conn.Connected = true
	conn.AuthType = "oauth"
	conn.ConnectedAt = time.Now().UTC().Format(time.RFC3339)
	conn.ExpiresAt = tokenResp.ExpiresAt.Format(time.RFC3339)
	if identity != nil {
		conn.AccountName = identity.AccountName
		conn.AccountEmail = identity.AccountEmail
		conn.AvatarURL = identity.AvatarURL
	}
	stored[provider] = conn
	_ = s.saveStoredConnectors(stored)
	connectorsMu.Unlock()

	// Redirect to frontend connectors page with success notification
	http.Redirect(w, r, fmt.Sprintf("/connectors?connected=%s&status=success", provider), http.StatusFound)
}

// POST /api/integrations/{provider}/token (Direct Personal Access Token / API Key)
func (s *Server) handleSaveDirectToken(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	var req struct {
		Token string `json:"token"`
	}
	if err := s.decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Token) == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "token is required")
		return
	}

	rawToken := strings.TrimSpace(req.Token)

	// Live validation against provider
	identity, err := validateTokenWithProvider(r.Context(), provider, rawToken)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "TOKEN_VERIFICATION_FAILED", err.Error())
		return
	}

	connectorsMu.Lock()
	defer connectorsMu.Unlock()

	stored := s.loadStoredConnectors()
	conn := stored[provider]
	conn.Connected = true
	conn.AuthType = "token"
	conn.DirectToken = rawToken
	conn.ConnectedAt = time.Now().UTC().Format(time.RFC3339)
	if identity != nil {
		conn.AccountName = identity.AccountName
		conn.AccountEmail = identity.AccountEmail
		conn.AvatarURL = identity.AvatarURL
	}
	stored[provider] = conn
	_ = s.saveStoredConnectors(stored)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "connected",
		"provider":  provider,
		"auth_type": "token",
		"identity":  identity,
	})
}

// POST /api/integrations/{provider}/config (Save custom Client ID / Secret)
func (s *Server) handleSaveProviderConfig(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	connectorsMu.Lock()
	defer connectorsMu.Unlock()

	stored := s.loadStoredConnectors()
	conn := stored[provider]
	if req.ClientID != "" {
		conn.ClientID = strings.TrimSpace(req.ClientID)
	}
	if req.ClientSecret != "" {
		conn.ClientSecret = strings.TrimSpace(req.ClientSecret)
	}
	stored[provider] = conn
	_ = s.saveStoredConnectors(stored)

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// POST /api/integrations/{provider}/test
func (s *Server) handleTestIntegration(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	connectorsMu.RLock()
	stored := s.loadStoredConnectors()
	connectorsMu.RUnlock()

	conn, exists := stored[provider]
	if !exists || !conn.Connected {
		s.respondError(w, http.StatusBadRequest, "NOT_CONNECTED", "connector is not connected")
		return
	}

	token := conn.DirectToken
	if conn.AuthType == "oauth" && s.tokenDaemon != nil {
		storedToken, err := s.tokenDaemon.GetToken(r.Context(), provider)
		if err == nil && storedToken != nil {
			token = storedToken.AccessToken
		}
	}

	if token == "" {
		s.respondError(w, http.StatusBadRequest, "TOKEN_UNAVAILABLE", "no active token found for provider")
		return
	}

	start := time.Now()
	identity, err := validateTokenWithProvider(r.Context(), provider, token)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		s.respondError(w, http.StatusBadRequest, "TEST_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":     "connected",
		"provider":   provider,
		"latency_ms": latency,
		"identity":   identity,
	})
}

// POST /api/integrations/{provider}/disconnect
func (s *Server) handleDisconnectIntegration(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	connectorsMu.Lock()
	defer connectorsMu.Unlock()

	stored := s.loadStoredConnectors()
	conn := stored[provider]
	conn.Connected = false
	conn.DirectToken = ""
	conn.AccountName = ""
	conn.AccountEmail = ""
	conn.AvatarURL = ""
	conn.ExpiresAt = ""
	stored[provider] = conn
	_ = s.saveStoredConnectors(stored)

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":   "disconnected",
		"provider": provider,
	})
}

// POST /api/integrations/{provider}/toggle (backward compat)
func (s *Server) handleToggleIntegration(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	connectorsMu.Lock()
	defer connectorsMu.Unlock()

	stored := s.loadStoredConnectors()
	conn := stored[provider]
	conn.Connected = !conn.Connected
	stored[provider] = conn
	_ = s.saveStoredConnectors(stored)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"provider":  provider,
		"connected": conn.Connected,
	})
}

// Channels & Inbound Webhooks

// ChannelConfigResponse returns channel configurations with multi-account support.
type ChannelConfigResponse struct {
	Telegram      []channels.ChannelAccount `json:"telegram"`
	Discord       []channels.ChannelAccount `json:"discord"`
	WhatsApp      []channels.ChannelAccount `json:"whatsapp"`
	WebhookSecret string                    `json:"webhook_secret"`
	WebhookURL    string                    `json:"webhook_url"`
}

func (s *Server) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	readKey := func(filename string) string {
		data, err := os.ReadFile(filepath.Join(configDir, filename))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	// Load multi-account JSON if available, fallback to single-token files
	tgAccounts := loadChannelAccounts(configDir, "telegram")
	if len(tgAccounts) == 0 {
		if tg := readKey("telegram.token"); tg != "" {
			tgAccounts = []channels.ChannelAccount{{
				ID: "tg_default", Name: "Primary Bot", Channel: "telegram", Token: maskKey(tg), Enabled: true, BoundAgentIDs: []string{"*"},
			}}
		}
	}

	dcAccounts := loadChannelAccounts(configDir, "discord")
	if len(dcAccounts) == 0 {
		if dc := readKey("discord.token"); dc != "" {
			dcAccounts = []channels.ChannelAccount{{
				ID: "dc_default", Name: "Primary Bot", Channel: "discord", Token: maskKey(dc), Enabled: true, BoundAgentIDs: []string{"*"},
			}}
		}
	}

	waAccounts := loadChannelAccounts(configDir, "whatsapp")
	if len(waAccounts) == 0 {
		waToken := readKey("whatsapp.token")
		waPhone := readKey("whatsapp.phone_id")
		if waToken != "" && waPhone != "" {
			waAccounts = []channels.ChannelAccount{{
				ID: "wa_default", Name: "Primary Number", Channel: "whatsapp", Token: maskKey(waToken),
				PhoneID: waPhone, Enabled: true, BoundAgentIDs: []string{"*"},
			}}
		}
	}

	wh := readKey("webhook.secret")
	if wh == "" {
		wh = "acton_sec_89fa2bc4d1"
	}

	s.respondJSON(w, http.StatusOK, ChannelConfigResponse{
		Telegram:      tgAccounts,
		Discord:       dcAccounts,
		WhatsApp:      waAccounts,
		WebhookSecret: wh,
		WebhookURL:    "/api/webhooks/inbound",
	})
}

// handleListAllChannelAccounts returns a flat list of all channel accounts across all channels.
func (s *Server) handleListAllChannelAccounts(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	var all []channels.ChannelAccount

	tg := loadChannelAccounts(configDir, "telegram")
	dc := loadChannelAccounts(configDir, "discord")
	wa := loadChannelAccounts(configDir, "whatsapp")

	all = append(all, tg...)
	all = append(all, dc...)
	all = append(all, wa...)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"accounts": all,
		"count":    len(all),
	})
}

// loadChannelAccounts reads the multi-account JSON file for a channel type.
func loadChannelAccounts(configDir, channelType string) []channels.ChannelAccount {
	data, err := os.ReadFile(filepath.Join(configDir, channelType+"_accounts.json"))
	if err != nil {
		return nil
	}
	var accounts []channels.ChannelAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil
	}
	// Mask tokens for API responses
	for i := range accounts {
		accounts[i].Channel = channelType
		if accounts[i].Token != "" {
			accounts[i].Token = maskKey(accounts[i].Token)
		}
	}
	return accounts
}

// saveChannelAccounts writes the multi-account JSON file for a channel type.
func saveChannelAccounts(configDir, channelType string, accounts []channels.ChannelAccount) error {
	// Preserve actual tokens if masked was passed
	existing := loadRawChannelAccounts(configDir, channelType)
	existingMap := make(map[string]string)
	for _, e := range existing {
		existingMap[e.ID] = e.Token
	}

	for i := range accounts {
		accounts[i].Channel = channelType
		if strings.Contains(accounts[i].Token, "•") || strings.Contains(accounts[i].Token, "...") {
			if realTok, ok := existingMap[accounts[i].ID]; ok && realTok != "" {
				accounts[i].Token = realTok
			}
		}
	}

	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, channelType+"_accounts.json"), data, 0600)
}

func loadRawChannelAccounts(configDir, channelType string) []channels.ChannelAccount {
	data, err := os.ReadFile(filepath.Join(configDir, channelType+"_accounts.json"))
	if err != nil {
		return nil
	}
	var accounts []channels.ChannelAccount
	_ = json.Unmarshal(data, &accounts)
	return accounts
}

func (s *Server) handleSaveChannels(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Join(s.dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)

	var req struct {
		TelegramToken    string                    `json:"telegram_token,omitempty"`
		DiscordToken     string                    `json:"discord_token,omitempty"`
		WhatsAppToken    string                    `json:"whatsapp_token,omitempty"`
		WhatsAppPhone    string                    `json:"whatsapp_phone_id,omitempty"`
		WebhookSecret    string                    `json:"webhook_secret,omitempty"`
		TelegramAccounts []channels.ChannelAccount `json:"telegram_accounts,omitempty"`
		DiscordAccounts  []channels.ChannelAccount `json:"discord_accounts,omitempty"`
		WhatsAppAccounts []channels.ChannelAccount `json:"whatsapp_accounts,omitempty"`
	}

	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	var allAccounts []channels.ChannelAccount

	// Handle multi-account saves
	if req.TelegramAccounts != nil {
		_ = saveChannelAccounts(configDir, "telegram", req.TelegramAccounts)
		rawTg := loadRawChannelAccounts(configDir, "telegram")
		allAccounts = append(allAccounts, rawTg...)
		foundActive := false
		for _, acc := range rawTg {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				foundActive = true
				break
			}
		}
		if !foundActive {
			_ = os.Remove(filepath.Join(configDir, "telegram.token"))
		}
	} else {
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, "telegram")...)
	}

	if req.DiscordAccounts != nil {
		_ = saveChannelAccounts(configDir, "discord", req.DiscordAccounts)
		rawDc := loadRawChannelAccounts(configDir, "discord")
		allAccounts = append(allAccounts, rawDc...)
		foundActive := false
		for _, acc := range rawDc {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "discord.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				foundActive = true
				break
			}
		}
		if !foundActive {
			_ = os.Remove(filepath.Join(configDir, "discord.token"))
		}
	} else {
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, "discord")...)
	}

	if req.WhatsAppAccounts != nil {
		_ = saveChannelAccounts(configDir, "whatsapp", req.WhatsAppAccounts)
		rawWa := loadRawChannelAccounts(configDir, "whatsapp")
		allAccounts = append(allAccounts, rawWa...)
		foundActive := false
		for _, acc := range rawWa {
			if acc.Enabled && acc.Token != "" {
				_ = os.WriteFile(filepath.Join(configDir, "whatsapp.token"), []byte(strings.TrimSpace(acc.Token)), 0600)
				if acc.PhoneID != "" {
					_ = os.WriteFile(filepath.Join(configDir, "whatsapp.phone_id"), []byte(strings.TrimSpace(acc.PhoneID)), 0600)
				}
				foundActive = true
				break
			}
		}
		if !foundActive {
			_ = os.Remove(filepath.Join(configDir, "whatsapp.token"))
			_ = os.Remove(filepath.Join(configDir, "whatsapp.phone_id"))
		}
	} else {
		allAccounts = append(allAccounts, loadRawChannelAccounts(configDir, "whatsapp")...)
	}

	// Legacy single-token save (backward compat)
	if req.TelegramToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "telegram.token"), []byte(strings.TrimSpace(req.TelegramToken)), 0600)
		if s.tgAdapter != nil {
			_ = s.tgAdapter.RestartWithToken(req.TelegramToken)
		}
	}
	if req.DiscordToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "discord.token"), []byte(strings.TrimSpace(req.DiscordToken)), 0600)
	}
	if req.WhatsAppToken != "" {
		_ = os.WriteFile(filepath.Join(configDir, "whatsapp.token"), []byte(strings.TrimSpace(req.WhatsAppToken)), 0600)
	}
	if req.WhatsAppPhone != "" {
		_ = os.WriteFile(filepath.Join(configDir, "whatsapp.phone_id"), []byte(strings.TrimSpace(req.WhatsAppPhone)), 0600)
	}
	if req.WebhookSecret != "" {
		_ = os.WriteFile(filepath.Join(configDir, "webhook.secret"), []byte(strings.TrimSpace(req.WebhookSecret)), 0600)
	}

	// Dynamically sync all active accounts with ChannelManager
	if s.channelMgr != nil {
		_ = s.channelMgr.SyncAccounts(r.Context(), allAccounts)
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// Channel Pairing Handlers
func (s *Server) handleGeneratePairingCode(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}

	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil || req.ChannelID == "" {
		req.ChannelID = "telegram"
	}

	code, err := s.pairingMgr.GeneratePairingCode(req.ChannelID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CODE_GEN_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"code":       code,
		"channel_id": req.ChannelID,
		"expires_in": 600, // 10 minutes
	})
}

func (s *Server) handleVerifyPairingCode(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}

	var req struct {
		ChannelID  string `json:"channel_id"`
		Code       string `json:"code"`
		SenderID   string `json:"sender_id"`
		SenderName string `json:"sender_name"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	ok, err := s.pairingMgr.ValidateAndPair(req.ChannelID, req.Code, req.SenderID, req.SenderName)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "PAIRING_FAILED", err.Error())
		return
	}

	if !ok {
		s.respondError(w, http.StatusBadRequest, "INVALID_CODE", "pairing code is invalid or expired")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "paired",
		"sender": req.SenderID,
	})
}

func (s *Server) handleListAuthorizations(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"users": []any{}, "count": 0})
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	users := s.pairingMgr.ListAuthorized(channelID)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"users": users,
		"count": len(users),
	})
}

func (s *Server) handleRevokeAuthorization(w http.ResponseWriter, r *http.Request) {
	if s.pairingMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PAIRING_NOT_AVAILABLE", "pairing manager not configured")
		return
	}

	var req struct {
		ChannelID string `json:"channel_id"`
		SenderID  string `json:"sender_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.pairingMgr.RevokeUser(req.ChannelID, req.SenderID); err != nil {
		s.respondError(w, http.StatusBadRequest, "REVOKE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// WhatsApp Webhook Handlers
func (s *Server) handleWhatsAppVerifyWebhook(w http.ResponseWriter, r *http.Request) {
	if s.waAdapter == nil {
		http.Error(w, "whatsapp adapter not configured", http.StatusNotImplemented)
		return
	}

	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if res, ok := s.waAdapter.VerifyWebhook(mode, token, challenge); ok {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(res))
		return
	}

	http.Error(w, "forbidden", http.StatusForbidden)
}

func (s *Server) handleWhatsAppInboundWebhook(w http.ResponseWriter, r *http.Request) {
	if s.waAdapter == nil {
		http.Error(w, "whatsapp adapter not configured", http.StatusNotImplemented)
		return
	}

	rawData, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_BODY_FAILED", err.Error())
		return
	}
	defer r.Body.Close()

	if err := s.waAdapter.HandleInboundPayload(r.Context(), rawData); err != nil {
		s.respondError(w, http.StatusBadRequest, "PAYLOAD_PROCESSING_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

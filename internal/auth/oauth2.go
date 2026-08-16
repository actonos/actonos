package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrTokenExchange = errors.New("oauth token exchange failed")
	ErrTokenRefresh  = errors.New("oauth token refresh failed")
)

// GenerateCodeVerifier generates an RFC 7636 compliant cryptographically random code_verifier (43-128 chars).
func GenerateCodeVerifier() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating random verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GenerateCodeChallenge computes code_challenge = BASE64URL-ENCODE(SHA256(code_verifier)).
func GenerateCodeChallenge(codeVerifier string) string {
	hash := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// VerifyPKCE validates that a given code_verifier hashes to the expected code_challenge (S256).
func VerifyPKCE(codeVerifier, expectedChallenge string) bool {
	calculated := GenerateCodeChallenge(codeVerifier)
	return subtle.ConstantTimeCompare([]byte(calculated), []byte(expectedChallenge)) == 1
}

// OAuthProviderConfig holds endpoints and credentials for an OAuth 2.1 SaaS provider.
type OAuthProviderConfig struct {
	ProviderID   string   `json:"provider_id"`
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes"`
}

// TokenResponse contains OAuth 2.1 token credentials.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// OAuthEngine provides OAuth 2.1 PKCE authorization flow management.
type OAuthEngine struct {
	httpClient *http.Client
	stateStore *StateStore
}

// NewOAuthEngine creates an OAuthEngine instance.
func NewOAuthEngine(stateStore *StateStore) *OAuthEngine {
	return &OAuthEngine{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		stateStore: stateStore,
	}
}

// BuildAuthURL constructs a standard OAuth 2.1 PKCE authorization URL.
func (e *OAuthEngine) BuildAuthURL(
	cfg OAuthProviderConfig,
	redirectURI string,
	scopes []string,
) (authURL string, state string, codeVerifier string, err error) {
	codeVerifier, err = GenerateCodeVerifier()
	if err != nil {
		return "", "", "", err
	}

	codeChallenge := GenerateCodeChallenge(codeVerifier)

	state, err = e.stateStore.Save(cfg.ProviderID, codeVerifier, redirectURI)
	if err != nil {
		return "", "", "", err
	}

	if len(scopes) == 0 {
		scopes = cfg.Scopes
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(scopes, " "))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("access_type", "offline") // Ensure refresh token is issued
	params.Set("prompt", "consent")

	parsedURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", "", "", fmt.Errorf("parsing auth url: %w", err)
	}

	parsedURL.RawQuery = params.Encode()
	return parsedURL.String(), state, codeVerifier, nil
}

// ExchangeCode exchanges the authorization code and verifier for access & refresh tokens.
func (e *OAuthEngine) ExchangeCode(
	ctx context.Context,
	cfg OAuthProviderConfig,
	code string,
	redirectURI string,
	codeVerifier string,
) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenExchange, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d: %s", ErrTokenExchange, resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response json: %w", err)
	}

	if tokenResp.ExpiresIn > 0 {
		tokenResp.ExpiresAt = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		// Default 1 hour if not specified
		tokenResp.ExpiresAt = time.Now().UTC().Add(1 * time.Hour)
	}

	return &tokenResp, nil
}

// RefreshToken obtains a new access token using a refresh token.
func (e *OAuthEngine) RefreshToken(
	ctx context.Context,
	cfg OAuthProviderConfig,
	refreshToken string,
) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", cfg.ClientID)
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenRefresh, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d: %s", ErrTokenRefresh, resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing refresh response json: %w", err)
	}

	if tokenResp.RefreshToken == "" {
		// Retain previous refresh token if new one wasn't rotated
		tokenResp.RefreshToken = refreshToken
	}

	if tokenResp.ExpiresIn > 0 {
		tokenResp.ExpiresAt = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		tokenResp.ExpiresAt = time.Now().UTC().Add(1 * time.Hour)
	}

	return &tokenResp, nil
}

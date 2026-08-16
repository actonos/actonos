package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ClientMetadata contains RFC 7591 Dynamic Client Registration metadata.
type ClientMetadata struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	CreatedAt               time.Time `json:"created_at"`
}

// DCRManager manages dynamic OAuth 2.1 client registration.
type DCRManager struct {
	mu      sync.RWMutex
	clients map[string]ClientMetadata
}

// NewDCRManager creates a new DCRManager.
func NewDCRManager() *DCRManager {
	return &DCRManager{
		clients: make(map[string]ClientMetadata),
	}
}

// RegisterClient dynamically registers a new OAuth client per RFC 7591.
func (m *DCRManager) RegisterClient(ctx context.Context, meta ClientMetadata) (*ClientMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(meta.RedirectURIs) == 0 {
		return nil, errors.New("redirect_uris is required")
	}

	clientIDBytes := make([]byte, 16)
	_, _ = rand.Read(clientIDBytes)
	meta.ClientID = "acton_client_" + hex.EncodeToString(clientIDBytes)

	clientSecretBytes := make([]byte, 32)
	_, _ = rand.Read(clientSecretBytes)
	meta.ClientSecret = hex.EncodeToString(clientSecretBytes)

	meta.CreatedAt = time.Now().UTC()
	if len(meta.GrantTypes) == 0 {
		meta.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
	if len(meta.ResponseTypes) == 0 {
		meta.ResponseTypes = []string{"code"}
	}
	if meta.TokenEndpointAuthMethod == "" {
		meta.TokenEndpointAuthMethod = "client_secret_post"
	}

	m.clients[meta.ClientID] = meta
	return &meta, nil
}

// GetClient retrieves registered client metadata.
func (m *DCRManager) GetClient(clientID string) (*ClientMetadata, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[clientID]
	return &c, ok
}

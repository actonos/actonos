package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrInvalidState = errors.New("invalid or expired oauth state")
)

// StateSession holds contextual data during an in-flight OAuth flow.
type StateSession struct {
	State        string    `json:"state"`
	Provider     string    `json:"provider"`
	CodeVerifier string    `json:"code_verifier"`
	RedirectURI  string    `json:"redirect_uri"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// StateStore is a thread-safe in-memory store for active PKCE state tokens with TTL cleanup.
type StateStore struct {
	mu       sync.Mutex
	sessions map[string]StateSession
	ttl      time.Duration
}

// NewStateStore initializes a StateStore with a given TTL (e.g. 10 minutes).
func NewStateStore(ttl time.Duration) *StateStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	store := &StateStore{
		sessions: make(map[string]StateSession),
		ttl:      ttl,
	}

	// Background cleanup goroutine
	go store.cleanupLoop()

	return store
}

// GenerateState generates a random 32-character hex state token.
func GenerateState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Save stores a PKCE session with expiration.
func (s *StateStore) Save(provider, codeVerifier, redirectURI string) (string, error) {
	state, err := GenerateState()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[state] = StateSession{
		State:        state,
		Provider:     provider,
		CodeVerifier: codeVerifier,
		RedirectURI:  redirectURI,
		ExpiresAt:    time.Now().UTC().Add(s.ttl),
	}

	return state, nil
}

// Consume verifies and removes a state session to prevent replay attacks.
func (s *StateStore) Consume(state string) (*StateSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[state]
	if !exists {
		return nil, ErrInvalidState
	}

	delete(s.sessions, state)

	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, ErrInvalidState
	}

	return &session, nil
}

func (s *StateStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now().UTC()
		for k, v := range s.sessions {
			if now.After(v.ExpiresAt) {
				delete(s.sessions, k)
			}
		}
		s.mu.Unlock()
	}
}

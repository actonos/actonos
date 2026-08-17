package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNotInitialized       = errors.New("system authentication is not initialized")
	ErrAlreadyInitialized   = errors.New("system is already initialized")
	ErrInvalidCredentials   = errors.New("invalid administrator password")
	ErrInvalidToken         = errors.New("invalid or expired session token")
	ErrPasswordTooShort     = errors.New("password must be at least 4 characters long")
)

type SessionInfo struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// SystemAuthManager manages administrator authentication, session tokens, and initial onboarding state.
type SystemAuthManager struct {
	db       *sql.DB
	mu       sync.RWMutex
	sessions map[string]SessionInfo
}

// NewSystemAuthManager creates a new SystemAuthManager instance.
func NewSystemAuthManager(db *sql.DB) *SystemAuthManager {
	return &SystemAuthManager{
		db:       db,
		sessions: make(map[string]SessionInfo),
	}
}

// IsInitialized returns true if the system has completed first-time setup and an admin password exists.
func (m *SystemAuthManager) IsInitialized(ctx context.Context) (bool, error) {
	if m.db == nil {
		return false, nil
	}

	var isInit bool
	query := `SELECT is_initialized FROM system_auth WHERE id = 'admin' LIMIT 1`
	err := m.db.QueryRowContext(ctx, query).Scan(&isInit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return isInit, nil
}

// SetupAdmin completes the initial onboarding by configuring the master administrator password.
func (m *SystemAuthManager) SetupAdmin(ctx context.Context, password string) (string, error) {
	if len(password) < 4 {
		return "", ErrPasswordTooShort
	}

	isInit, err := m.IsInitialized(ctx)
	if err != nil {
		return "", err
	}
	if isInit {
		return "", ErrAlreadyInitialized
	}

	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("failed to generate cryptographic salt: %w", err)
	}
	saltHex := hex.EncodeToString(saltBytes)
	hashHex := hashPassword(password, saltHex)

	now := time.Now().UTC()
	query := `
		INSERT INTO system_auth (id, password_hash, salt, is_initialized, created_at, updated_at)
		VALUES ('admin', ?, ?, 1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			password_hash = excluded.password_hash,
			salt = excluded.salt,
			is_initialized = 1,
			updated_at = excluded.updated_at
	`
	if _, err := m.db.ExecContext(ctx, query, hashHex, saltHex, now, now); err != nil {
		return "", fmt.Errorf("failed to save admin credentials: %w", err)
	}

	return m.generateSession()
}

// Login verifies the administrator password and returns a secure session token.
func (m *SystemAuthManager) Login(ctx context.Context, password string) (string, error) {
	if m.db == nil {
		return "", ErrNotInitialized
	}

	var storedHash, storedSalt string
	var isInit bool
	query := `SELECT password_hash, salt, is_initialized FROM system_auth WHERE id = 'admin' LIMIT 1`
	err := m.db.QueryRowContext(ctx, query).Scan(&storedHash, &storedSalt, &isInit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || !isInit {
			return "", ErrNotInitialized
		}
		return "", err
	}

	testHash := hashPassword(password, storedSalt)
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(testHash)) != 1 {
		return "", ErrInvalidCredentials
	}

	return m.generateSession()
}

// ValidateToken returns true if the token is valid and unexpired.
func (m *SystemAuthManager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}

	m.mu.RLock()
	session, exists := m.sessions[token]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
		return false
	}

	return true
}

// Logout removes an active session token.
func (m *SystemAuthManager) Logout(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

// ChangePassword updates the administrator password after validating current password.
func (m *SystemAuthManager) ChangePassword(ctx context.Context, currentPassword, newPassword string) error {
	if len(newPassword) < 4 {
		return ErrPasswordTooShort
	}

	var storedHash, storedSalt string
	query := `SELECT password_hash, salt FROM system_auth WHERE id = 'admin' LIMIT 1`
	err := m.db.QueryRowContext(ctx, query).Scan(&storedHash, &storedSalt)
	if err != nil {
		return err
	}

	testHash := hashPassword(currentPassword, storedSalt)
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(testHash)) != 1 {
		return ErrInvalidCredentials
	}

	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return err
	}
	newSaltHex := hex.EncodeToString(saltBytes)
	newHashHex := hashPassword(newPassword, newSaltHex)

	now := time.Now().UTC()
	updateQuery := `UPDATE system_auth SET password_hash = ?, salt = ?, updated_at = ? WHERE id = 'admin'`
	_, err = m.db.ExecContext(ctx, updateQuery, newHashHex, newSaltHex, now)
	return err
}

func (m *SystemAuthManager) generateSession() (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now().UTC()
	session := SessionInfo{
		Token:     token,
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour), // 7-day session validity
	}

	m.mu.Lock()
	m.sessions[token] = session
	m.mu.Unlock()

	return token, nil
}

func hashPassword(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(password))
	h.Write([]byte(salt))
	return hex.EncodeToString(h.Sum(nil))
}

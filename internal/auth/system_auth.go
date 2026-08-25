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
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMinLen       = 8
	loginFailLimit       = 5
	loginFailWindow      = 15 * time.Minute
	argonPasswordTime    = 1
	argonPasswordMemKB   = 32 * 1024
	argonPasswordThreads = 4
	argonPasswordKeyLen  = 32
)

var (
	ErrNotInitialized     = errors.New("system authentication is not initialized")
	ErrAlreadyInitialized = errors.New("system is already initialized")
	ErrInvalidCredentials = errors.New("invalid administrator password")
	ErrInvalidToken       = errors.New("invalid or expired session token")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters long")
	ErrTooManyAttempts    = errors.New("too many failed login attempts")
)

type SessionInfo struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// SystemAuthManager manages administrator authentication, session tokens, and initial onboarding state.
type SystemAuthManager struct {
	db          *sql.DB
	mu          sync.RWMutex
	sessions    map[string]SessionInfo
	failMu      sync.Mutex
	failCount   int
	failWindow  time.Time
	lockedUntil time.Time
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
	if len(password) < passwordMinLen {
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
	if err := m.checkLoginLock(); err != nil {
		return "", err
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

	if !passwordMatches(password, storedHash, storedSalt) {
		m.recordLoginFailure()
		return "", ErrInvalidCredentials
	}
	m.clearLoginFailures()

	if isLegacyPasswordHash(storedHash) {
		_ = m.persistPassword(ctx, password)
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
	if len(newPassword) < passwordMinLen {
		return ErrPasswordTooShort
	}

	var storedHash, storedSalt string
	query := `SELECT password_hash, salt FROM system_auth WHERE id = 'admin' LIMIT 1`
	err := m.db.QueryRowContext(ctx, query).Scan(&storedHash, &storedSalt)
	if err != nil {
		return err
	}

	if !passwordMatches(currentPassword, storedHash, storedSalt) {
		return ErrInvalidCredentials
	}

	return m.persistPassword(ctx, newPassword)
}

func (m *SystemAuthManager) persistPassword(ctx context.Context, password string) error {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return err
	}
	saltHex := hex.EncodeToString(saltBytes)
	hash := hashPassword(password, saltHex)
	now := time.Now().UTC()
	_, err := m.db.ExecContext(ctx, `UPDATE system_auth SET password_hash = ?, salt = ?, updated_at = ? WHERE id = 'admin'`, hash, saltHex, now)
	return err
}

func (m *SystemAuthManager) checkLoginLock() error {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	now := time.Now().UTC()
	if now.Before(m.lockedUntil) {
		return ErrTooManyAttempts
	}
	if !m.failWindow.IsZero() && now.Sub(m.failWindow) > loginFailWindow {
		m.failCount = 0
		m.failWindow = time.Time{}
	}
	return nil
}

func (m *SystemAuthManager) recordLoginFailure() {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	now := time.Now().UTC()
	if m.failWindow.IsZero() || now.Sub(m.failWindow) > loginFailWindow {
		m.failWindow = now
		m.failCount = 0
	}
	m.failCount++
	if m.failCount >= loginFailLimit {
		m.lockedUntil = now.Add(loginFailWindow)
	}
}

func (m *SystemAuthManager) clearLoginFailures() {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	m.failCount = 0
	m.failWindow = time.Time{}
	m.lockedUntil = time.Time{}
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

func hashPassword(password, saltHex string) string {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		salt = []byte(saltHex)
	}
	key := argon2.IDKey([]byte(password), salt, argonPasswordTime, argonPasswordMemKB, argonPasswordThreads, argonPasswordKeyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonPasswordMemKB, argonPasswordTime, argonPasswordThreads, saltHex, hex.EncodeToString(key))
}

func legacySHA256Hash(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(password))
	h.Write([]byte(salt))
	return hex.EncodeToString(h.Sum(nil))
}

func isLegacyPasswordHash(stored string) bool {
	return !strings.HasPrefix(stored, "argon2id$")
}

func passwordMatches(password, storedHash, salt string) bool {
	var candidate string
	if isLegacyPasswordHash(storedHash) {
		candidate = legacySHA256Hash(password, salt)
	} else {
		candidate = hashPassword(password, salt)
	}
	if len(candidate) != len(storedHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(storedHash)) == 1
}

package channels

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// AuthorizedUser represents a validated user permitted to interact via a channel.
type AuthorizedUser struct {
	ChannelID    string    `json:"channel_id"`
	SenderID     string    `json:"sender_id"`
	SenderName   string    `json:"sender_name"`
	PairedAt     time.Time `json:"paired_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	Status       string    `json:"status"` // "active", "revoked"
}

// PairingRequest tracks an unverified user attempting to connect.
type PairingRequest struct {
	Code      string    `json:"code"`
	ChannelID string    `json:"channel_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PairingManager coordinates zero-trust device/channel authorization.
type PairingManager struct {
	mu          sync.RWMutex
	db          *sql.DB
	activeCodes map[string]PairingRequest // code -> request
	users       map[string]AuthorizedUser // "channel:sender" -> user
}

// NewPairingManager creates a new PairingManager.
func NewPairingManager(db *sql.DB) (*PairingManager, error) {
	pm := &PairingManager{
		db:          db,
		activeCodes: make(map[string]PairingRequest),
		users:       make(map[string]AuthorizedUser),
	}

	if db != nil {
		if err := pm.initSchema(); err != nil {
			return nil, err
		}
		if err := pm.loadUsers(); err != nil {
			return nil, err
		}
	}

	return pm, nil
}

func (pm *PairingManager) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS channel_authorizations (
		channel_id TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		sender_name TEXT NOT NULL,
		paired_at DATETIME NOT NULL,
		last_active_at DATETIME NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		PRIMARY KEY(channel_id, sender_id)
	);
	`
	_, err := pm.db.Exec(schema)
	return err
}

func (pm *PairingManager) loadUsers() error {
	rows, err := pm.db.Query(`
		SELECT channel_id, sender_id, sender_name, paired_at, last_active_at, status
		FROM channel_authorizations
		WHERE status = 'active'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for rows.Next() {
		var u AuthorizedUser
		if err := rows.Scan(&u.ChannelID, &u.SenderID, &u.SenderName, &u.PairedAt, &u.LastActiveAt, &u.Status); err == nil {
			key := fmt.Sprintf("%s:%s", u.ChannelID, u.SenderID)
			pm.users[key] = u
		}
	}
	return nil
}

// GeneratePairingCode generates a 6-digit numeric PIN with a 10-minute expiry.
func (pm *PairingManager) GeneratePairingCode(channelID string) (string, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clean up expired codes
	now := time.Now().UTC()
	for code, req := range pm.activeCodes {
		if now.After(req.ExpiresAt) {
			delete(pm.activeCodes, code)
		}
	}

	// Generate random 6-digit PIN
	nBig, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", nBig.Int64()+100000)

	pm.activeCodes[code] = PairingRequest{
		Code:      code,
		ChannelID: channelID,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}

	return code, nil
}

// ValidateAndPair matches an incoming code from a chat user and authorizes them.
func (pm *PairingManager) ValidateAndPair(channelID, code, senderID, senderName string) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, exists := pm.activeCodes[code]
	if !exists {
		return false, nil
	}

	if req.ChannelID != "" && req.ChannelID != channelID {
		return false, nil
	}

	if time.Now().UTC().After(req.ExpiresAt) {
		delete(pm.activeCodes, code)
		return false, nil
	}

	// Code matched — consume code
	delete(pm.activeCodes, code)

	now := time.Now().UTC()
	user := AuthorizedUser{
		ChannelID:    channelID,
		SenderID:     senderID,
		SenderName:   senderName,
		PairedAt:     now,
		LastActiveAt: now,
		Status:       "active",
	}

	key := fmt.Sprintf("%s:%s", channelID, senderID)
	pm.users[key] = user

	if pm.db != nil {
		_, err := pm.db.Exec(`
			INSERT INTO channel_authorizations (channel_id, sender_id, sender_name, paired_at, last_active_at, status)
			VALUES (?, ?, ?, ?, ?, 'active')
			ON CONFLICT(channel_id, sender_id) DO UPDATE SET
				sender_name = excluded.sender_name,
				last_active_at = excluded.last_active_at,
				status = 'active'
		`, channelID, senderID, senderName, now, now)
		if err != nil {
			return true, err
		}
	}

	return true, nil
}

// IsAuthorized checks if the given sender is allowed on this channel.
func (pm *PairingManager) IsAuthorized(channelID, senderID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", channelID, senderID)
	u, exists := pm.users[key]
	return exists && u.Status == "active"
}

// TouchUser updates the last active timestamp for an authorized user.
func (pm *PairingManager) TouchUser(channelID, senderID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := fmt.Sprintf("%s:%s", channelID, senderID)
	if u, exists := pm.users[key]; exists {
		u.LastActiveAt = time.Now().UTC()
		pm.users[key] = u

		if pm.db != nil {
			go func() {
				_, _ = pm.db.Exec(`
					UPDATE channel_authorizations
					SET last_active_at = ?
					WHERE channel_id = ? AND sender_id = ?
				`, u.LastActiveAt, channelID, senderID)
			}()
		}
	}
}

// ListAuthorized returns all authorized users for a given channel or all channels.
func (pm *PairingManager) ListAuthorized(channelID string) []AuthorizedUser {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []AuthorizedUser
	for _, u := range pm.users {
		if u.Status != "active" {
			continue
		}
		if channelID == "" || u.ChannelID == channelID {
			result = append(result, u)
		}
	}
	return result
}

// RevokeUser removes access for a sender on a channel.
func (pm *PairingManager) RevokeUser(channelID, senderID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := fmt.Sprintf("%s:%s", channelID, senderID)
	delete(pm.users, key)

	if pm.db != nil {
		_, err := pm.db.Exec(`
			UPDATE channel_authorizations
			SET status = 'revoked'
			WHERE channel_id = ? AND sender_id = ?
		`, channelID, senderID)
		return err
	}
	return nil
}

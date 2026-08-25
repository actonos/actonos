package channels

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

const (
	pairingCodeLength   = 8
	pairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	pairingFailLimit    = 5
	pairingFailWindow   = 15 * time.Minute
)

// ErrUnpairedSender is returned when pairing is required and the sender is not authorized.
var ErrUnpairedSender = errors.New("unpaired sender")

// ErrPairingLocked is returned after too many failed pairing attempts.
var ErrPairingLocked = errors.New("too many failed pairing attempts")

// UnpairedSenderError carries the inbound identity so the router can reply in-channel.
type UnpairedSenderError struct {
	ChannelID  string
	SenderID   string
	SenderName string
}

func (e *UnpairedSenderError) Error() string {
	if e == nil {
		return ErrUnpairedSender.Error()
	}
	return fmt.Sprintf("unpaired sender %s on channel %s", e.SenderID, e.ChannelID)
}

func (e *UnpairedSenderError) Unwrap() error { return ErrUnpairedSender }

// PendingSender is an unknown chat user waiting for a pairing code or operator allow.
type PendingSender struct {
	ChannelID   string    `json:"channel_id"`
	SenderID    string    `json:"sender_id"`
	SenderName  string    `json:"sender_name"`
	LastContent string    `json:"last_content,omitempty"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

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
	policies    map[string]bool           // channel_id -> pairing required
	pending     map[string]PendingSender  // "channel:sender" -> waiting user
	failCount   map[string]int            // "channel:sender" -> consecutive failed PIN tries
	failWindow  map[string]time.Time
}

// NewPairingManager creates a new PairingManager.
func NewPairingManager(db *sql.DB) (*PairingManager, error) {
	pm := &PairingManager{
		db:          db,
		activeCodes: make(map[string]PairingRequest),
		users:       make(map[string]AuthorizedUser),
		policies:    make(map[string]bool),
		pending:     make(map[string]PendingSender),
		failCount:   make(map[string]int),
		failWindow:  make(map[string]time.Time),
	}

	if db != nil {
		if err := pm.initSchema(); err != nil {
			return nil, err
		}
		if err := pm.loadUsers(); err != nil {
			return nil, err
		}
		if err := pm.loadPolicies(); err != nil {
			return nil, err
		}
		if err := pm.loadCodes(); err != nil {
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
	CREATE TABLE IF NOT EXISTS channel_pairing_policy (
		channel_id TEXT PRIMARY KEY,
		required INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS channel_pairing_codes (
		code TEXT PRIMARY KEY,
		channel_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL
	);
	`
	_, err := pm.db.Exec(schema)
	return err
}

func (pm *PairingManager) loadPolicies() error {
	rows, err := pm.db.Query(`SELECT channel_id, required FROM channel_pairing_policy`)
	if err != nil {
		return err
	}
	defer rows.Close()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for rows.Next() {
		var channelID string
		var required int
		if err := rows.Scan(&channelID, &required); err == nil && channelID != "" {
			pm.policies[strings.ToLower(channelID)] = required != 0
		}
	}
	return nil
}

func (pm *PairingManager) loadCodes() error {
	now := time.Now().UTC()
	_, _ = pm.db.Exec(`DELETE FROM channel_pairing_codes WHERE expires_at <= ?`, now)
	rows, err := pm.db.Query(`SELECT code, channel_id, created_at, expires_at FROM channel_pairing_codes WHERE expires_at > ?`, now)
	if err != nil {
		return err
	}
	defer rows.Close()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for rows.Next() {
		var req PairingRequest
		if err := rows.Scan(&req.Code, &req.ChannelID, &req.CreatedAt, &req.ExpiresAt); err == nil && req.Code != "" {
			pm.activeCodes[req.Code] = req
		}
	}
	return rows.Err()
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
			key := pairingKey(u.ChannelID, u.SenderID)
			pm.users[key] = u
		}
	}
	return nil
}

func generatePairingCode() (string, error) {
	out := make([]byte, pairingCodeLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(pairingCodeAlphabet))))
		if err != nil {
			return "", err
		}
		out[i] = pairingCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

// GeneratePairingCode generates an 8-character pairing code with a 10-minute expiry.
// It does not change pairing policy; the operator must enable pairing separately.
func (pm *PairingManager) GeneratePairingCode(channelID string) (string, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now().UTC()
	for code, req := range pm.activeCodes {
		if now.After(req.ExpiresAt) {
			delete(pm.activeCodes, code)
		}
	}

	code, err := generatePairingCode()
	if err != nil {
		return "", err
	}

	channelID = strings.ToLower(strings.TrimSpace(channelID))
	if channelID == "" {
		channelID = "all"
	}
	req := PairingRequest{
		Code:      code,
		ChannelID: channelID,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}
	pm.activeCodes[code] = req
	if pm.db != nil {
		_, _ = pm.db.Exec(`
			INSERT INTO channel_pairing_codes (code, channel_id, created_at, expires_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				channel_id = excluded.channel_id,
				created_at = excluded.created_at,
				expires_at = excluded.expires_at
		`, code, channelID, now, req.ExpiresAt)
	}

	return code, nil
}

// ValidateAndPair matches an incoming code from a chat user and authorizes them.
func pairingKey(channelID, senderID string) string {
	return strings.ToLower(strings.TrimSpace(channelID)) + ":" + strings.TrimSpace(senderID)
}

func (pm *PairingManager) ValidateAndPair(channelID, code, senderID, senderName string) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	channelID = strings.ToLower(strings.TrimSpace(channelID))
	code = strings.ToUpper(strings.TrimSpace(code))
	failKey := pairingKey(channelID, senderID)
	now := time.Now().UTC()
	if started, ok := pm.failWindow[failKey]; !ok || now.Sub(started) > pairingFailWindow {
		pm.failCount[failKey] = 0
		pm.failWindow[failKey] = now
	}
	if pm.failCount[failKey] >= pairingFailLimit {
		return false, ErrPairingLocked
	}

	req, exists := pm.activeCodes[code]
	if !exists {
		pm.failCount[failKey]++
		return false, nil
	}

	reqChannel := strings.ToLower(req.ChannelID)
	if reqChannel != "" && reqChannel != "all" && reqChannel != channelID {
		pm.failCount[failKey]++
		return false, nil
	}

	if time.Now().UTC().After(req.ExpiresAt) {
		delete(pm.activeCodes, code)
		pm.failCount[failKey]++
		return false, nil
	}

	delete(pm.activeCodes, code)
	delete(pm.failCount, failKey)
	delete(pm.failWindow, failKey)
	if pm.db != nil {
		_, _ = pm.db.Exec(`DELETE FROM channel_pairing_codes WHERE code = ?`, code)
	}

	now = time.Now().UTC()
	user := AuthorizedUser{
		ChannelID:    channelID,
		SenderID:     senderID,
		SenderName:   senderName,
		PairedAt:     now,
		LastActiveAt: now,
		Status:       "active",
	}

	key := pairingKey(channelID, senderID)
	pm.users[key] = user
	delete(pm.pending, key)

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

	key := pairingKey(channelID, senderID)
	u, exists := pm.users[key]
	return exists && u.Status == "active"
}

// TouchUser updates the last active timestamp for an authorized user.
func (pm *PairingManager) TouchUser(channelID, senderID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := pairingKey(channelID, senderID)
	if u, exists := pm.users[key]; exists {
		u.LastActiveAt = time.Now().UTC()
		pm.users[key] = u

		if pm.db != nil {
			_, _ = pm.db.Exec(`
				UPDATE channel_authorizations
				SET last_active_at = ?
				WHERE channel_id = ? AND sender_id = ?
			`, u.LastActiveAt, channelID, senderID)
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

	key := pairingKey(channelID, senderID)
	delete(pm.users, key)
	delete(pm.pending, key)

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

// ExtractPairingPIN extracts a pairing code only from an explicit "/pair CODE" command.
func ExtractPairingPIN(text string) string {
	trimmed := strings.TrimSpace(text)
	idx := strings.Index(strings.ToLower(trimmed), "/pair")
	if idx == -1 {
		return ""
	}
	after := strings.TrimSpace(trimmed[idx+len("/pair"):])
	fields := strings.Fields(after)
	if len(fields) == 0 {
		return ""
	}
	code := strings.ToUpper(strings.Trim(fields[0], ":,.!?()[]{}'\""))
	if isPairingCode(code) {
		return code
	}
	return ""
}

func isPairingCode(code string) bool {
	if len(code) != pairingCodeLength {
		return false
	}
	for _, c := range code {
		if !strings.ContainsRune(pairingCodeAlphabet, c) {
			return false
		}
	}
	return true
}

// ListActiveCodes returns unexpired pairing PINs for the operator UI.
func (pm *PairingManager) ListActiveCodes() []PairingRequest {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	now := time.Now().UTC()
	out := make([]PairingRequest, 0, len(pm.activeCodes))
	for code, req := range pm.activeCodes {
		if now.After(req.ExpiresAt) {
			delete(pm.activeCodes, code)
			continue
		}
		out = append(out, req)
	}
	return out
}

// ListPolicies returns channel_id -> pairing required.
func (pm *PairingManager) ListPolicies() map[string]bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make(map[string]bool, len(pm.policies))
	for k, v := range pm.policies {
		out[k] = v
	}
	return out
}

// ChannelRequiresPairing reports the persisted policy for a channel type.
func (pm *PairingManager) ChannelRequiresPairing(channelID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.policies[strings.ToLower(strings.TrimSpace(channelID))]
}

// SetChannelRequiresPairing stores whether inbound chat on channelID must pair first.
func (pm *PairingManager) SetChannelRequiresPairing(channelID string, required bool) error {
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	if channelID == "" {
		return errors.New("channel_id is required")
	}
	pm.mu.Lock()
	pm.policies[channelID] = required
	pm.mu.Unlock()
	if pm.db == nil {
		return nil
	}
	flag := 0
	if required {
		flag = 1
	}
	_, err := pm.db.Exec(`
		INSERT INTO channel_pairing_policy (channel_id, required) VALUES (?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET required = excluded.required
	`, channelID, flag)
	return err
}

// NoteUnpaired records an unknown sender so the operator can allow or issue a PIN.
func (pm *PairingManager) NoteUnpaired(channelID, senderID, senderName, content string) {
	if pm == nil || senderID == "" {
		return
	}
	now := time.Now().UTC()
	key := pairingKey(channelID, senderID)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	prev, exists := pm.pending[key]
	if !exists {
		prev.ChannelID = strings.ToLower(strings.TrimSpace(channelID))
		prev.SenderID = senderID
		prev.FirstSeen = now
	}
	prev.SenderName = senderName
	prev.LastContent = strings.TrimSpace(content)
	if len(prev.LastContent) > 240 {
		prev.LastContent = prev.LastContent[:240]
	}
	prev.LastSeen = now
	pm.pending[key] = prev
}

// ListPending returns unpaired senders waiting on the operator.
func (pm *PairingManager) ListPending() []PendingSender {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]PendingSender, 0, len(pm.pending))
	for _, p := range pm.pending {
		out = append(out, p)
	}
	return out
}

// AuthorizeSender lets the operator allow a chat user without a PIN.
func (pm *PairingManager) AuthorizeSender(channelID, senderID, senderName string) error {
	if strings.TrimSpace(channelID) == "" || strings.TrimSpace(senderID) == "" {
		return errors.New("channel_id and sender_id are required")
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	now := time.Now().UTC()
	if senderName == "" {
		senderName = senderID
	}
	user := AuthorizedUser{
		ChannelID: channelID, SenderID: senderID, SenderName: senderName,
		PairedAt: now, LastActiveAt: now, Status: "active",
	}
	key := pairingKey(channelID, senderID)
	pm.users[key] = user
	delete(pm.pending, key)
	if pm.db != nil {
		_, err := pm.db.Exec(`
			INSERT INTO channel_authorizations (channel_id, sender_id, sender_name, paired_at, last_active_at, status)
			VALUES (?, ?, ?, ?, ?, 'active')
			ON CONFLICT(channel_id, sender_id) DO UPDATE SET
				sender_name = excluded.sender_name,
				last_active_at = excluded.last_active_at,
				status = 'active'
		`, channelID, senderID, senderName, now, now)
		return err
	}
	return nil
}

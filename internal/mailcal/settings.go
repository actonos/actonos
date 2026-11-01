package mailcal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SettingsStore keeps a handful of switches. The master switch defaults to OFF: writing
// mail or calendar entries stays disabled until a person turns it on.
type SettingsStore struct{ db *sql.DB }

const settingEnabled = "enabled"

// NewSettingsStore creates the table.
func NewSettingsStore(db *sql.DB) (*SettingsStore, error) {
	if db == nil {
		return nil, errors.New("mailcal: database is required")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mailcal_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return nil, fmt.Errorf("creating mailcal_settings: %w", err)
	}
	return &SettingsStore{db: db}, nil
}

// Enabled reports the master switch. Any read error means "off".
func (s *SettingsStore) Enabled(ctx context.Context) bool {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM mailcal_settings WHERE key = ?`, settingEnabled).Scan(&v); err != nil {
		return false
	}
	return strings.EqualFold(v, "true")
}

// SetEnabled flips the master switch.
func (s *SettingsStore) SetEnabled(ctx context.Context, on bool) error {
	v := "false"
	if on {
		v = "true"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO mailcal_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, settingEnabled, v)
	return err
}

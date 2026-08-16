package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/memory"
)

const (
	// DefaultRefreshBuffer is 5 minutes prior to expiry as defined in the ActonOS architecture.
	DefaultRefreshBuffer = 5 * time.Minute

	// DefaultCheckInterval is how frequently the daemon scans stored tokens.
	DefaultCheckInterval = 60 * time.Second
)

// StoredTokenData contains credentials persisted securely in the Vault.
type StoredTokenData struct {
	Provider     string    `json:"provider"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

// TokenRefreshDaemon periodically scans and renews OAuth tokens before expiry.
type TokenRefreshDaemon struct {
	mu            sync.RWMutex
	oauthEngine   *OAuthEngine
	vault         *memory.Vault
	db            *memory.DB
	bus           *bus.EventBus
	configs       map[string]OAuthProviderConfig
	refreshBuffer time.Duration
	interval      time.Duration
	stopCh        chan struct{}
}

// NewTokenRefreshDaemon creates a new token refresh daemon.
func NewTokenRefreshDaemon(
	engine *OAuthEngine,
	vault *memory.Vault,
	db *memory.DB,
	eventBus *bus.EventBus,
) *TokenRefreshDaemon {
	return &TokenRefreshDaemon{
		oauthEngine:   engine,
		vault:         vault,
		db:            db,
		bus:           eventBus,
		configs:       make(map[string]OAuthProviderConfig),
		refreshBuffer: DefaultRefreshBuffer,
		interval:      DefaultCheckInterval,
		stopCh:        make(chan struct{}),
	}
}

// RegisterProviderConfig configures provider endpoints and client credentials.
func (d *TokenRefreshDaemon) RegisterProviderConfig(cfg OAuthProviderConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.configs[cfg.ProviderID] = cfg
}

// SetRefreshBuffer overrides the default 5-minute pre-expiry buffer (useful in tests).
func (d *TokenRefreshDaemon) SetRefreshBuffer(buffer time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refreshBuffer = buffer
}

// SetCheckInterval overrides the scan loop interval.
func (d *TokenRefreshDaemon) SetCheckInterval(interval time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interval = interval
}

// SaveToken encrypts and saves an OAuth token into the SQLite database.
func (d *TokenRefreshDaemon) SaveToken(ctx context.Context, provider string, token *TokenResponse) error {
	data := StoredTokenData{
		Provider:     provider,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresAt:    token.ExpiresAt,
		Scope:        token.Scope,
	}

	rawJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshalling token data: %w", err)
	}

	encrypted, err := d.vault.Encrypt(rawJSON)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	query := `
		INSERT INTO oauth_tokens (provider, encrypted_data, expires_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			encrypted_data = excluded.encrypted_data,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at
	`
	now := time.Now().UTC()
	_, err = d.db.SQLDB().ExecContext(ctx, query, provider, encrypted, token.ExpiresAt, now)
	return err
}

// GetToken retrieves and decrypts the active token for a provider.
func (d *TokenRefreshDaemon) GetToken(ctx context.Context, provider string) (*StoredTokenData, error) {
	query := `SELECT encrypted_data FROM oauth_tokens WHERE provider = ?`
	row := d.db.SQLDB().QueryRowContext(ctx, query, provider)

	var encrypted string
	if err := row.Scan(&encrypted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no token stored for provider: %s", provider)
		}
		return nil, fmt.Errorf("querying token for %s: %w", provider, err)
	}

	decryptedBytes, err := d.vault.Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypting token for %s: %w", provider, err)
	}

	var data StoredTokenData
	if err := json.Unmarshal(decryptedBytes, &data); err != nil {
		return nil, fmt.Errorf("unmarshalling token data: %w", err)
	}

	return &data, nil
}

// CheckAndRefreshAll scans all stored tokens and refreshes those expiring within refreshBuffer.
func (d *TokenRefreshDaemon) CheckAndRefreshAll(ctx context.Context) int {
	d.mu.RLock()
	buffer := d.refreshBuffer
	d.mu.RUnlock()

	threshold := time.Now().UTC().Add(buffer)

	query := `SELECT provider, encrypted_data, expires_at FROM oauth_tokens WHERE expires_at <= ?`
	rows, err := d.db.SQLDB().QueryContext(ctx, query, threshold)
	if err != nil {
		slog.Error("token refresh daemon: failed to query expiring tokens", "error", err)
		return 0
	}
	defer rows.Close()

	type expiringToken struct {
		provider  string
		encrypted string
		expiresAt time.Time
	}

	var expiring []expiringToken
	for rows.Next() {
		var item expiringToken
		if err := rows.Scan(&item.provider, &item.encrypted, &item.expiresAt); err == nil {
			expiring = append(expiring, item)
		}
	}

	refreshedCount := 0
	for _, item := range expiring {
		decryptedBytes, err := d.vault.Decrypt(item.encrypted)
		if err != nil {
			slog.Error("token refresh daemon: failed to decrypt token", "provider", item.provider, "error", err)
			continue
		}

		var tokenData StoredTokenData
		if err := json.Unmarshal(decryptedBytes, &tokenData); err != nil {
			continue
		}

		if tokenData.RefreshToken == "" {
			slog.Warn("token refresh daemon: no refresh token available", "provider", item.provider)
			if d.bus != nil {
				d.bus.Publish(bus.NewEvent(bus.EventTokenExpired, item.provider, map[string]any{
					"reason": "no_refresh_token",
				}))
			}
			continue
		}

		d.mu.RLock()
		cfg, hasCfg := d.configs[item.provider]
		d.mu.RUnlock()

		if !hasCfg {
			slog.Warn("token refresh daemon: missing provider config for refresh", "provider", item.provider)
			continue
		}

		// Perform token refresh
		newResp, err := d.oauthEngine.RefreshToken(ctx, cfg, tokenData.RefreshToken)
		if err != nil {
			slog.Error("token refresh daemon: refresh request failed", "provider", item.provider, "error", err)
			if d.bus != nil {
				d.bus.Publish(bus.NewEvent(bus.EventTokenFailed, item.provider, err.Error()))
			}
			continue
		}

		// Save updated token
		if err := d.SaveToken(ctx, item.provider, newResp); err != nil {
			slog.Error("token refresh daemon: saving refreshed token failed", "provider", item.provider, "error", err)
			continue
		}

		refreshedCount++
		slog.Info("token refresh daemon: successfully refreshed token",
			"provider", item.provider,
			"new_expires_at", newResp.ExpiresAt,
		)

		if d.bus != nil {
			d.bus.Publish(bus.NewEvent(bus.EventTokenRefreshed, item.provider, map[string]any{
				"expires_at": newResp.ExpiresAt,
			}))
		}
	}

	return refreshedCount
}

// Start launches the background refresh daemon loop.
func (d *TokenRefreshDaemon) Start(ctx context.Context) {
	d.mu.RLock()
	interval := d.interval
	d.mu.RUnlock()

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = d.CheckAndRefreshAll(ctx)
			case <-d.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop terminates the background refresh daemon loop.
func (d *TokenRefreshDaemon) Stop() {
	close(d.stopCh)
}

package memory

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrSecretNotFound = errors.New("secret not found in vault")
	ErrDecryptionFail = errors.New("failed to decrypt vault payload")
)

// Vault provides AES-256-GCM hardware/passphrase bound encryption for sensitive credentials.
type Vault struct {
	db         *DB
	derivedKey []byte
	salt       []byte
}

// NewVault derives a 256-bit AES key from a master passphrase and salt using Argon2id.
func NewVault(db *DB, masterSecret string, salt []byte) (*Vault, error) {
	if len(salt) == 0 {
		// Default stable device salt if not provided
		salt = []byte("actonos-hardware-bound-vault-salt-v1")
	}

	if masterSecret == "" {
		masterSecret = "actonos-default-local-device-key"
	}

	// Derive 32-byte (256-bit) AES key via Argon2id
	// time=1, memory=64MB, threads=4, keyLen=32
	derivedKey := argon2.IDKey([]byte(masterSecret), salt, 1, 64*1024, 4, 32)

	return &Vault{
		db:         db,
		derivedKey: derivedKey,
		salt:       salt,
	}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a fresh random 12-byte nonce.
// Output format: base64(nonce || ciphertext || tag)
func (v *Vault) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(v.derivedKey)
	if err != nil {
		return "", fmt.Errorf("creating aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating random nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64 encoded AES-256-GCM payload.
func (v *Vault) Decrypt(encodedCiphertext string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decoding base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(v.derivedKey)
	if err != nil {
		return nil, fmt.Errorf("creating aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrDecryptionFail
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFail, err)
	}

	return plaintext, nil
}

// SetSecret encrypts and stores a named secret in the vault database.
func (v *Vault) SetSecret(ctx context.Context, keyName string, secretValue string) error {
	encrypted, err := v.Encrypt([]byte(secretValue))
	if err != nil {
		return fmt.Errorf("encrypting secret %s: %w", keyName, err)
	}

	query := `
		INSERT INTO vault_entries (key_name, encrypted_val, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key_name) DO UPDATE SET
			encrypted_val = excluded.encrypted_val,
			updated_at = excluded.updated_at
	`
	_, err = v.db.db.ExecContext(ctx, query, keyName, encrypted, time.Now().UTC())
	return err
}

// GetSecret retrieves and decrypts a named secret from the vault database.
func (v *Vault) GetSecret(ctx context.Context, keyName string) (string, error) {
	query := `SELECT encrypted_val FROM vault_entries WHERE key_name = ?`
	row := v.db.db.QueryRowContext(ctx, query, keyName)

	var encrypted string
	if err := row.Scan(&encrypted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", ErrSecretNotFound, keyName)
		}
		return "", fmt.Errorf("querying secret %s: %w", keyName, err)
	}

	plaintextBytes, err := v.Decrypt(encrypted)
	if err != nil {
		return "", err
	}

	return string(plaintextBytes), nil
}

// VaultSecretMeta represents metadata of a stored encrypted secret.
type VaultSecretMeta struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListSecrets returns the metadata of all secrets currently stored in the vault.
func (v *Vault) ListSecrets(ctx context.Context) ([]VaultSecretMeta, error) {
	if v == nil || v.db == nil || v.db.db == nil {
		return nil, errors.New("vault is unavailable")
	}

	query := `SELECT key_name, updated_at FROM vault_entries ORDER BY key_name ASC`
	rows, err := v.db.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing vault secrets: %w", err)
	}
	defer rows.Close()

	var list []VaultSecretMeta
	for rows.Next() {
		var meta VaultSecretMeta
		if err := rows.Scan(&meta.Name, &meta.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []VaultSecretMeta{}
	}
	return list, nil
}

// DeleteSecret permanently removes a named secret from the encrypted vault.
func (v *Vault) DeleteSecret(ctx context.Context, keyName string) error {
	if v == nil || v.db == nil || v.db.db == nil {
		return errors.New("vault is unavailable")
	}
	if _, err := v.db.db.ExecContext(ctx, `DELETE FROM vault_entries WHERE key_name = ?`, keyName); err != nil {
		return fmt.Errorf("deleting secret %s: %w", keyName, err)
	}
	return nil
}

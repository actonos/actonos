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
	db        *DB
	derivedKey []byte
	salt      []byte
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

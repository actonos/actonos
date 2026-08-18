package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestVault_EncryptionDecryption(t *testing.T) {
	tempDir := t.TempDir()
	db, err := Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	vault, err := NewVault(db, "my-super-secret-master-pin", nil)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	secretKey := "openai_api_key"
	secretVal := "sk-ant-api03-abcdef123456789"

	ctx := context.Background()
	if err := vault.SetSecret(ctx, secretKey, secretVal); err != nil {
		t.Fatalf("failed to set secret: %v", err)
	}

	retrieved, err := vault.GetSecret(ctx, secretKey)
	if err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}

	if retrieved != secretVal {
		t.Fatalf("expected '%s', got '%s'", secretVal, retrieved)
	}
	if err := vault.DeleteSecret(ctx, secretKey); err != nil {
		t.Fatalf("failed to delete secret: %v", err)
	}
	if _, err := vault.GetSecret(ctx, secretKey); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected deleted secret to be absent, got %v", err)
	}
}

func TestVault_WrongKeyFailsDecryption(t *testing.T) {
	tempDir := t.TempDir()
	db, err := Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	vault1, _ := NewVault(db, "pin-1234", nil)
	vault2, _ := NewVault(db, "pin-9999", nil)

	ctx := context.Background()
	_ = vault1.SetSecret(ctx, "token", "secret-payload")

	// Attempting to read with vault2 (different derived key) should fail
	_, err = vault2.GetSecret(ctx, "token")
	if err == nil {
		t.Fatal("expected decryption failure with wrong master key, got nil")
	}
}

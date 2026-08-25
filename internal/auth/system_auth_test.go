package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS system_auth (
		id TEXT PRIMARY KEY,
		password_hash TEXT NOT NULL,
		salt TEXT NOT NULL,
		is_initialized BOOLEAN NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestSystemAuthManager_Flow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	mgr := NewSystemAuthManager(db)
	ctx := context.Background()

	// 1. Initialized check should be false
	isInit, err := mgr.IsInitialized(ctx)
	if err != nil {
		t.Fatalf("IsInitialized error: %v", err)
	}
	if isInit {
		t.Fatalf("expected isInit to be false initially")
	}

	// 2. Setup Admin
	token, err := mgr.SetupAdmin(ctx, "SuperSecret123!")
	if err != nil {
		t.Fatalf("SetupAdmin error: %v", err)
	}
	if token == "" {
		t.Fatalf("expected non-empty session token")
	}

	// 3. Initialized check should now be true
	isInit, err = mgr.IsInitialized(ctx)
	if err != nil || !isInit {
		t.Fatalf("expected isInit to be true, got %v, err: %v", isInit, err)
	}

	// 4. Validate Token
	if !mgr.ValidateToken(token) {
		t.Fatalf("expected token to be valid")
	}
	if mgr.ValidateToken("invalid_token") {
		t.Fatalf("expected invalid token to return false")
	}

	// 5. Login with wrong password
	_, err = mgr.Login(ctx, "WrongPassword")
	if err == nil {
		t.Fatalf("expected login with wrong password to fail")
	}

	// 6. Login with correct password
	newToken, err := mgr.Login(ctx, "SuperSecret123!")
	if err != nil {
		t.Fatalf("login failed with correct password: %v", err)
	}
	if !mgr.ValidateToken(newToken) {
		t.Fatalf("expected newToken to be valid")
	}

	// 7. Logout
	mgr.Logout(newToken)
	if mgr.ValidateToken(newToken) {
		t.Fatalf("expected logged out token to be invalid")
	}

	// 8. Change Password
	err = mgr.ChangePassword(ctx, "SuperSecret123!", "NewSecret456!")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// 9. Login with new password
	changedToken, err := mgr.Login(ctx, "NewSecret456!")
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}
	if !mgr.ValidateToken(changedToken) {
		t.Fatalf("expected changedToken to be valid")
	}
}

func TestSystemAuthManager_PasswordMinLength(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := NewSystemAuthManager(db)
	if _, err := mgr.SetupAdmin(context.Background(), "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected short password error, got %v", err)
	}
}

func TestSystemAuthManager_LoginLockout(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := NewSystemAuthManager(db)
	ctx := context.Background()
	if _, err := mgr.SetupAdmin(ctx, "SuperSecret123!"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < loginFailLimit; i++ {
		_, err := mgr.Login(ctx, "wrong-password")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", i, err)
		}
	}
	_, err := mgr.Login(ctx, "SuperSecret123!")
	if !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("expected lockout, got %v", err)
	}
}

func TestSystemAuthManager_LegacyHashMigration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	salt := "aabbccddeeff00112233445566778899"
	legacy := legacySHA256Hash("SuperSecret123!", salt)
	_, err := db.Exec(`INSERT INTO system_auth (id, password_hash, salt, is_initialized, created_at, updated_at) VALUES ('admin', ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, legacy, salt)
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewSystemAuthManager(db)
	token, err := mgr.Login(context.Background(), "SuperSecret123!")
	if err != nil {
		t.Fatalf("legacy login failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected session token")
	}
	var stored string
	if err := db.QueryRow(`SELECT password_hash FROM system_auth WHERE id = 'admin'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if isLegacyPasswordHash(stored) {
		t.Fatalf("expected argon2id rehash, got %s", stored)
	}
}

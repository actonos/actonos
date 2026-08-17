package auth

import (
	"context"
	"database/sql"
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

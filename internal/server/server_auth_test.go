package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/memory"
)

func TestServer_AuthAndProtection(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	sysAuth := auth.NewSystemAuthManager(db.SQLDB())
	eventBus := bus.NewEventBus()
	defer eventBus.Close()
	agentMgr, _ := agent.NewAgentManager(db, eventBus)
	profileMgr, _ := agent.NewUserProfileManager(db, tempDir)

	cfg := Config{
		AgentManager:   agentMgr,
		EventBus:       eventBus,
		SystemAuth:     sysAuth,
		ProfileManager: profileMgr,
	}
	srv := NewServer(cfg)

	// 1. Status initially should be not initialized
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	wStatus := httptest.NewRecorder()
	srv.Router().ServeHTTP(wStatus, reqStatus)
	if wStatus.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wStatus.Code)
	}

	// 2. Protected endpoint before setup should be 403 Forbidden
	reqProtected := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	wProtected := httptest.NewRecorder()
	srv.Router().ServeHTTP(wProtected, reqProtected)
	if wProtected.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden before setup, got %d", wProtected.Code)
	}

	// 3. Setup Initial Admin
	setupBody := `{"password":"AdminPassword123!","user_name":"TestAdmin"}`
	reqSetup := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(setupBody))
	reqSetup.Header.Set("Content-Type", "application/json")
	wSetup := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSetup, reqSetup)
	if wSetup.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for setup, got %d: %s", wSetup.Code, wSetup.Body.String())
	}

	var setupResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wSetup.Body).Decode(&setupResp)
	token := setupResp.Data.Token
	if token == "" {
		t.Fatalf("expected token from setup")
	}

	// 4. Request with valid token should pass
	reqAuthed := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	reqAuthed.Header.Set("Authorization", "Bearer "+token)
	wAuthed := httptest.NewRecorder()
	srv.Router().ServeHTTP(wAuthed, reqAuthed)
	if wAuthed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with token, got %d", wAuthed.Code)
	}

	// 5. Request with invalid token should be 401
	reqBadToken := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	reqBadToken.Header.Set("Authorization", "Bearer invalid")
	wBadToken := httptest.NewRecorder()
	srv.Router().ServeHTTP(wBadToken, reqBadToken)
	if wBadToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized with bad token, got %d", wBadToken.Code)
	}

	// 6. Login with password
	loginBody := `{"password":"AdminPassword123!"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	srv.Router().ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for login, got %d", wLogin.Code)
	}

	change := httptest.NewRequest(http.MethodPut, "/api/auth/password", strings.NewReader(`{"current_password":"AdminPassword123!","new_password":"AdminPassword456!"}`))
	change.Header.Set("Content-Type", "application/json")
	change.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	srv.Router().ServeHTTP(cw, change)
	if cw.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", cw.Code, cw.Body.String())
	}

	queryTok := httptest.NewRequest(http.MethodGet, "/api/agents?token="+token, nil)
	wQuery := httptest.NewRecorder()
	srv.Router().ServeHTTP(wQuery, queryTok)
	if wQuery.Code != http.StatusUnauthorized {
		t.Fatalf("expected query token to be ignored, got %d", wQuery.Code)
	}
}

func TestServer_MissingAuthFailClosed(t *testing.T) {
	srv := NewServer(Config{DataDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when auth is missing, got %d %s", w.Code, w.Body.String())
	}
}

func TestServer_VaultSecretsEndpoints(t *testing.T) {
	srv := newTestServer(t)

	// 1. GET /api/vault/secrets (initial empty)
	reqList := httptest.NewRequest(http.MethodGet, "/api/vault/secrets", nil)
	wList := httptest.NewRecorder()
	srv.Router().ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 for list secrets, got %d: %s", wList.Code, wList.Body.String())
	}

	// 2. POST /api/vault/secrets (set custom secret)
	reqSet := httptest.NewRequest(
		http.MethodPost,
		"/api/vault/secrets",
		bytes.NewBufferString(`{"name":"discord_bot_token","value":"test_token_12345"}`),
	)
	reqSet.Header.Set("Content-Type", "application/json")
	wSet := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSet, reqSet)
	if wSet.Code != http.StatusOK {
		t.Fatalf("expected 200 for set secret, got %d: %s", wSet.Code, wSet.Body.String())
	}

	// 3. GET /api/vault/secrets/discord_bot_token (check masked retrieval)
	reqGet := httptest.NewRequest(http.MethodGet, "/api/vault/secrets/discord_bot_token", nil)
	wGet := httptest.NewRecorder()
	srv.Router().ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get secret, got %d: %s", wGet.Code, wGet.Body.String())
	}
	var getResp struct {
		Data struct {
			Configured bool   `json:"configured"`
			Masked     string `json:"masked"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wGet.Body).Decode(&getResp)
	if !getResp.Data.Configured {
		t.Errorf("expected secret to be configured")
	}

	// 4. GET /api/vault/secrets (count = 1)
	reqList2 := httptest.NewRequest(http.MethodGet, "/api/vault/secrets", nil)
	wList2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(wList2, reqList2)
	var listResp struct {
		Data struct {
			Count   int `json:"count"`
			Secrets []struct {
				Name string `json:"name"`
			} `json:"secrets"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wList2.Body).Decode(&listResp)
	if listResp.Data.Count != 1 || listResp.Data.Secrets[0].Name != "discord_bot_token" {
		t.Errorf("expected 1 secret with name 'discord_bot_token', got %+v", listResp.Data)
	}

	// 5. DELETE /api/vault/secrets/discord_bot_token
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/vault/secrets/discord_bot_token", nil)
	wDel := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 for delete secret, got %d: %s", wDel.Code, wDel.Body.String())
	}
}

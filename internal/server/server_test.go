package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	eventBus := bus.NewEventBus()
	t.Cleanup(func() { eventBus.Close() })

	agentMgr, _ := agent.NewAgentManager(db, eventBus)
	toolReg := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(toolReg, filepath.Join(tempDir, "workspace"))

	llmRouter := llm.NewModelCascadeRouter()
	mockLLM := llm.NewMockProvider("test-model", "Test response")
	llmRouter.RegisterProvider("test-model", mockLLM)

	engine := agent.NewEngine(agentMgr, eventBus, llmRouter, nil)
	hal := system.NewDockerHAL(tempDir)
	tailscale := system.NewTailscaleManager(tempDir, "test-node", "")

	cfg := Config{
		AgentManager: agentMgr,
		Engine:       engine,
		LLMRouter:    llmRouter,
		ToolRegistry: toolReg,
		HAL:          hal,
		Tailscale:    tailscale,
		EventBus:     eventBus,
	}

	return NewServer(cfg)
}

func TestServer_Health(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			Status      string `json:"status"`
			RuntimeMode string `json:"runtime_mode"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Data.Status != "healthy" {
		t.Fatalf("expected status 'healthy', got '%s'", resp.Data.Status)
	}
}

func TestServer_AgentEndpoints(t *testing.T) {
	srv := newTestServer(t)

	// 1. Create Agent via POST /api/agents
	createBody := map[string]any{
		"name": "Dev Assistant",
		"model_config": map[string]any{
			"primary_model": "test-model",
		},
		"system_instructions": "You assist with development.",
		"authorized_tools":    []string{"native_sysinfo"},
	}
	jsonData, _ := json.Marshal(createBody)

	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp struct {
		Data agent.AgentManifest `json:"data"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.AgentID == "" {
		t.Fatal("expected generated agent_id")
	}

	// 2. List Agents via GET /api/agents
	reqList := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	wList := httptest.NewRecorder()
	srv.Router().ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wList.Code)
	}

	// 3. Get Agent via GET /api/agents/{id}
	reqGet := httptest.NewRequest(http.MethodGet, "/api/agents/"+resp.Data.AgentID, nil)
	wGet := httptest.NewRecorder()
	srv.Router().ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wGet.Code)
	}

	// 4. Delete Agent via DELETE /api/agents/{id}
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/agents/"+resp.Data.AgentID, nil)
	wDel := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", wDel.Code)
	}
}

func TestServer_ToolsAndSystem(t *testing.T) {
	srv := newTestServer(t)

	// GET /api/tools
	reqTools := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	wTools := httptest.NewRecorder()
	srv.Router().ServeHTTP(wTools, reqTools)

	if wTools.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wTools.Code)
	}

	// GET /api/system/metrics
	reqMetrics := httptest.NewRequest(http.MethodGet, "/api/system/metrics", nil)
	wMetrics := httptest.NewRecorder()
	srv.Router().ServeHTTP(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wMetrics.Code)
	}
}

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
}

func TestServer_ProductionEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	db, err := memory.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	eventBus := bus.NewEventBus()
	defer eventBus.Close()
	agentMgr, _ := agent.NewAgentManager(db, eventBus)
	toolReg := tools.NewToolRegistry(eventBus)
	llmRouter := llm.NewModelCascadeRouter()
	llmRouter.RegisterProvider("test-model", llm.NewMockProvider("test-model", "Test response"))
	engine := agent.NewEngine(agentMgr, eventBus, llmRouter, nil)

	tokenTracker := memory.NewTokenTracker(db.SQLDB())
	cronSched := agent.NewCronScheduler(engine, eventBus, db.SQLDB())
	heartbeat := agent.NewHeartbeatDaemon(agentMgr, engine, eventBus, db.SQLDB(), tempDir, 5*time.Minute)

	cfg := Config{
		AgentManager:    agentMgr,
		Engine:          engine,
		ToolRegistry:    toolReg,
		LLMRouter:       llmRouter,
		EventBus:        eventBus,
		TokenTracker:    tokenTracker,
		CronScheduler:   cronSched,
		HeartbeatDaemon: heartbeat,
	}
	srv := NewServer(cfg)

	// 1. GET /api/system/token-usage
	reqTokens := httptest.NewRequest(http.MethodGet, "/api/system/token-usage", nil)
	wTokens := httptest.NewRecorder()
	srv.Router().ServeHTTP(wTokens, reqTokens)
	if wTokens.Code != http.StatusOK {
		t.Fatalf("expected 200 for token-usage, got %d: %s", wTokens.Code, wTokens.Body.String())
	}

	// 2. GET /api/system/metrics/prometheus
	reqProm := httptest.NewRequest(http.MethodGet, "/api/system/metrics/prometheus", nil)
	wProm := httptest.NewRecorder()
	srv.Router().ServeHTTP(wProm, reqProm)
	if wProm.Code != http.StatusOK {
		t.Fatalf("expected 200 for prometheus metrics, got %d: %s", wProm.Code, wProm.Body.String())
	}
	if !strings.Contains(wProm.Body.String(), "actonos_uptime_seconds") {
		t.Errorf("expected prometheus metrics in response")
	}

	// 3. GET /api/cron/history
	reqCronHist := httptest.NewRequest(http.MethodGet, "/api/cron/history", nil)
	wCronHist := httptest.NewRecorder()
	srv.Router().ServeHTTP(wCronHist, reqCronHist)
	if wCronHist.Code != http.StatusOK {
		t.Fatalf("expected 200 for cron history, got %d", wCronHist.Code)
	}

	// 4. GET /api/system/heartbeat/history
	reqHbHist := httptest.NewRequest(http.MethodGet, "/api/system/heartbeat/history", nil)
	wHbHist := httptest.NewRecorder()
	srv.Router().ServeHTTP(wHbHist, reqHbHist)
	if wHbHist.Code != http.StatusOK {
		t.Fatalf("expected 200 for heartbeat history, got %d", wHbHist.Code)
	}
}



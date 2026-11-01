package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
)

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
		AgentManager:       agentMgr,
		Engine:             engine,
		ToolRegistry:       toolReg,
		LLMRouter:          llmRouter,
		EventBus:           eventBus,
		TokenTracker:       tokenTracker,
		CronScheduler:      cronSched,
		HeartbeatDaemon:    heartbeat,
		DisableAuthForTest: true,
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

func TestServer_ApprovalAndRunEndpoints(t *testing.T) {
	srv := newTestServer(t)
	agentWs := filepath.Join(srv.dataDir, "agents", "agent_system_core", "workspace")
	_ = os.MkdirAll(agentWs, 0755)
	_ = os.WriteFile(filepath.Join(agentWs, "approval.txt"), []byte("data"), 0644)

	body := bytes.NewBufferString(`{
		"name":"native_file_delete",
		"agent_id":"agent_system_core",
		"input":{"path":"approval.txt"}
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/tools/execute", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Router().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected approval-required 202, got %d: %s", response.Code, response.Body.String())
	}

	var pending struct {
		Data struct {
			Approval tools.ApprovalRequest `json:"approval"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatalf("decoding approval response: %v", err)
	}
	if pending.Data.Approval.ID == "" {
		t.Fatal("expected durable approval id")
	}

	approve := httptest.NewRequest(
		http.MethodPost,
		"/api/approvals/"+pending.Data.Approval.ID+"/approve",
		bytes.NewBufferString(`{"reason":"test approval"}`),
	)
	approve.Header.Set("Content-Type", "application/json")
	approveResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(approveResponse, approve)
	if approveResponse.Code != http.StatusOK {
		t.Fatalf("expected approval execution 200, got %d: %s", approveResponse.Code, approveResponse.Body.String())
	}

	runsRequest := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	runsResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(runsResponse, runsRequest)
	if runsResponse.Code != http.StatusOK {
		t.Fatalf("expected runs endpoint 200, got %d: %s", runsResponse.Code, runsResponse.Body.String())
	}
}

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/actonos/actonos/internal/agent"
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

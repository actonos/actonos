package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

type serverTestEmbedder struct{}

func (serverTestEmbedder) EmbedQuery(_ context.Context, texts []string) ([][]float32, error) {
	return serverTestVectors(texts), nil
}

func (serverTestEmbedder) EmbedPassages(_ context.Context, texts []string) ([][]float32, error) {
	return serverTestVectors(texts), nil
}

func (serverTestEmbedder) Health(context.Context) error { return nil }

func serverTestVectors(texts []string) [][]float32 {
	vectors := make([][]float32, len(texts))
	for index := range texts {
		vectors[index] = make([]float32, memory.EmbeddingDimension)
		vectors[index][0] = 1
	}
	return vectors
}

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
	workspaceStore, err := workspacepkg.NewStore(context.Background(), db.SQLDB(), filepath.Join(tempDir, "workspace"))
	if err != nil {
		t.Fatalf("creating database workspace: %v", err)
	}
	toolReg := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeToolsWithConfig(toolReg, tools.NativeToolsConfig{
		DataDir: tempDir, AgentsDir: filepath.Join(tempDir, "agents"), UserWorkspace: workspaceStore,
	})
	approvalMgr := tools.NewApprovalManager(db.SQLDB())
	toolReg.SetApprovalManager(approvalMgr)
	toolReg.SetPolicyResolver(func(ctx context.Context, agentID string) (tools.AgentToolPolicy, error) {
		manifest, policyErr := agentMgr.Get(ctx, agentID)
		if policyErr != nil {
			return tools.AgentToolPolicy{}, policyErr
		}
		return tools.AgentToolPolicy{
			AuthorizedTools:   manifest.AuthorizedTools,
			ApprovalThreshold: string(manifest.DelegationScope.RequireHumanApproval),
			AllowedPaths:      manifest.DelegationScope.AllowedWorkspacePaths,
		}, nil
	})

	llmRouter := llm.NewModelCascadeRouter()
	mockLLM := llm.NewMockProvider("test-model", "Test response")
	llmRouter.RegisterProvider("test-model", mockLLM)

	vectorStore, err := memory.NewVectorStore(filepath.Join(tempDir, "vectors"))
	if err != nil {
		t.Fatalf("creating vector store: %v", err)
	}
	hybrid := memory.NewHybridEngine(db, vectorStore, nil)
	embedding := memory.NewEmbeddingService(db.SQLDB(), vectorStore, serverTestEmbedder{})
	embedding.SetWorkspaceStore(workspaceStore)
	toolReg.SetWorkspaceMutationSink(embedding)
	hybrid.SetEmbeddingService(embedding)
	engine := agent.NewEngine(agentMgr, eventBus, llmRouter, hybrid)
	engine.SetEmbeddingService(embedding)
	engine.SetToolRegistry(toolReg)
	runStore := agent.NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)
	taskMgr, err := agent.NewTaskManager(db.SQLDB(), filepath.Join(tempDir, "workspace"))
	if err != nil {
		t.Fatalf("creating task manager: %v", err)
	}
	auditLogger, err := system.NewAuditLogger(filepath.Join(tempDir, "audit"))
	if err != nil {
		t.Fatalf("creating audit logger: %v", err)
	}
	t.Cleanup(func() { _ = auditLogger.Close() })
	profileMgr, err := agent.NewUserProfileManager(db, tempDir)
	if err != nil {
		t.Fatalf("creating profile manager: %v", err)
	}
	tokenTracker := memory.NewTokenTracker(db.SQLDB())
	vault, err := memory.NewVault(db, "server-test-master-secret", nil)
	if err != nil {
		t.Fatalf("creating test vault: %v", err)
	}
	cronScheduler := agent.NewCronScheduler(engine, eventBus, db.SQLDB())
	hubManager := tools.NewHubManager(filepath.Join(tempDir, "skills"))
	skillWatcher := tools.NewSkillWatcher(toolReg, filepath.Join(tempDir, "skills"))
	hubManager.SetToolRegistry(toolReg)
	hubManager.SetSkillWatcher(skillWatcher)
	mcpHost := tools.NewMCPHostEngine(toolReg)
	if err := mcpHost.SetPersistence(db.SQLDB(), nil); err != nil {
		t.Fatalf("configuring MCP persistence: %v", err)
	}
	pairingManager, err := channels.NewPairingManager(db.SQLDB())
	if err != nil {
		t.Fatalf("creating pairing manager: %v", err)
	}
	channelManager := channels.NewChannelManager(eventBus, pairingManager)
	heartbeat := agent.NewHeartbeatDaemon(agentMgr, engine, eventBus, db.SQLDB(), filepath.Join(tempDir, "workspace"), time.Hour)
	hal := system.NewDockerHAL(tempDir)
	tailscale := system.NewTailscaleManager(tempDir, "test-node", "")
	notifMgr, err := system.NewNotificationManager(db.SQLDB(), eventBus)
	if err != nil {
		t.Fatalf("creating notification manager: %v", err)
	}

	cfg := Config{
		AgentManager:        agentMgr,
		Engine:              engine,
		LLMRouter:           llmRouter,
		ToolRegistry:        toolReg,
		SkillWatcher:        skillWatcher,
		ApprovalManager:     approvalMgr,
		RunStore:            runStore,
		TaskManager:         taskMgr,
		AuditLogger:         auditLogger,
		Vault:               vault,
		ProfileManager:      profileMgr,
		TokenTracker:        tokenTracker,
		Memory:              hybrid,
		Embedding:           embedding,
		CronScheduler:       cronScheduler,
		HubManager:          hubManager,
		MCPHost:             mcpHost,
		PairingManager:      pairingManager,
		ChannelManager:      channelManager,
		HeartbeatDaemon:     heartbeat,
		NotificationManager: notifMgr,
		HAL:                 hal,
		Tailscale:           tailscale,
		EventBus:            eventBus,
		WorkspaceDir:        filepath.Join(tempDir, "workspace"),
		WorkspaceStore:      workspaceStore,
		SkillsDir:           filepath.Join(tempDir, "skills"),
		WASMDir:             filepath.Join(tempDir, "tools", "wasm"),
		DataDir:             tempDir,
	}

	return NewServer(cfg)
}

func TestServer_EmbeddingStatusAndWorkspaceQueue(t *testing.T) {
	srv := newTestServer(t)

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/system/embedding", nil)
	statusResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("embedding status failed: %d %s", statusResponse.Code, statusResponse.Body.String())
	}
	if !strings.Contains(statusResponse.Body.String(), memory.EmbeddingModelID) ||
		!strings.Contains(statusResponse.Body.String(), `"service_ready":true`) {
		t.Fatalf("unexpected embedding status: %s", statusResponse.Body.String())
	}

	directory := createWorkspaceDirectory(t, srv, "", "embedding")
	file := createWorkspaceFile(t, srv, workspaceString(directory["id"]), "status.txt", "alpha")
	fileID := workspaceString(file["id"])
	var operation, scope, sourceRef string
	var dueAt, createdAt time.Time
	err := srv.memory.DB().SQLDB().QueryRow(`SELECT operation, scope, source_ref, due_at, created_at
		FROM embedding_jobs WHERE source_type = 'workspace_file' AND source_key = ?`, fileID).
		Scan(&operation, &scope, &sourceRef, &dueAt, &createdAt)
	if err != nil {
		t.Fatalf("reading workspace embedding job: %v", err)
	}
	if operation != string(memory.EmbeddingUpsert) || scope != "shared" ||
		sourceRef != fileID {
		t.Fatalf("unexpected workspace embedding job: operation=%s scope=%s ref=%s delay=%s",
			operation, scope, sourceRef, dueAt.Sub(createdAt))
	}

	workspaceJSONRequest(t, srv, http.MethodDelete, "/api/workspace/file?id="+fileID, "", http.StatusOK)
	var generation int
	if err := srv.memory.DB().SQLDB().QueryRow(`SELECT operation, generation FROM embedding_jobs
		WHERE source_type = 'workspace_file' AND source_key = ?`, fileID).Scan(&operation, &generation); err != nil {
		t.Fatal(err)
	}
	if operation != string(memory.EmbeddingDelete) || generation != 2 {
		t.Fatalf("delete did not supersede upsert: operation=%s generation=%d", operation, generation)
	}
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

func TestServer_WorkspaceDirectMutationAndFileOperations(t *testing.T) {
	srv := newTestServer(t)
	directory := createWorkspaceDirectory(t, srv, "", "notes")
	file := createWorkspaceFile(t, srv, workspaceString(directory["id"]), "result.txt", "approved content")
	fileID := workspaceString(file["id"])
	read := httptest.NewRequest(http.MethodGet, "/api/workspace/file?id="+fileID, nil)
	readResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(readResult, read)
	if readResult.Code != http.StatusOK || !strings.Contains(readResult.Body.String(), "approved content") {
		t.Fatalf("workspace read failed: %d %s", readResult.Code, readResult.Body.String())
	}

	rawReq := httptest.NewRequest(http.MethodGet, "/api/workspace/raw?id="+fileID, nil)
	rawResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(rawResult, rawReq)
	if rawResult.Code != http.StatusOK || rawResult.Body.String() != "approved content" {
		t.Fatalf("workspace raw read failed: %d %s", rawResult.Code, rawResult.Body.String())
	}
}

func TestServer_TaskRunApprovalAndAuditEndpoints(t *testing.T) {
	srv := newTestServer(t)

	create := httptest.NewRequest(http.MethodPost, "/api/tasks/", strings.NewReader(`{"title":"API lifecycle","priority":"p1_high"}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	srv.Router().ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create task failed: %d %s", created.Code, created.Body.String())
	}
	var taskResponse struct {
		Data agent.AutonomousTask `json:"data"`
	}
	if err := json.NewDecoder(created.Body).Decode(&taskResponse); err != nil {
		t.Fatal(err)
	}
	taskID := taskResponse.Data.ID

	for _, endpoint := range []string{
		"/api/tasks/?status=pending&priority=p1_high",
		"/api/tasks/" + taskID,
		"/api/heartbeat/config",
		"/api/heartbeat/runs",
		"/api/runs/?limit=5",
		"/api/approvals/?status=pending",
		"/api/system/audit/verify",
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
		if (strings.Contains(endpoint, "/approvals/") || strings.Contains(endpoint, "/runs/")) &&
			strings.Contains(w.Body.String(), ":null") {
			t.Fatalf("collection endpoint %s returned null instead of []: %s", endpoint, w.Body.String())
		}
	}

	update := httptest.NewRequest(http.MethodPut, "/api/tasks/"+taskID, strings.NewReader(`{"title":"API lifecycle","status":"completed","priority":"p1_high"}`))
	update.Header.Set("Content-Type", "application/json")
	updated := httptest.NewRecorder()
	srv.Router().ServeHTTP(updated, update)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"progress":100`) {
		t.Fatalf("update task failed: %d %s", updated.Code, updated.Body.String())
	}

	config := httptest.NewRequest(http.MethodPut, "/api/heartbeat/config", strings.NewReader(`{"enabled":true,"interval_minutes":2,"directives":"verify continuously"}`))
	config.Header.Set("Content-Type", "application/json")
	configured := httptest.NewRecorder()
	srv.Router().ServeHTTP(configured, config)
	if configured.Code != http.StatusOK {
		t.Fatalf("save heartbeat config failed: %d %s", configured.Code, configured.Body.String())
	}

	events := httptest.NewRequest(http.MethodGet, "/api/runs/nonexistent/events", nil)
	eventResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(eventResult, events)
	if eventResult.Code != http.StatusOK {
		t.Fatalf("empty run events failed: %d %s", eventResult.Code, eventResult.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+taskID, nil)
	deleted := httptest.NewRecorder()
	srv.Router().ServeHTTP(deleted, deleteReq)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete task failed: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestServer_ConversationIdentityMetricsAndCatalogLifecycle(t *testing.T) {
	srv := newTestServer(t)
	create := httptest.NewRequest(http.MethodPost, "/api/conversations/", strings.NewReader(`{"agent_id":"agent_system_core","title":"Integration session"}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	srv.Router().ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("conversation create failed: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		Data Conversation `json:"data"`
	}
	if err := json.NewDecoder(created.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	convID := response.Data.ID
	for _, endpoint := range []string{
		"/api/conversations/",
		"/api/conversations/?agent_id=agent_system_core",
		"/api/conversations/" + convID,
		"/api/system/metrics/prometheus",
		"/api/system/token-usage",
		"/api/system/token-usage/history?agent_id=all&source=all",
		"/api/system/heartbeat/history",
		"/api/system/identity",
		"/api/system/profile",
		"/api/system/models",
		"/api/models",
		"/api/system/storage",
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
	if generateConversationTitle("   ") != "New Session" ||
		!strings.HasSuffix(generateConversationTitle(strings.Repeat("x", 50)), "...") {
		t.Fatal("conversation title generation failed")
	}
	update := httptest.NewRequest(http.MethodPut, "/api/conversations/"+convID, strings.NewReader(`{"title":"Renamed"}`))
	update.Header.Set("Content-Type", "application/json")
	updated := httptest.NewRecorder()
	srv.Router().ServeHTTP(updated, update)
	if updated.Code != http.StatusOK {
		t.Fatalf("conversation update failed: %d %s", updated.Code, updated.Body.String())
	}
	identity := httptest.NewRequest(http.MethodPut, "/api/system/identity", strings.NewReader(`{"user_name":"Tester","language":"vi","soul":"Test soul"}`))
	identity.Header.Set("Content-Type", "application/json")
	saved := httptest.NewRecorder()
	srv.Router().ServeHTTP(saved, identity)
	if saved.Code != http.StatusOK {
		t.Fatalf("identity save failed: %d %s", saved.Code, saved.Body.String())
	}
	remove := httptest.NewRequest(http.MethodDelete, "/api/conversations/"+convID, nil)
	removed := httptest.NewRecorder()
	srv.Router().ServeHTTP(removed, remove)
	if removed.Code != http.StatusOK {
		t.Fatalf("conversation delete failed: %d %s", removed.Code, removed.Body.String())
	}
	missing := httptest.NewRequest(http.MethodGet, "/api/conversations/"+convID, nil)
	notFound := httptest.NewRecorder()
	srv.Router().ServeHTTP(notFound, missing)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("deleted conversation still available: %d", notFound.Code)
	}
}

func TestServer_AgentLifecycleChatStreamingSoulAndCron(t *testing.T) {
	srv := newTestServer(t)
	createBody := `{
		"name":"Streaming API Agent",
		"model_config":{"primary_model":"test-model"},
		"system_instructions":"Answer directly.",
		"authorized_tools":[]
	}`
	create := httptest.NewRequest(http.MethodPost, "/api/agents/", strings.NewReader(createBody))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	srv.Router().ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("agent create failed: %d %s", created.Code, created.Body.String())
	}
	var agentResponse struct {
		Data agent.AgentManifest `json:"data"`
	}
	_ = json.NewDecoder(created.Body).Decode(&agentResponse)
	agentID := agentResponse.Data.AgentID

	update := httptest.NewRequest(http.MethodPut, "/api/agents/"+agentID+"/", strings.NewReader(`{"name":"Updated Agent","model_config":{"primary_model":"test-model"}}`))
	update.Header.Set("Content-Type", "application/json")
	updated := httptest.NewRecorder()
	srv.Router().ServeHTTP(updated, update)
	if updated.Code != http.StatusOK {
		t.Fatalf("agent update failed: %d %s", updated.Code, updated.Body.String())
	}
	for _, action := range []string{"stop", "start"} {
		req := httptest.NewRequest(http.MethodPost, "/api/agents/"+agentID+"/"+action, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("agent %s failed: %d %s", action, w.Code, w.Body.String())
		}
	}

	saveSoul := httptest.NewRequest(http.MethodPut, "/api/agents/"+agentID+"/soul", strings.NewReader(`{"content":"API-specific soul"}`))
	saveSoul.Header.Set("Content-Type", "application/json")
	soulSaved := httptest.NewRecorder()
	srv.Router().ServeHTTP(soulSaved, saveSoul)
	if soulSaved.Code != http.StatusOK {
		t.Fatalf("soul save failed: %d %s", soulSaved.Code, soulSaved.Body.String())
	}
	for _, endpoint := range []string{
		"/api/agents/" + agentID + "/soul",
		"/api/agents/" + agentID + "/memory-md",
		"/api/agents/soul?agent_id=" + agentID,
		"/api/agents/memory-md?agent_id=" + agentID,
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}

	chat := httptest.NewRequest(http.MethodPost, "/api/agents/"+agentID+"/chat", strings.NewReader(`{"message":"normal chat"}`))
	chat.Header.Set("Content-Type", "application/json")
	chatResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(chatResult, chat)
	if chatResult.Code != http.StatusOK || !strings.Contains(chatResult.Body.String(), "Test response") {
		t.Fatalf("chat failed: %d %s", chatResult.Code, chatResult.Body.String())
	}

	stream := httptest.NewRequest(http.MethodPost, "/api/agents/"+agentID+"/chat/stream", strings.NewReader(`{"message":"stream chat"}`))
	stream.Header.Set("Content-Type", "application/json")
	streamResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(streamResult, stream)
	if streamResult.Code != http.StatusOK ||
		streamResult.Header().Get("Content-Type") != "text/event-stream" ||
		!strings.Contains(streamResult.Body.String(), "event: token") ||
		!strings.Contains(streamResult.Body.String(), "event: done") {
		t.Fatalf("true SSE stream failed: status=%d headers=%v body=%s", streamResult.Code, streamResult.Header(), streamResult.Body.String())
	}
	var chatJobs int
	if err := srv.memory.DB().SQLDB().QueryRow(`SELECT COUNT(*) FROM embedding_jobs j
		JOIN messages m ON m.id = j.source_key
		WHERE j.source_type = 'message' AND m.agent_id = ? AND m.role = 'user'
		AND m.content IN ('normal chat', 'stream chat')`, agentID).Scan(&chatJobs); err != nil {
		t.Fatal(err)
	}
	if chatJobs != 2 {
		t.Fatalf("chat endpoints enqueued %d user messages, want 2", chatJobs)
	}

	cronBody := `{"id":"api_cron","name":"API Cron","cron_expr":"0 8 * * *","prompt":"status","enabled":true}`
	saveCron := httptest.NewRequest(http.MethodPost, "/api/cron/", strings.NewReader(cronBody))
	saveCron.Header.Set("Content-Type", "application/json")
	cronSaved := httptest.NewRecorder()
	srv.Router().ServeHTTP(cronSaved, saveCron)
	if cronSaved.Code != http.StatusOK {
		t.Fatalf("cron save failed: %d %s", cronSaved.Code, cronSaved.Body.String())
	}
	for _, endpoint := range []string{"/api/cron/", "/api/agents/cron", "/api/cron/history", "/api/cron/api_cron/history"} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
	runCron := httptest.NewRequest(http.MethodPost, "/api/cron/api_cron/run", nil)
	runResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(runResult, runCron)
	if runResult.Code != http.StatusOK {
		t.Fatalf("cron run failed: %d %s", runResult.Code, runResult.Body.String())
	}
	deleteCron := httptest.NewRequest(http.MethodDelete, "/api/cron/api_cron", nil)
	deleteResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(deleteResult, deleteCron)
	if deleteResult.Code != http.StatusOK {
		t.Fatalf("cron delete failed: %d %s", deleteResult.Code, deleteResult.Body.String())
	}
}

func TestServer_SystemSetupKeysDashboardAndAdministrativeTools(t *testing.T) {
	srv := newTestServer(t)
	for _, endpoint := range []string{
		"/api/dashboard/summary",
		"/api/setup/status",
		"/api/system/keys",
		"/api/system/audit",
		"/api/system/storage",
		"/api/system/tailscale",
		"/api/tools/?category=native",
		"/api/tools/hub/catalog",
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
	ota := httptest.NewRequest(http.MethodPost, "/api/system/ota/check", nil)
	otaResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(otaResult, ota)
	if otaResult.Code != http.StatusOK {
		t.Fatalf("OTA check failed: %d %s", otaResult.Code, otaResult.Body.String())
	}
	if _, err := srv.memory.DB().SQLDB().Exec(`CREATE TABLE backup_probe (value TEXT); INSERT INTO backup_probe VALUES ('committed-wal-data')`); err != nil {
		t.Fatal(err)
	}
	backup := httptest.NewRequest(http.MethodGet, "/api/system/backup", nil)
	backupResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(backupResult, backup)
	if backupResult.Code != http.StatusOK || backupResult.Header().Get("Content-Type") != "application/x-sqlite3" {
		t.Fatalf("backup failed: %d %s", backupResult.Code, backupResult.Body.String())
	}
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := os.WriteFile(backupPath, backupResult.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	var probe string
	if err := backupDB.QueryRow(`SELECT value FROM backup_probe`).Scan(&probe); err != nil || probe != "committed-wal-data" {
		t.Fatalf("backup omitted committed WAL data: probe=%q err=%v", probe, err)
	}
	setup := httptest.NewRequest(http.MethodPost, "/api/setup/wizard", strings.NewReader(`{"openai_key":"setup-key"}`))
	setup.Header.Set("Content-Type", "application/json")
	setupResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(setupResult, setup)
	if setupResult.Code != http.StatusOK {
		t.Fatalf("setup wizard failed: %d %s", setupResult.Code, setupResult.Body.String())
	}
	setupSecret, err := srv.vault.GetSecret(context.Background(), providerSecretName("openai"))
	if err != nil || setupSecret != "setup-key" {
		t.Fatalf("setup key was not stored in Vault: secret=%q err=%v", setupSecret, err)
	}
	if _, err := os.Stat(filepath.Join(srv.dataDir, "config", "openai.key")); !os.IsNotExist(err) {
		t.Fatalf("setup wrote a plaintext key file: %v", err)
	}
	keys := httptest.NewRequest(http.MethodPost, "/api/system/keys", strings.NewReader(`{"provider":"custom_openai","api_key":"custom-secret-key","base_url":"https://example.com/v1","default_model":"custom","enabled":true}`))
	keys.Header.Set("Content-Type", "application/json")
	keysResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(keysResult, keys)
	if keysResult.Code != http.StatusOK {
		t.Fatalf("provider save failed: %d %s", keysResult.Code, keysResult.Body.String())
	}
	providerFile, err := os.ReadFile(filepath.Join(srv.dataDir, "config", "llm_providers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(providerFile), "custom-secret-key") {
		t.Fatal("provider secret leaked into plaintext metadata")
	}
	secret, err := srv.vault.GetSecret(context.Background(), providerSecretName("custom_openai"))
	if err != nil || secret != "custom-secret-key" {
		t.Fatalf("provider secret was not encrypted in Vault: secret=%q err=%v", secret, err)
	}
	if maskKey("") != "" || maskKey("short") == "short" || !strings.Contains(maskKey("123456789012"), "1234") {
		t.Fatal("API key masking failed")
	}
	missingKey := httptest.NewRequest(http.MethodPost, "/api/system/keys/test", strings.NewReader(`{"provider":"missing_provider"}`))
	missingKey.Header.Set("Content-Type", "application/json")
	missingResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(missingResult, missingKey)
	if missingResult.Code != http.StatusBadRequest {
		t.Fatalf("missing provider key was accepted: %d %s", missingResult.Code, missingResult.Body.String())
	}
	deleteKey := httptest.NewRequest(http.MethodDelete, "/api/system/keys/custom_openai", nil)
	deleteResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(deleteResult, deleteKey)
	if deleteResult.Code != http.StatusOK {
		t.Fatalf("provider delete failed: %d %s", deleteResult.Code, deleteResult.Body.String())
	}
	if _, err := srv.vault.GetSecret(context.Background(), providerSecretName("custom_openai")); !errors.Is(err, memory.ErrSecretNotFound) {
		t.Fatalf("deleted provider secret remains in Vault: %v", err)
	}

	type approvalEnvelope struct {
		Data struct {
			Approval tools.ApprovalRequest `json:"approval"`
		} `json:"data"`
	}
	requestApproval := func(method, endpoint, contentType string, body *bytes.Buffer) tools.ApprovalRequest {
		t.Helper()
		req := httptest.NewRequest(method, endpoint, body)
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("%s %s failed: %d %s", method, endpoint, w.Code, w.Body.String())
		}
		var envelope approvalEnvelope
		if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data.Approval
	}
	approve := func(item tools.ApprovalRequest, expected int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+item.ID+"/approve", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != expected {
			t.Fatalf("approval %s expected %d, got %d: %s", item.ToolName, expected, w.Code, w.Body.String())
		}
	}

	skillReq := httptest.NewRequest(http.MethodPost, "/api/tools/skill", bytes.NewBufferString(`{"name":"api_skill","description":"API skill","content":"Instructions"}`))
	skillReq.Header.Set("Content-Type", "application/json")
	skillW := httptest.NewRecorder()
	srv.Router().ServeHTTP(skillW, skillReq)
	if skillW.Code != http.StatusOK {
		t.Fatalf("expected skill creation status 200, got %d: %s", skillW.Code, skillW.Body.String())
	}
	if _, err := os.Stat(filepath.Join(srv.skillsDir, "api_skill", "SKILL.md")); err != nil {
		t.Fatalf("created skill was not found on disk: %v", err)
	}

	wasmBody := &bytes.Buffer{}
	wasmWriter := multipart.NewWriter(wasmBody)
	wasmPart, _ := wasmWriter.CreateFormFile("file", "plugin.wasm")
	_, _ = wasmPart.Write([]byte("wasm bytes"))
	_ = wasmWriter.Close()
	wasmApproval := requestApproval(http.MethodPost, "/api/tools/wasm", wasmWriter.FormDataContentType(), wasmBody)
	approve(wasmApproval, http.StatusOK)
	if _, err := os.Stat(filepath.Join(srv.wasmDir, "plugin.wasm")); err != nil {
		t.Fatalf("approved WASM was not stored: %v", err)
	}

	var catalog []tools.HubSkill
	for i := 0; i < 20; i++ {
		catalog = srv.hubMgr.ListCatalog()
		if len(catalog) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(catalog) == 0 {
		_ = srv.hubMgr.FetchRemoteCatalog(context.Background())
		catalog = srv.hubMgr.ListCatalog()
	}
	if len(catalog) > 0 {
		skillID := catalog[0].ID
		installReq := httptest.NewRequest(http.MethodPost, "/api/tools/hub/install", bytes.NewBufferString(`{"skill_id":"`+skillID+`"}`))
		installReq.Header.Set("Content-Type", "application/json")
		installW := httptest.NewRecorder()
		srv.Router().ServeHTTP(installW, installReq)
		if installW.Code != http.StatusOK {
			t.Fatalf("expected install status 200, got %d: %s", installW.Code, installW.Body.String())
		}

		uninstallReq := httptest.NewRequest(http.MethodPost, "/api/tools/hub/uninstall", bytes.NewBufferString(`{"skill_id":"`+skillID+`"}`))
		uninstallReq.Header.Set("Content-Type", "application/json")
		uninstallW := httptest.NewRecorder()
		srv.Router().ServeHTTP(uninstallW, uninstallReq)
		if uninstallW.Code != http.StatusOK {
			t.Fatalf("expected uninstall status 200, got %d: %s", uninstallW.Code, uninstallW.Body.String())
		}

		// Verify skill is unregistered from tool registry
		for _, tool := range srv.toolReg.ListByCategory("skill") {
			if tool.Name == "skill_"+skillID || tool.Name == skillID {
				t.Fatalf("expected skill %s to be unregistered from tool registry, found: %s", skillID, tool.Name)
			}
		}
	}

	restartApproval := requestApproval(
		http.MethodPost, "/api/system/restart", "application/json", bytes.NewBuffer(nil),
	)
	if restartApproval.ToolName != "admin_system_restart" {
		t.Fatalf("unexpected restart approval: %+v", restartApproval)
	}
	reject := httptest.NewRequest(http.MethodPost, "/api/approvals/"+restartApproval.ID+"/reject", strings.NewReader(`{"reason":"not now"}`))
	reject.Header.Set("Content-Type", "application/json")
	rejected := httptest.NewRecorder()
	srv.Router().ServeHTTP(rejected, reject)
	if rejected.Code != http.StatusOK {
		t.Fatalf("restart rejection failed: %d %s", rejected.Code, rejected.Body.String())
	}
}

func TestProviderKeyLegacyMigrationToVault(t *testing.T) {
	srv := newTestServer(t)
	configDir := filepath.Join(srv.dataDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	legacyJSON := `{"openai":{"id":"openai","name":"OpenAI","api_key":"legacy-json-secret","enabled":true}}`
	if err := os.WriteFile(filepath.Join(configDir, "llm_providers.json"), []byte(legacyJSON), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "anthropic.key"), []byte("legacy-file-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	stored := loadStoredProvidersWithVault(context.Background(), configDir, srv.vault)
	if stored["openai"].APIKey != "legacy-json-secret" || stored["anthropic"].APIKey != "legacy-file-secret" {
		t.Fatalf("legacy keys were not loaded during migration: %+v", stored)
	}
	for provider, expected := range map[string]string{
		"openai": "legacy-json-secret", "anthropic": "legacy-file-secret",
	} {
		secret, err := srv.vault.GetSecret(context.Background(), providerSecretName(provider))
		if err != nil || secret != expected {
			t.Fatalf("legacy %s key was not migrated: secret=%q err=%v", provider, secret, err)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(configDir, "llm_providers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), "legacy-json-secret") {
		t.Fatal("legacy JSON secret remained after migration")
	}
	if _, err := os.Stat(filepath.Join(configDir, "anthropic.key")); !os.IsNotExist(err) {
		t.Fatalf("legacy key file remained after migration: %v", err)
	}
}

func TestServer_IntegrationsChannelsPairingAndWebhookValidation(t *testing.T) {
	srv := newTestServer(t)
	for _, endpoint := range []string{
		"/api/integrations/",
		"/api/integrations/channels",
		"/api/integrations/channels/accounts",
		"/api/integrations/authorizations",
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
	config := httptest.NewRequest(http.MethodPost, "/api/integrations/custom/config", strings.NewReader(`{"client_id":"client","client_secret":"secret"}`))
	config.Header.Set("Content-Type", "application/json")
	configResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(configResult, config)
	if configResult.Code != http.StatusOK {
		t.Fatalf("connector config failed: %d %s", configResult.Code, configResult.Body.String())
	}
	token := httptest.NewRequest(http.MethodPost, "/api/integrations/custom/token", strings.NewReader(`{"token":"direct-token"}`))
	token.Header.Set("Content-Type", "application/json")
	tokenResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(tokenResult, token)
	if tokenResult.Code != http.StatusOK {
		t.Fatalf("custom connector token failed: %d %s", tokenResult.Code, tokenResult.Body.String())
	}
	testConnector := httptest.NewRequest(http.MethodPost, "/api/integrations/custom/test", nil)
	testResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(testResult, testConnector)
	if testResult.Code != http.StatusOK {
		t.Fatalf("custom connector test failed: %d %s", testResult.Code, testResult.Body.String())
	}
	for _, action := range []string{"toggle", "disconnect"} {
		req := httptest.NewRequest(http.MethodPost, "/api/integrations/custom/"+action, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("connector %s failed: %d %s", action, w.Code, w.Body.String())
		}
	}
	authURL := httptest.NewRequest(http.MethodPost, "/api/integrations/github/auth-url", strings.NewReader(`{}`))
	authURL.Header.Set("Content-Type", "application/json")
	authResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(authResult, authURL)
	if authResult.Code != http.StatusInternalServerError {
		t.Fatalf("unconfigured OAuth should fail cleanly: %d %s", authResult.Code, authResult.Body.String())
	}
	callback := httptest.NewRequest(http.MethodGet, "/api/integrations/oauth/callback?error=denied&error_description=no", nil)
	callbackResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(callbackResult, callback)
	if callbackResult.Code != http.StatusFound {
		t.Fatalf("OAuth error callback did not redirect: %d", callbackResult.Code)
	}

	channelBody := `{
		"webhook_secret":"integration-secret",
		"telegram_accounts":[{"id":"tg_test","name":"Test Bot","channel":"telegram","token":"tg-token","enabled":true,"bound_agent_ids":["*"]}],
		"discord_accounts":[{"id":"dc_test","name":"Discord","channel":"discord","token":"dc-token","enabled":false}],
		"whatsapp_accounts":[{"id":"wa_test","name":"WhatsApp","channel":"whatsapp","token":"wa-token","phone_id":"phone","enabled":true}]
	}`
	saveChannels := httptest.NewRequest(http.MethodPost, "/api/integrations/channels", strings.NewReader(channelBody))
	saveChannels.Header.Set("Content-Type", "application/json")
	channelResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(channelResult, saveChannels)
	if channelResult.Code != http.StatusOK {
		t.Fatalf("channel save failed: %d %s", channelResult.Code, channelResult.Body.String())
	}
	accounts := httptest.NewRequest(http.MethodGet, "/api/integrations/channels/accounts", nil)
	accountsResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(accountsResult, accounts)
	if accountsResult.Code != http.StatusOK || !strings.Contains(accountsResult.Body.String(), "tg_test") ||
		strings.Contains(accountsResult.Body.String(), "tg-token") {
		t.Fatalf("channel accounts were not returned safely: %d %s", accountsResult.Code, accountsResult.Body.String())
	}

	pair := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/code", strings.NewReader(`{"channel_id":"telegram"}`))
	pair.Header.Set("Content-Type", "application/json")
	pairResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(pairResult, pair)
	if pairResult.Code != http.StatusOK {
		t.Fatalf("pairing code failed: %d %s", pairResult.Code, pairResult.Body.String())
	}
	var pairEnvelope struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	_ = json.NewDecoder(pairResult.Body).Decode(&pairEnvelope)
	verifyBody := fmt.Sprintf(`{"channel_id":"telegram","code":%q,"sender_id":"user-1","sender_name":"Tester"}`, pairEnvelope.Data.Code)
	verify := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/verify", strings.NewReader(verifyBody))
	verify.Header.Set("Content-Type", "application/json")
	verifyResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(verifyResult, verify)
	if verifyResult.Code != http.StatusOK {
		t.Fatalf("pairing verification failed: %d %s", verifyResult.Code, verifyResult.Body.String())
	}
	revoke := httptest.NewRequest(http.MethodDelete, "/api/integrations/authorizations", strings.NewReader(`{"channel_id":"telegram","sender_id":"user-1"}`))
	revoke.Header.Set("Content-Type", "application/json")
	revokeResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(revokeResult, revoke)
	if revokeResult.Code != http.StatusOK {
		t.Fatalf("authorization revoke failed: %d %s", revokeResult.Code, revokeResult.Body.String())
	}
	for _, webhook := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/webhooks/whatsapp"},
		{http.MethodPost, "/api/webhooks/whatsapp"},
	} {
		req := httptest.NewRequest(webhook.method, webhook.path, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("unconfigured webhook expected 501, got %d", w.Code)
		}
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

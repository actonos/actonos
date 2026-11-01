package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actonos/actonos/internal/memory"
)

// call sends a request through the full router and decodes the {"data": ...} envelope.
func call(t *testing.T, srv *Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	var envelope map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("%s %s: response is not JSON: %v\n%s", method, path, err, rec.Body.String())
		}
	}
	return rec.Code, envelope
}

func dataOf(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", envelope)
	}
	return data
}

func TestMemoryManagementEndpoints(t *testing.T) {
	srv := newTestServer(t)
	rec, err := srv.memory.StoreMemory(context.Background(), "agent-mem", memory.LayerEpisodic,
		"deploys happen on Fridays", make([]float32, memory.EmbeddingDimension), nil, 1)
	if err != nil {
		t.Fatalf("seeding memory: %v", err)
	}
	base := "/api/agents/agent-mem/memories"

	status, env := call(t, srv, http.MethodGet, base+"?layer=episodic&limit=5", "")
	if status != http.StatusOK || dataOf(t, env)["count"].(float64) != 1 {
		t.Fatalf("list: status=%d body=%v", status, env)
	}

	status, env = call(t, srv, http.MethodPost, base+"/"+rec.ID+"/pin", "")
	if status != http.StatusOK || dataOf(t, env)["status"] != "pinned" {
		t.Fatalf("pin: status=%d body=%v", status, env)
	}

	status, env = call(t, srv, http.MethodPut, base+"/"+rec.ID+"/importance", `{"importance":"critical"}`)
	if status != http.StatusOK || dataOf(t, env)["importance"] != "critical" {
		t.Fatalf("set importance: status=%d body=%v", status, env)
	}

	// An empty tier falls back to "normal" rather than failing.
	status, env = call(t, srv, http.MethodPut, base+"/"+rec.ID+"/importance", `{}`)
	if status != http.StatusOK || dataOf(t, env)["importance"] != "normal" {
		t.Fatalf("default importance: status=%d body=%v", status, env)
	}

	if status, _ = call(t, srv, http.MethodPut, base+"/"+rec.ID+"/importance", `{not json`); status != http.StatusBadRequest {
		t.Fatalf("malformed body: status=%d, want 400", status)
	}

	status, env = call(t, srv, http.MethodDelete, base+"/"+rec.ID+"/pin", "")
	if status != http.StatusOK || dataOf(t, env)["status"] != "unpinned" {
		t.Fatalf("unpin: status=%d body=%v", status, env)
	}

	listed, err := srv.memory.ListMemories(context.Background(), "agent-mem", "", 10)
	if err != nil || len(listed) != 1 || listed[0].Pinned {
		t.Fatalf("final state wrong: %+v err=%v", listed, err)
	}
}

func TestMemoryEndpointsUnavailableWithoutEngine(t *testing.T) {
	srv := newTestServer(t)
	srv.memory = nil
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/agents/a/memories", ""},
		{http.MethodPost, "/api/agents/a/memories/m/pin", ""},
		{http.MethodDelete, "/api/agents/a/memories/m/pin", ""},
		{http.MethodPut, "/api/agents/a/memories/m/importance", `{"importance":"high"}`},
	} {
		if status, _ := call(t, srv, tc.method, tc.path, tc.body); status != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status=%d, want 503", tc.method, tc.path, status)
		}
	}
}

func TestAgentTemplateEndpoints(t *testing.T) {
	srv := newTestServer(t)

	status, env := call(t, srv, http.MethodGet, "/api/agents/templates", "")
	if status != http.StatusOK {
		t.Fatalf("list templates: status=%d", status)
	}
	data := dataOf(t, env)
	templates, _ := data["templates"].([]any)
	if len(templates) == 0 || data["count"].(float64) != float64(len(templates)) {
		t.Fatalf("expected a non-empty template catalogue with a matching count: %v", data["count"])
	}

	first := templates[0].(map[string]any)
	id, _ := first["id"].(string)
	if id == "" {
		t.Fatalf("template has no id: %v", first)
	}
	if status, env = call(t, srv, http.MethodGet, "/api/agents/templates/"+id, ""); status != http.StatusOK {
		t.Fatalf("get template %q: status=%d body=%v", id, status, env)
	}
	if status, _ = call(t, srv, http.MethodGet, "/api/agents/templates/no-such-template", ""); status != http.StatusNotFound {
		t.Fatalf("unknown template: status=%d, want 404", status)
	}

	// Filters that match nothing return an empty list, not an error.
	status, env = call(t, srv, http.MethodGet, "/api/agents/templates?q=zzz-nothing-matches-this", "")
	if status != http.StatusOK || dataOf(t, env)["count"].(float64) != 0 {
		t.Fatalf("empty search: status=%d body=%v", status, env)
	}
}

func TestNotificationPreferencesRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	status, env := call(t, srv, http.MethodGet, "/api/notifications/preferences", "")
	if status != http.StatusOK {
		t.Fatalf("get preferences: status=%d body=%v", status, env)
	}
	prefs := dataOf(t, env)
	prefs["quiet_hours_enabled"] = true
	prefs["min_push_severity"] = "warning"
	body, _ := json.Marshal(prefs)

	if status, env = call(t, srv, http.MethodPut, "/api/notifications/preferences", string(body)); status != http.StatusOK {
		t.Fatalf("save preferences: status=%d body=%v", status, env)
	}
	status, env = call(t, srv, http.MethodGet, "/api/notifications/preferences", "")
	if status != http.StatusOK {
		t.Fatalf("re-read preferences: status=%d", status)
	}
	got := dataOf(t, env)
	if got["quiet_hours_enabled"] != true || got["min_push_severity"] != "warning" {
		t.Fatalf("preferences were not persisted: %v", got)
	}

	if status, _ = call(t, srv, http.MethodPut, "/api/notifications/preferences", `{broken`); status != http.StatusBadRequest {
		t.Fatalf("malformed preferences: status=%d, want 400", status)
	}
}

func TestOptionalEnginesDegradeGracefully(t *testing.T) {
	srv := newTestServer(t)
	srv.proactiveEngine = nil
	srv.reflectionEngine = nil

	// Read endpoints report an empty/default state instead of failing.
	for _, path := range []string{
		"/api/ops/anomalies",
		"/api/ops/anomalies/config",
		"/api/agents/a/insights",
	} {
		if status, _ := call(t, srv, http.MethodGet, path, ""); status != http.StatusOK {
			t.Errorf("GET %s: status=%d, want 200", path, status)
		}
	}
	// Mutations make the missing dependency explicit.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/ops/anomalies/x/act", `{"action":"resolve"}`},
		{http.MethodPost, "/api/ops/anomalies/scan", ""},
		{http.MethodPut, "/api/ops/anomalies/config", `{"enabled":true}`},
		{http.MethodPost, "/api/agents/a/insights/i/apply", ""},
		{http.MethodPost, "/api/agents/a/insights/i/dismiss", ""},
		{http.MethodPost, "/api/agents/a/insights/self-review", ""},
	} {
		if status, _ := call(t, srv, tc.method, tc.path, tc.body); status != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status=%d, want 503", tc.method, tc.path, status)
		}
	}
}

func TestListBackupsStartsEmpty(t *testing.T) {
	srv := newTestServer(t)
	status, env := call(t, srv, http.MethodGet, "/api/system/backups", "")
	if status != http.StatusOK {
		t.Fatalf("list backups: status=%d body=%v", status, env)
	}
	if count := dataOf(t, env)["count"].(float64); count != 0 {
		t.Fatalf("fresh install should have no backups, got %v", count)
	}
}

func TestBackupLifecycle(t *testing.T) {
	srv := newTestServer(t)

	status, env := call(t, srv, http.MethodPost, "/api/system/backup", `{"notes":"before upgrade"}`)
	if status != http.StatusOK {
		t.Fatalf("create backup: status=%d body=%v", status, env)
	}
	manifest, _ := dataOf(t, env)["manifest"].(map[string]any)
	id, _ := manifest["id"].(string)
	if id == "" {
		t.Fatalf("backup manifest has no id: %v", manifest)
	}

	status, env = call(t, srv, http.MethodGet, "/api/system/backups", "")
	if status != http.StatusOK || dataOf(t, env)["count"].(float64) != 1 {
		t.Fatalf("list after create: status=%d body=%v", status, env)
	}

	if status, env = call(t, srv, http.MethodDelete, "/api/system/backups/"+id, ""); status != http.StatusOK {
		t.Fatalf("delete backup: status=%d body=%v", status, env)
	}
	status, env = call(t, srv, http.MethodGet, "/api/system/backups", "")
	if status != http.StatusOK || dataOf(t, env)["count"].(float64) != 0 {
		t.Fatalf("list after delete: status=%d body=%v", status, env)
	}
}

func TestAuditExportFormats(t *testing.T) {
	srv := newTestServer(t)

	csvReq := httptest.NewRecorder()
	srv.Router().ServeHTTP(csvReq, httptest.NewRequest(http.MethodGet, "/api/system/audit/export", nil))
	if csvReq.Code != http.StatusOK || csvReq.Header().Get("Content-Type") != "text/csv" {
		t.Fatalf("csv export: status=%d type=%q", csvReq.Code, csvReq.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(csvReq.Body.Bytes(), []byte("Timestamp,TraceID,AgentID")) {
		t.Fatalf("csv export is missing its header row: %q", csvReq.Body.String())
	}

	jsonReq := httptest.NewRecorder()
	srv.Router().ServeHTTP(jsonReq, httptest.NewRequest(http.MethodGet, "/api/system/audit/export?format=JSON", nil))
	if jsonReq.Code != http.StatusOK || jsonReq.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("json export: status=%d type=%q", jsonReq.Code, jsonReq.Header().Get("Content-Type"))
	}
	if !json.Valid(jsonReq.Body.Bytes()) {
		t.Fatalf("json export is not valid JSON: %q", jsonReq.Body.String())
	}
}

func TestSystemStatusEndpoints(t *testing.T) {
	srv := newTestServer(t)

	status, env := call(t, srv, http.MethodGet, "/api/llm/health", "")
	if status != http.StatusOK {
		t.Fatalf("llm health: status=%d body=%v", status, env)
	}
	if _, ok := dataOf(t, env)["providers"]; !ok {
		t.Fatalf("llm health has no providers field: %v", env)
	}

	if status, env = call(t, srv, http.MethodGet, "/api/system/ota/status", ""); status != http.StatusOK {
		t.Fatalf("ota status: status=%d body=%v", status, env)
	}

	srv.llmRouter = nil
	if status, _ = call(t, srv, http.MethodGet, "/api/llm/health", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("llm health without router: status=%d, want 503", status)
	}
}

func TestCancelUnknownRunIsRejected(t *testing.T) {
	srv := newTestServer(t)
	if status, _ := call(t, srv, http.MethodPost, "/api/runs/no-such-run/cancel", ""); status != http.StatusBadRequest {
		t.Fatalf("cancel unknown run: status=%d, want 400", status)
	}
}

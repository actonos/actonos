package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/system"
)

func TestRealtimeSnapshotIncludesOldRunningRun(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	old, err := srv.runStore.Start(ctx, "trace-old", "agent_system_core", "long running heartbeat", "heartbeat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.memory.DB().SQLDB().ExecContext(ctx, `UPDATE agent_runs SET started_at = datetime('now', '-2 day') WHERE id = ?`, old.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 31; i++ {
		run, err := srv.runStore.Start(ctx, "trace", "agent_system_core", "done", "chat")
		if err != nil {
			t.Fatal(err)
		}
		run.Status = agent.RunCompleted
		run.TerminationReason = "goal_completed"
		if err := srv.runStore.Finish(ctx, run); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := srv.collectRealtimeSnapshot(ctx)
	runs, ok := snapshot.Runs.([]agent.AgentRun)
	if !ok {
		t.Fatalf("runs type %T", snapshot.Runs)
	}
	found := false
	for _, run := range runs {
		if run.ID == old.ID && run.Status == agent.RunRunning {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("old running run %s missing from snapshot (%d rows)", old.ID, len(runs))
	}
}

func TestGetAgentRunFoundAndMissing(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	run, err := srv.runStore.Start(ctx, "trace", "agent_system_core", "goal", "chat")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID, nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET run: %d %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data struct {
			Run agent.AgentRun `json:"run"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Run.ID != run.ID {
		t.Fatalf("got %+v", envelope.Data.Run)
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/runs/run_does_not_exist", nil)
	mw := httptest.NewRecorder()
	srv.Router().ServeHTTP(mw, missing)
	if mw.Code != http.StatusNotFound {
		t.Fatalf("missing run: %d %s", mw.Code, mw.Body.String())
	}
}

func TestHandleCheckOTAUsesInjectedGitHubJSON(t *testing.T) {
	srv := newTestServer(t)
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases" {
			t.Errorf("HTML /releases must not be fetched")
		}
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(gh.Close)
	eng := system.NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetAPIURL(gh.URL + "/repos/actonos/actonos/releases/latest")
	eng.SetSkipRestart(true)
	srv.ota = eng

	req := httptest.NewRequest(http.MethodPost, "/api/system/ota/check", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data system.CheckResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.UpdateAvailable {
		t.Fatal("429 must not look like an available update")
	}
	if envelope.Data.ErrorCode != system.ErrCodeRateLimit {
		t.Fatalf("error_code = %q", envelope.Data.ErrorCode)
	}
}

package server

import (
	"net/http"
	"testing"
)

func TestConversationLifecycleEndpoints(t *testing.T) {
	srv := newTestServer(t)

	status, env := call(t, srv, http.MethodPost, "/api/conversations", `{"title":"Planning"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: status=%d body=%v", status, env)
	}
	id, _ := dataOf(t, env)["id"].(string)
	if id == "" {
		t.Fatalf("create returned no id: %v", env)
	}
	path := "/api/conversations/" + id

	if status, env = call(t, srv, http.MethodPut, path, `{"title":"Renamed","is_pinned":true}`); status != http.StatusOK {
		t.Fatalf("update title+pin: status=%d body=%v", status, env)
	}
	if status, env = call(t, srv, http.MethodPut, path, `{"title":"Renamed again"}`); status != http.StatusOK {
		t.Fatalf("update title: status=%d body=%v", status, env)
	}
	if status, env = call(t, srv, http.MethodPut, path, `{"is_pinned":false}`); status != http.StatusOK {
		t.Fatalf("update pin: status=%d body=%v", status, env)
	}
	if status, _ = call(t, srv, http.MethodPut, path, `{broken`); status != http.StatusBadRequest {
		t.Fatalf("malformed update: status=%d, want 400", status)
	}

	status, env = call(t, srv, http.MethodPut, path+"/pin", `{"is_pinned":true}`)
	if status != http.StatusOK || dataOf(t, env)["is_pinned"] != true {
		t.Fatalf("pin: status=%d body=%v", status, env)
	}
	if status, _ = call(t, srv, http.MethodPut, path+"/pin", `{nope`); status != http.StatusBadRequest {
		t.Fatalf("malformed pin: status=%d, want 400", status)
	}

	status, env = call(t, srv, http.MethodGet, path, "")
	conv, _ := dataOf(t, env)["conversation"].(map[string]any)
	if status != http.StatusOK || conv["title"] != "Renamed again" || conv["is_pinned"] != true {
		t.Fatalf("get after updates: status=%d conv=%v", status, conv)
	}

	if status, _ = call(t, srv, http.MethodDelete, path, ""); status != http.StatusOK {
		t.Fatalf("delete: status=%d", status)
	}
	if status, _ = call(t, srv, http.MethodGet, path, ""); status != http.StatusNotFound {
		t.Fatalf("get after delete: status=%d, want 404", status)
	}
}

func TestFactoryResetRequiresConfirmationToken(t *testing.T) {
	srv := newTestServer(t)
	_, env := call(t, srv, http.MethodPost, "/api/conversations", `{"title":"Keep until confirmed"}`)
	id, _ := dataOf(t, env)["id"].(string)

	for _, body := range []string{`{}`, `{"confirm_token":"nope"}`, `{"confirm_token":"reset-actonos"}`} {
		if status, _ := call(t, srv, http.MethodPost, "/api/system/factory-reset", body); status != http.StatusForbidden {
			t.Errorf("factory reset with %s: status=%d, want 403", body, status)
		}
	}
	if status, _ := call(t, srv, http.MethodPost, "/api/system/factory-reset", `{broken`); status != http.StatusBadRequest {
		t.Errorf("malformed factory reset: status=%d, want 400", status)
	}
	if status, _ := call(t, srv, http.MethodGet, "/api/conversations/"+id, ""); status != http.StatusOK {
		t.Fatalf("a rejected reset must not delete data: status=%d", status)
	}

	status, env := call(t, srv, http.MethodPost, "/api/system/factory-reset", `{"confirm_token":"RESET-ACTONOS"}`)
	if status != http.StatusOK || dataOf(t, env)["status"] != "reset_complete" {
		t.Fatalf("confirmed reset: status=%d body=%v", status, env)
	}
	if status, _ := call(t, srv, http.MethodGet, "/api/conversations/"+id, ""); status != http.StatusNotFound {
		t.Fatalf("conversation survived the factory reset: status=%d", status)
	}
}

func TestOTAApplyAndRollbackRequireApproval(t *testing.T) {
	srv := newTestServer(t)
	for _, path := range []string{"/api/system/ota/apply", "/api/system/ota/rollback"} {
		status, env := call(t, srv, http.MethodPost, path, "")
		if status != http.StatusAccepted {
			t.Fatalf("%s: status=%d, want 202 (approval required): %v", path, status, env)
		}
		if dataOf(t, env)["status"] != "approval_required" {
			t.Fatalf("%s must not run without approval: %v", path, env)
		}
	}
}

func TestNotificationMaintenanceEndpoints(t *testing.T) {
	srv := newTestServer(t)

	for _, body := range []string{`{"all":true}`, `{"id":"all"}`, `{}`, `{"id":"does-not-exist"}`} {
		status, env := call(t, srv, http.MethodPost, "/api/notifications/mark-read", body)
		if status != http.StatusOK && status != http.StatusInternalServerError {
			t.Errorf("mark-read %s: unexpected status %d: %v", body, status, env)
		}
	}
	if status, _ := call(t, srv, http.MethodPost, "/api/notifications/mark-read", `{broken`); status != http.StatusBadRequest {
		t.Errorf("malformed mark-read: status=%d, want 400", status)
	}

	for _, path := range []string{"/api/notifications?all=true", "/api/notifications?all=1", "/api/notifications?id=all", "/api/notifications?id=missing", "/api/notifications"} {
		if status, env := call(t, srv, http.MethodDelete, path, ""); status != http.StatusOK && status != http.StatusInternalServerError {
			t.Errorf("DELETE %s: unexpected status %d: %v", path, status, env)
		}
	}

	if status, env := call(t, srv, http.MethodPost, "/api/notifications/digest/trigger", ""); status != http.StatusOK {
		t.Fatalf("digest trigger: status=%d body=%v", status, env)
	}
}

package server

import (
	"net/http"
	"testing"
)

func TestMailcalAccountLifecycleOverHTTP(t *testing.T) {
	srv := newTestServer(t)

	status, env := call(t, srv, http.MethodGet, "/api/mailcal/providers", "")
	providers, _ := dataOf(t, env)["providers"].([]any)
	if status != http.StatusOK || len(providers) != 1 {
		t.Fatalf("providers: status=%d body=%v", status, env)
	}

	status, env = call(t, srv, http.MethodPost, "/api/mailcal/accounts",
		`{"label":"Work","plugin_id":"refmem","kind":"both","email":"Me@Example.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: status=%d body=%v", status, env)
	}
	acct := dataOf(t, env)
	id, _ := acct["id"].(string)
	if id == "" || acct["email"] != "me@example.com" || acct["status"] != "ok" {
		t.Fatalf("unexpected account %v", acct)
	}

	if _, err := srv.toolRegistryForTest().Get("native_mail_list"); err != nil {
		t.Fatalf("creating the first account must publish the tools: %v", err)
	}

	status, env = call(t, srv, http.MethodPost, "/api/mailcal/accounts/"+id+"/test", "")
	if status != http.StatusOK || dataOf(t, env)["ok"] != true {
		t.Fatalf("test: status=%d body=%v", status, env)
	}

	status, env = call(t, srv, http.MethodPut, "/api/mailcal/accounts/"+id, `{"label":"Personal","email":"other@example.com","config":{"calendar":"home"}}`)
	if status != http.StatusOK || dataOf(t, env)["label"] != "Personal" {
		t.Fatalf("update: status=%d body=%v", status, env)
	}

	status, env = call(t, srv, http.MethodGet, "/api/mailcal/status", "")
	if status != http.StatusOK || dataOf(t, env)["accounts"].(float64) != 1 {
		t.Fatalf("status: %d %v", status, env)
	}

	if status, _ = call(t, srv, http.MethodDelete, "/api/mailcal/accounts/"+id, ""); status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}
	if _, err := srv.toolRegistryForTest().Get("native_mail_list"); err == nil {
		t.Fatal("deleting the last account must withdraw the tools")
	}
	if status, _ = call(t, srv, http.MethodGet, "/api/mailcal/accounts/"+id, ""); status != http.StatusNotFound {
		t.Fatalf("get after delete: %d", status)
	}
}

func TestMailcalAccountValidationOverHTTP(t *testing.T) {
	srv := newTestServer(t)
	for name, body := range map[string]string{
		"unknown plugin": `{"label":"x","plugin_id":"nope","kind":"mail"}`,
		"bad kind":       `{"label":"x","plugin_id":"refmem","kind":"fax"}`,
		"no label":       `{"plugin_id":"refmem","kind":"mail"}`,
		"secret config":  `{"label":"x","plugin_id":"refmem","kind":"mail","config":{"password":"hunter2"}}`,
		"bad email":      `{"label":"x","plugin_id":"refmem","kind":"mail","email":"nope"}`,
		"malformed":      `{`,
	} {
		if status, _ := call(t, srv, http.MethodPost, "/api/mailcal/accounts", body); status != http.StatusBadRequest {
			t.Errorf("%s: status=%d, want 400", name, status)
		}
	}
	for _, path := range []string{"/api/mailcal/accounts/acct_nope", "/api/mailcal/drafts/draft_nope"} {
		if status, _ := call(t, srv, http.MethodGet, path, ""); status != http.StatusNotFound {
			t.Errorf("GET %s: status=%d, want 404", path, status)
		}
	}
}

func TestMailcalIsUnavailableWithoutAService(t *testing.T) {
	srv := newTestServer(t)
	srv.mailcal = nil
	for _, path := range []string{"/api/mailcal/status", "/api/mailcal/accounts", "/api/mailcal/providers"} {
		if status, _ := call(t, srv, http.MethodGet, path, ""); status != http.StatusServiceUnavailable {
			t.Errorf("GET %s: status=%d, want 503", path, status)
		}
	}
}

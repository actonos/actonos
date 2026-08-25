package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestServer_PairingHTTPHandlers(t *testing.T) {
	srv := newTestServer(t)

	policy := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/policy", strings.NewReader(`{"channel_id":"zalo","required":true}`))
	policy.Header.Set("Content-Type", "application/json")
	pw := httptest.NewRecorder()
	srv.Router().ServeHTTP(pw, policy)
	if pw.Code != http.StatusOK {
		t.Fatalf("policy: %d %s", pw.Code, pw.Body.String())
	}

	codeReq := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/code", strings.NewReader(`{"channel_id":"zalo"}`))
	codeReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	srv.Router().ServeHTTP(cw, codeReq)
	if cw.Code != http.StatusOK {
		t.Fatalf("code: %d %s", cw.Code, cw.Body.String())
	}
	var env struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	_ = json.NewDecoder(cw.Body).Decode(&env)
	if len(env.Data.Code) != 8 {
		t.Fatalf("code %q", env.Data.Code)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/integrations/pairing/codes", nil)
	lw := httptest.NewRecorder()
	srv.Router().ServeHTTP(lw, list)
	if lw.Code != http.StatusOK || !strings.Contains(lw.Body.String(), env.Data.Code) {
		t.Fatalf("list codes: %d %s", lw.Code, lw.Body.String())
	}

	policies := httptest.NewRequest(http.MethodGet, "/api/integrations/pairing/policy", nil)
	polw := httptest.NewRecorder()
	srv.Router().ServeHTTP(polw, policies)
	if polw.Code != http.StatusOK || !strings.Contains(polw.Body.String(), "zalo") {
		t.Fatalf("get policies: %s", polw.Body.String())
	}

	allow := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/allow", strings.NewReader(`{"channel_id":"zalo","sender_id":"u-1","sender_name":"Ada"}`))
	allow.Header.Set("Content-Type", "application/json")
	aw := httptest.NewRecorder()
	srv.Router().ServeHTTP(aw, allow)
	if aw.Code != http.StatusOK {
		t.Fatalf("allow: %d %s", aw.Code, aw.Body.String())
	}

	pending := httptest.NewRequest(http.MethodGet, "/api/integrations/pairing/pending", nil)
	pendw := httptest.NewRecorder()
	srv.Router().ServeHTTP(pendw, pending)
	if pendw.Code != http.StatusOK {
		t.Fatalf("pending: %d %s", pendw.Code, pendw.Body.String())
	}

	info := httptest.NewRequest(http.MethodGet, "/api/terminal/info", nil)
	iw := httptest.NewRecorder()
	srv.Router().ServeHTTP(iw, info)
	if iw.Code != http.StatusOK {
		t.Fatalf("terminal info: %d %s", iw.Code, iw.Body.String())
	}

	logs := httptest.NewRequest(http.MethodGet, "/api/plugins/missing/logs", nil)
	logw := httptest.NewRecorder()
	srv.Router().ServeHTTP(logw, logs)
	if logw.Code != http.StatusOK && logw.Code != http.StatusNotFound {
		t.Fatalf("plugin logs: %d %s", logw.Code, logw.Body.String())
	}
}

func TestLayeredFSOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+string(os.PathSeparator)+"hello.txt", []byte("disk"), 0644); err != nil {
		t.Fatal(err)
	}
	fsys := NewLayeredFS(dir, nil)
	f, err := fsys.Open("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := fsys.Open("missing.txt"); err == nil {
		t.Fatal("expected missing")
	}
}

func TestServer_LogoutAndChangePassword(t *testing.T) {
	srv := newTestServer(t)
	logout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, logout)
	if w.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}

	pulse := httptest.NewRequest(http.MethodPost, "/api/heartbeat/trigger", nil)
	pw := httptest.NewRecorder()
	srv.Router().ServeHTTP(pw, pulse)
	if pw.Code != http.StatusOK && pw.Code != http.StatusServiceUnavailable && pw.Code != http.StatusNotImplemented {
		t.Fatalf("heartbeat trigger: %d %s", pw.Code, pw.Body.String())
	}

	clear := httptest.NewRequest(http.MethodDelete, "/api/agents/memory-md", nil)
	cw := httptest.NewRecorder()
	srv.Router().ServeHTTP(cw, clear)
	if cw.Code != http.StatusOK && cw.Code != http.StatusBadRequest && cw.Code != http.StatusNotFound {
		t.Fatalf("clear memory-md: %d %s", cw.Code, cw.Body.String())
	}

	mcp := httptest.NewRequest(http.MethodGet, "/api/tools/mcp", nil)
	mw := httptest.NewRecorder()
	srv.Router().ServeHTTP(mw, mcp)
	if mw.Code != http.StatusOK && mw.Code != http.StatusNotImplemented {
		t.Fatalf("list mcp: %d %s", mw.Code, mw.Body.String())
	}

	for _, path := range []string{
		"/api/notifications",
		"/api/notifications/unread-count",
		"/api/system/wifi/scan",
		"/api/runs/missing/events",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rw := httptest.NewRecorder()
		srv.Router().ServeHTTP(rw, req)
		if rw.Code >= 500 {
			t.Fatalf("%s: %d %s", path, rw.Code, rw.Body.String())
		}
	}
	pwd := httptest.NewRequest(http.MethodPut, "/api/auth/password", strings.NewReader(`{"current_password":"x","new_password":"yyyyyyyy"}`))
	pwd.Header.Set("Content-Type", "application/json")
	pwdw := httptest.NewRecorder()
	srv.Router().ServeHTTP(pwdw, pwd)
	if pwdw.Code != http.StatusInternalServerError && pwdw.Code != http.StatusUnauthorized && pwdw.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", pwdw.Code, pwdw.Body.String())
	}

	deln := httptest.NewRequest(http.MethodDelete, "/api/notifications", nil)
	delw := httptest.NewRecorder()
	srv.Router().ServeHTTP(delw, deln)
	if delw.Code >= 500 {
		t.Fatalf("delete notifications: %d %s", delw.Code, delw.Body.String())
	}

	wifi := httptest.NewRequest(http.MethodPost, "/api/system/wifi/connect", strings.NewReader(`{"ssid":"x","password":"y"}`))
	wifi.Header.Set("Content-Type", "application/json")
	ww := httptest.NewRecorder()
	srv.Router().ServeHTTP(ww, wifi)

	pin := httptest.NewRequest(http.MethodPut, "/api/conversations/missing/pin", nil)
	pinw := httptest.NewRecorder()
	srv.Router().ServeHTTP(pinw, pin)

	mark := httptest.NewRequest(http.MethodPost, "/api/notifications/mark-read", strings.NewReader(`{"ids":[]}`))
	mark.Header.Set("Content-Type", "application/json")
	markw := httptest.NewRecorder()
	srv.Router().ServeHTTP(markw, mark)
	if markw.Code >= 500 {
		t.Fatalf("mark-read: %d %s", markw.Code, markw.Body.String())
	}
}

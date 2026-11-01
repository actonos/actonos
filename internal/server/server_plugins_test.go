package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/plugin"
)

func TestServer_PluginsEndpoint(t *testing.T) {
	srv := newTestServer(t)

	// 1. GET /api/plugins (empty)
	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshaling json response: %v", err)
	}
	data, ok := res["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data wrapper in response: %v", res)
	}
	if count, ok := data["count"].(float64); !ok || count != 0 {
		t.Errorf("expected count 0, got %v", data["count"])
	}

	// 2. POST /api/plugins/upload multipart
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test_plugin.wasm")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	// minimal wasm binary
	minimalWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	_, _ = part.Write(minimalWasm)
	_ = writer.WriteField("id", "test_plugin")
	_ = writer.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadW := httptest.NewRecorder()
	srv.Router().ServeHTTP(uploadW, uploadReq)

	if uploadW.Code != http.StatusOK {
		t.Fatalf("expected 200 from upload, got %d: %s", uploadW.Code, uploadW.Body.String())
	}

	// 3. GET /api/plugins (now has 1)
	req2 := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	var res2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &res2)
	data2 := res2["data"].(map[string]any)
	if count, ok := data2["count"].(float64); !ok || count != 1 {
		t.Errorf("expected count 1, got %v", data2["count"])
	}

	// 4. POST /api/plugins/{id}/disable
	disReq := httptest.NewRequest(http.MethodPost, "/api/plugins/test_plugin/disable", nil)
	disW := httptest.NewRecorder()
	srv.Router().ServeHTTP(disW, disReq)
	if disW.Code != http.StatusOK {
		t.Fatalf("expected 200 from disable, got %d: %s", disW.Code, disW.Body.String())
	}

	// 5. POST /api/plugins/{id}/enable
	enReq := httptest.NewRequest(http.MethodPost, "/api/plugins/test_plugin/enable", nil)
	enW := httptest.NewRecorder()
	srv.Router().ServeHTTP(enW, enReq)
	if enW.Code != http.StatusOK {
		t.Fatalf("expected 200 from enable, got %d: %s", enW.Code, enW.Body.String())
	}

	// 6. DELETE /api/plugins/{id}
	delReq := httptest.NewRequest(http.MethodDelete, "/api/plugins/test_plugin", nil)
	delW := httptest.NewRecorder()
	srv.Router().ServeHTTP(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("expected 200 from delete, got %d: %s", delW.Code, delW.Body.String())
	}
}

func TestServer_PluginsEndpoint_ActonPkg(t *testing.T) {
	srv := newTestServer(t)

	// Create an in-memory .actonpkg zip bundle
	var pkgBuf bytes.Buffer
	zw := zip.NewWriter(&pkgBuf)

	manifestData := []byte(`{
		"id": "channel_discord",
		"name": "Discord Channel Gateway",
		"version": "1.0.0",
		"capabilities": ["channel"],
		"permissions": {
			"net_outbound": ["discord.com"]
		}
	}`)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("creating manifest in zip: %v", err)
	}
	_, _ = mw.Write(manifestData)

	minimalWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	ww, err := zw.Create("plugin.wasm")
	if err != nil {
		t.Fatalf("creating wasm in zip: %v", err)
	}
	_, _ = ww.Write(minimalWasm)

	_ = zw.Close()

	// Upload .actonpkg
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "channel-discord-1.0.0.actonpkg")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	_, _ = part.Write(pkgBuf.Bytes())
	_ = writer.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadW := httptest.NewRecorder()
	srv.Router().ServeHTTP(uploadW, uploadReq)

	if uploadW.Code != http.StatusOK {
		t.Fatalf("expected 200 from upload .actonpkg, got %d: %s", uploadW.Code, uploadW.Body.String())
	}

	var uploadResp map[string]any
	_ = json.Unmarshal(uploadW.Body.Bytes(), &uploadResp)
	data := uploadResp["data"].(map[string]any)
	if status, ok := data["status"].(string); !ok || status != "installed" {
		t.Errorf("expected status 'installed', got %v", data["status"])
	}

	// Verify plugin list has channel_discord
	listReq := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	listW := httptest.NewRecorder()
	srv.Router().ServeHTTP(listW, listReq)
	var listResp map[string]any
	_ = json.Unmarshal(listW.Body.Bytes(), &listResp)
	listData := listResp["data"].(map[string]any)
	if count, ok := listData["count"].(float64); !ok || count != 1 {
		t.Errorf("expected count 1, got %v", listData["count"])
	}
}

func TestServer_PluginConfigEndpoint(t *testing.T) {
	srv := newTestServer(t)

	// Create and upload valid .actonpkg
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	manifestContent := `{
		"id": "test-channel-discord",
		"name": "Discord Test Bot",
		"version": "1.0.0",
		"capabilities": ["channel"],
		"permissions": {
			"net_outbound": ["discord.com"],
			"secrets": ["discord_bot_token"]
		},
		"config_schema": {
			"type": "object",
			"properties": {
				"poll_interval": {
					"type": "integer",
					"default": 5
				}
			}
		}
	}`
	fManifest, _ := zw.Create("manifest.json")
	_, _ = fManifest.Write([]byte(manifestContent))

	fWasm, _ := zw.Create("plugin.wasm")
	_, _ = fWasm.Write([]byte("\x00asm\x01\x00\x00\x00"))
	_ = zw.Close()

	// 1. Upload plugin
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "discord.actonpkg")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	reqUpload := httptest.NewRequest(http.MethodPost, "/api/plugins/upload", body)
	reqUpload.Header.Set("Content-Type", writer.FormDataContentType())
	wUpload := httptest.NewRecorder()
	srv.Router().ServeHTTP(wUpload, reqUpload)
	if wUpload.Code != http.StatusOK {
		t.Fatalf("expected 200 for package upload, got %d: %s", wUpload.Code, wUpload.Body.String())
	}

	// 2. Update config and vault secret via POST /api/plugins/{id}/config
	configPayload := `{
		"config": {
			"poll_interval": 15,
			"custom_prefix": "!"
		},
		"secrets": {
			"discord_bot_token": "token_123456789"
		}
	}`
	reqConfig := httptest.NewRequest(http.MethodPost, "/api/plugins/test-channel-discord/config", strings.NewReader(configPayload))
	reqConfig.Header.Set("Content-Type", "application/json")
	wConfig := httptest.NewRecorder()
	srv.Router().ServeHTTP(wConfig, reqConfig)

	if wConfig.Code != http.StatusOK {
		t.Fatalf("expected 200 for plugin config update, got %d: %s", wConfig.Code, wConfig.Body.String())
	}

	// 3. Verify that secret was stored in Vault
	reqSecret := httptest.NewRequest(http.MethodGet, "/api/vault/secrets/discord_bot_token", nil)
	wSecret := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSecret, reqSecret)
	if wSecret.Code != http.StatusOK {
		t.Fatalf("expected 200 for vault secret verification, got %d", wSecret.Code)
	}
}

func TestServer_AvailablePlugins(t *testing.T) {
	srv := newTestServer(t)

	// Mock registry server
	mockRegistryServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"schema_version": "1.0.0",
			"plugins": []map[string]any{
				{
					"id":           "test_installed_plugin",
					"name":         "Test Installed Plugin",
					"version":      "1.0.0",
					"capabilities": []string{"tool"},
				},
				{
					"id":           "channel-discord",
					"name":         "Discord Bot Channel",
					"version":      "1.0.0",
					"capabilities": []string{"channel"},
				},
			},
		})
	}))
	defer mockRegistryServer.Close()

	if srv.pluginHubMgr != nil {
		srv.pluginHubMgr.SetRegistryURLs(mockRegistryServer.URL, mockRegistryServer.URL)
		_ = srv.pluginHubMgr.FetchRemoteCatalog(context.Background())
	}

	// 1. GET /api/plugins/available
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/available", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/plugins/available, got %d: %s", w.Code, w.Body.String())
	}

	var envelope struct {
		Data struct {
			Catalog []plugin.RegistryPlugin `json:"catalog"`
			Count   int                     `json:"count"`
		} `json:"data"`
		Catalog []plugin.RegistryPlugin `json:"catalog"`
		Count   int                     `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode catalog response: %v", err)
	}
	catalog := envelope.Data.Catalog
	if len(catalog) == 0 {
		catalog = envelope.Catalog
	}
	if len(catalog) == 0 {
		t.Fatalf("expected non-empty available catalog")
	}

	// 2. Mock asset server for installation test
	var pkgBuf bytes.Buffer
	zw := zip.NewWriter(&pkgBuf)
	manifestData := []byte(`{
		"id": "test_installed_plugin",
		"name": "Test Installed Plugin",
		"version": "1.0.0",
		"capabilities": ["tool"]
	}`)
	mf, _ := zw.Create("manifest.json")
	_, _ = mf.Write(manifestData)
	wasm := []byte("\x00asm\x01\x00\x00\x00")
	wf, _ := zw.Create("plugin.wasm")
	_, _ = wf.Write(wasm)
	_ = zw.Close()

	assetServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/zip")
		_, _ = rw.Write(pkgBuf.Bytes())
	}))
	defer assetServer.Close()

	// 3. POST /api/plugins/install
	installBody := fmt.Sprintf(`{"plugin_id": "test_installed_plugin", "download_url": "%s/test.actonpkg"}`, assetServer.URL)
	installReq := httptest.NewRequest(http.MethodPost, "/api/plugins/install", strings.NewReader(installBody))
	installReq.Header.Set("Content-Type", "application/json")
	installW := httptest.NewRecorder()
	srv.Router().ServeHTTP(installW, installReq)

	if installW.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/plugins/install, got %d: %s", installW.Code, installW.Body.String())
	}

	// 4. GET /api/plugins/available should now mark it as installed
	reqAfter := httptest.NewRequest(http.MethodGet, "/api/plugins/available", nil)
	wAfter := httptest.NewRecorder()
	srv.Router().ServeHTTP(wAfter, reqAfter)

	var envAfter struct {
		Data struct {
			Catalog []plugin.RegistryPlugin `json:"catalog"`
			Count   int                     `json:"count"`
		} `json:"data"`
		Catalog []plugin.RegistryPlugin `json:"catalog"`
	}
	_ = json.NewDecoder(wAfter.Body).Decode(&envAfter)
	catAfter := envAfter.Data.Catalog
	if len(catAfter) == 0 {
		catAfter = envAfter.Catalog
	}
	foundInstalled := false
	for _, p := range catAfter {
		if p.ID == "test_installed_plugin" && p.Installed {
			foundInstalled = true
			break
		}
	}
	if !foundInstalled {
		// Also verify via GET /api/plugins
		listReq := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
		listW := httptest.NewRecorder()
		srv.Router().ServeHTTP(listW, listReq)
		if !strings.Contains(listW.Body.String(), "test_installed_plugin") {
			t.Errorf("installed plugin was not registered in /api/plugins: %s", listW.Body.String())
		}
	}
}

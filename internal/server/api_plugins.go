package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/actonos/actonos/internal/plugin"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"plugins": []plugin.PluginInfo{},
			"count":   0,
		})
		return
	}

	plugins := s.pluginMgr.ListPlugins()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"plugins": plugins,
		"count":   len(plugins),
	})
}

func (s *Server) handleListAvailablePlugins(w http.ResponseWriter, r *http.Request) {
	if s.pluginHubMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_HUB_UNAVAILABLE", "plugin registry manager is not configured")
		return
	}

	var installed []plugin.PluginInfo
	if s.pluginMgr != nil {
		installed = s.pluginMgr.ListPlugins()
	}

	catalog := s.pluginHubMgr.ListCatalog(r.Context(), installed)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"catalog": catalog,
		"count":   len(catalog),
	})
}

type installPluginRequest struct {
	PluginID    string `json:"plugin_id"`
	DownloadURL string `json:"download_url,omitempty"`
}

func (s *Server) handleInstallAvailablePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginHubMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_HUB_UNAVAILABLE", "plugin registry manager is not configured")
		return
	}

	var req installPluginRequest
	if err := s.decodeJSON(r, &req); err != nil || req.PluginID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "plugin_id is required")
		return
	}

	info, err := s.pluginHubMgr.InstallPlugin(r.Context(), req.PluginID, req.DownloadURL)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "INSTALL_PLUGIN_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "installed",
		"plugin": info,
	})
}

func (s *Server) handleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		s.respondError(w, http.StatusBadRequest, "PARSE_FORM_FAILED", err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "FILE_MISSING", "package file (.actonpkg) is required in 'file' form field")
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_FAILED", err.Error())
		return
	}

	fileName := filepath.Base(header.Filename)
	lowerName := strings.ToLower(fileName)
	isZip := strings.HasSuffix(lowerName, ".actonpkg") ||
		strings.HasSuffix(lowerName, ".zip") ||
		(len(fileBytes) >= 4 && string(fileBytes[:4]) == "PK\x03\x04")

	var (
		manifestBytes []byte
		wasmBytes     []byte
		sigBytes      []byte
		readmeBytes   []byte
		pluginID      string
	)

	if isZip {
		mBytes, wBytes, sBytes, rBytes, err := plugin.ExtractPluginPackage(fileBytes)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "INVALID_PACKAGE", fmt.Sprintf("failed to unpack .actonpkg: %v", err))
			return
		}
		manifestBytes = mBytes
		wasmBytes = wBytes
		sigBytes = sBytes
		readmeBytes = rBytes

		var manifest plugin.PluginManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			s.respondError(w, http.StatusBadRequest, "INVALID_MANIFEST", fmt.Sprintf("invalid manifest.json in package: %v", err))
			return
		}
		pluginID = manifest.ID
		if pluginID == "" {
			trimmed := strings.TrimSuffix(fileName, ".actonpkg")
			trimmed = strings.TrimSuffix(trimmed, ".zip")
			pluginID = trimmed
			manifest.ID = pluginID
			manifestBytes, _ = json.MarshalIndent(manifest, "", "  ")
		}
	} else if strings.HasSuffix(lowerName, ".wasm") {
		wasmBytes = fileBytes
		pluginID = strings.TrimSuffix(fileName, ".wasm")
		manifestJSON := r.FormValue("manifest")
		if manifestJSON != "" {
			manifestBytes = []byte(manifestJSON)
		} else {
			manifest := plugin.PluginManifest{
				ID:           pluginID,
				Name:         pluginID,
				Version:      "1.0.0",
				Capabilities: []string{string(plugin.CapabilityTool)},
				Tools: []plugin.PluginToolDef{
					{
						Name:        "wasm_" + pluginID,
						Description: fmt.Sprintf("WASM plugin tool (%s)", pluginID),
						Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
					},
				},
			}
			manifestBytes, _ = json.MarshalIndent(manifest, "", "  ")
		}
	} else {
		s.respondError(w, http.StatusBadRequest, "INVALID_FILE_TYPE", "only .actonpkg plugin packages are supported")
		return
	}

	if idField := r.FormValue("id"); idField != "" {
		pluginID = idField
	}

	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" || strings.Contains(pluginID, "..") || strings.ContainsAny(pluginID, `/\`) {
		s.respondError(w, http.StatusBadRequest, "INVALID_PLUGIN_ID", "plugin id contains invalid characters")
		return
	}

	pluginDir := filepath.Join(s.pluginsDir, pluginID)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		s.respondError(w, http.StatusInternalServerError, "CREATE_DIR_FAILED", err.Error())
		return
	}

	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.wasm"), wasmBytes, 0644); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_WASM_FAILED", err.Error())
		return
	}

	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifestBytes, 0644); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_MANIFEST_FAILED", err.Error())
		return
	}

	if len(sigBytes) > 0 {
		_ = os.WriteFile(filepath.Join(pluginDir, "signature.sig"), sigBytes, 0644)
	}
	if len(readmeBytes) > 0 {
		_ = os.WriteFile(filepath.Join(pluginDir, "README.md"), readmeBytes, 0644)
	}

	if err := s.pluginMgr.ScanAndLoadAll(r.Context()); err != nil {
		s.respondError(w, http.StatusInternalServerError, "LOAD_PLUGIN_FAILED", err.Error())
		return
	}

	info, _ := s.pluginMgr.GetPlugin(pluginID)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "installed",
		"plugin": info,
	})
}

func (s *Server) handleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	pluginID := chi.URLParam(r, "id")
	if pluginID == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_PLUGIN_ID", "plugin id is required")
		return
	}

	if err := s.pluginMgr.EnablePlugin(r.Context(), pluginID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "ENABLE_FAILED", err.Error())
		return
	}

	info, _ := s.pluginMgr.GetPlugin(pluginID)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "enabled",
		"plugin": info,
	})
}

func (s *Server) handleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	pluginID := chi.URLParam(r, "id")
	if pluginID == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_PLUGIN_ID", "plugin id is required")
		return
	}

	if err := s.pluginMgr.DisablePlugin(r.Context(), pluginID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DISABLE_FAILED", err.Error())
		return
	}

	info, _ := s.pluginMgr.GetPlugin(pluginID)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "disabled",
		"plugin": info,
	})
}

func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	pluginID := chi.URLParam(r, "id")
	if pluginID == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_PLUGIN_ID", "plugin id is required")
		return
	}

	if err := s.pluginMgr.UninstallPlugin(r.Context(), pluginID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "UNINSTALL_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "uninstalled",
		"id":     pluginID,
	})
}

func (s *Server) handleGetPluginLogs(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	pluginID := chi.URLParam(r, "id")
	info, found := s.pluginMgr.GetPlugin(pluginID)
	if !found {
		s.respondError(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "plugin not found")
		return
	}

	logs := s.pluginMgr.GetPluginLogs(pluginID)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"id":     pluginID,
		"status": info.Status,
		"logs":   logs,
	})
}

type updatePluginConfigRequest struct {
	Config  map[string]any    `json:"config"`
	Secrets map[string]string `json:"secrets"`
}

func (s *Server) handleUpdatePluginConfig(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	pluginID := chi.URLParam(r, "id")
	if pluginID == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_PLUGIN_ID", "plugin id is required")
		return
	}

	var req updatePluginConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		s.respondError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	// 1. Persist secrets securely to Hardware Vault
	if len(req.Secrets) > 0 && s.vault != nil {
		for secretKey, secretVal := range req.Secrets {
			if secretVal != "" {
				if err := s.vault.SetSecret(r.Context(), secretKey, secretVal); err != nil {
					s.respondError(w, http.StatusInternalServerError, "VAULT_SECRET_FAILED", fmt.Sprintf("failed to save secret %s: %v", secretKey, err))
					return
				}
			}
		}
	}

	// 2. Update config in manifest.json
	if s.pluginsDir != "" && req.Config != nil {
		pluginDir := filepath.Join(s.pluginsDir, pluginID)
		manifestPath := filepath.Join(pluginDir, "manifest.json")
		if manifestBytes, err := os.ReadFile(manifestPath); err == nil {
			var manifest plugin.PluginManifest
			if err := json.Unmarshal(manifestBytes, &manifest); err == nil {
				if manifest.Config == nil {
					manifest.Config = make(map[string]any)
				}
				for k, v := range req.Config {
					manifest.Config[k] = v
				}
				if updatedBytes, err := json.MarshalIndent(manifest, "", "  "); err == nil {
					_ = os.WriteFile(manifestPath, updatedBytes, 0600)
				}
			}
		}
	}

	// 3. Scan & hot-reload
	if err := s.pluginMgr.ScanAndLoadAll(r.Context()); err != nil {
		s.respondError(w, http.StatusInternalServerError, "RELOAD_FAILED", err.Error())
		return
	}

	info, found := s.pluginMgr.GetPlugin(pluginID)
	if !found {
		s.respondError(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", "plugin not found after reload")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "updated",
		"plugin": info,
	})
}


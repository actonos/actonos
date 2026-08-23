package server

import (
	"encoding/json"
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

func (s *Server) handleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "PLUGIN_MANAGER_UNAVAILABLE", "plugin manager is not configured")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.respondError(w, http.StatusBadRequest, "PARSE_FORM_FAILED", err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "FILE_MISSING", "wasm file is required in 'file' form field")
		return
	}
	defer file.Close()

	wasmBytes, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_FAILED", err.Error())
		return
	}

	fileName := filepath.Base(header.Filename)
	pluginID := strings.TrimSuffix(fileName, ".wasm")
	if idField := r.FormValue("id"); idField != "" {
		pluginID = idField
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

	manifestJSON := r.FormValue("manifest")
	if manifestJSON != "" {
		_ = os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifestJSON), 0644)
	} else {
		// Generate default manifest if none provided
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
		data, _ := json.MarshalIndent(manifest, "", "  ")
		_ = os.WriteFile(filepath.Join(pluginDir, "manifest.json"), data, 0644)
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

	s.respondJSON(w, http.StatusOK, map[string]any{
		"id":     pluginID,
		"status": info.Status,
		"logs":   []string{},
	})
}

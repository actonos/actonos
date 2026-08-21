package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/actonos/actonos/internal/tools"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	var toolList []tools.ToolInfo
	if category != "" {
		toolList = s.toolReg.ListByCategory(category)
	} else {
		toolList = s.toolReg.List()
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"tools": toolList,
		"count": len(toolList),
	})
}

func (s *Server) handleConnectMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcpHost == nil {
		s.respondError(w, http.StatusNotImplemented, "MCP_NOT_ENABLED", "mcp host is not configured")
		return
	}

	var cfg tools.MCPServerConfig
	if err := s.decodeJSON(r, &cfg); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if cfg.ID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "server id is required")
		return
	}
	normalized, err := json.Marshal(cfg)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	approvalID := r.URL.Query().Get("approval_id")
	if s.approvalMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "APPROVALS_NOT_ENABLED", "MCP connections require the approval manager")
		return
	}
	if approvalID == "" {
		request, requestErr := s.approvalMgr.Request(
			r.Context(), "", "agent_system_core", "system_mcp_connect", "High", normalized,
		)
		if requestErr != nil {
			s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", requestErr.Error())
			return
		}
		s.respondJSON(w, http.StatusAccepted, map[string]any{
			"status":   "approval_required",
			"approval": request,
		})
		return
	}
	if err := s.approvalMgr.ValidateApproved(r.Context(), approvalID, "agent_system_core", "system_mcp_connect", normalized); err != nil {
		s.respondError(w, http.StatusConflict, "APPROVAL_INVALID", err.Error())
		return
	}

	if err := s.mcpHost.ConnectServer(r.Context(), cfg); err != nil {
		s.respondError(w, http.StatusBadRequest, "MCP_CONNECT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "connected",
		"server_id": cfg.ID,
	})
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.mcpHost == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"servers": []any{}})
		return
	}
	items, err := s.mcpHost.ListServers(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "MCP_LIST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"servers": items})
}

func (s *Server) handleToggleMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.mcpHost == nil {
		s.respondError(w, http.StatusNotImplemented, "MCP_NOT_ENABLED", "mcp host is not configured")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	approval, err := s.requestAdminAction(r.Context(), "mcp_toggle", map[string]any{
		"server_id": chi.URLParam(r, "serverID"),
		"enabled":   req.Enabled,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleDisconnectMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcpHost == nil {
		s.respondError(w, http.StatusNotImplemented, "MCP_NOT_ENABLED", "mcp host is not configured")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	approval, err := s.requestAdminAction(r.Context(), "mcp_disconnect", map[string]string{"server_id": serverID})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string          `json:"name"`
		AgentID    string          `json:"agent_id"`
		ApprovalID string          `json:"approval_id,omitempty"`
		Input      json.RawMessage `json:"input"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "tool name is required")
		return
	}

	if req.AgentID == "" {
		req.AgentID = "agent_system_core"
	} else if req.AgentID != "agent_system_core" {
		s.respondError(w, http.StatusBadRequest, "INVALID_AGENT_ID", "direct tool execution is bound to the system agent")
		return
	}

	ctx := r.Context()
	if r.URL.Query().Get("test") == "true" || r.URL.Query().Get("bypass_approval") == "true" {
		ctx = tools.WithBypassApproval(ctx)
	}
	if req.ApprovalID != "" {
		ctx = tools.WithApprovalID(ctx, req.ApprovalID)
	}
	res, err := s.toolReg.Execute(ctx, req.AgentID, req.Name, req.Input)
	if err != nil {
		var approvalErr *tools.ApprovalRequiredError
		if errors.As(err, &approvalErr) {
			s.respondJSON(w, http.StatusAccepted, map[string]any{
				"status":   "approval_required",
				"approval": approvalErr.Approval,
			})
			return
		}
		s.respondError(w, http.StatusBadRequest, "TOOL_EXECUTION_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, res)
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Name == "" || strings.ContainsAny(req.Name, `/\`) {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "valid skill name is required")
		return
	}

	dir := filepath.Join(s.skillsDir, req.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DIRECTORY_CREATION_FAILED", err.Error())
		return
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", req.Name, req.Description, req.Content)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FILE_FAILED", err.Error())
		return
	}

	if s.skillWatcher != nil {
		s.skillWatcher.ScanAll()
	} else if s.toolReg != nil {
		if st, err := tools.NewSkillTool(dir); err == nil {
			s.toolReg.RegisterOrReplace(st)
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "created", "name": req.Name, "path": dir})
}

func (s *Server) handleUploadWASM(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "PARSE_FORM_FAILED", err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "FILE_MISSING", err.Error())
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 16<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_FAILED", err.Error())
		return
	}
	approval, err := s.requestAdminAction(r.Context(), "wasm_upload", map[string]string{
		"filename": filepath.Base(header.Filename),
		"data":     base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

// Hub Catalog & Marketplace Handlers
func (s *Server) handleListHubCatalog(w http.ResponseWriter, r *http.Request) {
	if s.hubMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "HUB_NOT_AVAILABLE", "hub manager not configured")
		return
	}

	catalog := s.hubMgr.ListCatalog()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"catalog": catalog,
		"count":   len(catalog),
	})
}

func (s *Server) handleInstallHubSkill(w http.ResponseWriter, r *http.Request) {
	if s.hubMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "HUB_NOT_AVAILABLE", "hub manager not configured")
		return
	}

	var req struct {
		SkillID string `json:"skill_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil || req.SkillID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "skill_id is required")
		return
	}

	if err := s.hubMgr.InstallSkill(req.SkillID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "INSTALL_FAILED", err.Error())
		return
	}
	if s.skillWatcher != nil {
		s.skillWatcher.ScanAll()
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "installed", "skill_id": req.SkillID})
}

func (s *Server) handleUninstallHubSkill(w http.ResponseWriter, r *http.Request) {
	if s.hubMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "HUB_NOT_AVAILABLE", "hub manager not configured")
		return
	}

	var req struct {
		SkillID string `json:"skill_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil || req.SkillID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "skill_id is required")
		return
	}

	if err := s.hubMgr.UninstallSkill(req.SkillID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "UNINSTALL_FAILED", err.Error())
		return
	}
	if s.skillWatcher != nil {
		s.skillWatcher.ScanAll()
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "uninstalled", "skill_id": req.SkillID})
}

func (s *Server) handleToggleSkill(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "skill name is required")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if s.toolReg == nil {
		s.respondError(w, http.StatusNotImplemented, "TOOLS_NOT_AVAILABLE", "tool registry not configured")
		return
	}

	err := s.toolReg.SetToolEnabled(name, req.Enabled)
	if err != nil && !strings.HasPrefix(name, "skill_") {
		err = s.toolReg.SetToolEnabled("skill_"+name, req.Enabled)
	}

	if err != nil {
		s.respondError(w, http.StatusNotFound, "SKILL_NOT_FOUND", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"name":    name,
		"enabled": req.Enabled,
		"status":  "updated",
	})
}


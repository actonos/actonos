package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

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

	if err := s.mcpHost.ConnectServer(r.Context(), cfg); err != nil {
		s.respondError(w, http.StatusBadRequest, "MCP_CONNECT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "connected",
		"server_id": cfg.ID,
	})
}

func (s *Server) handleDisconnectMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcpHost == nil {
		s.respondError(w, http.StatusNotImplemented, "MCP_NOT_ENABLED", "mcp host is not configured")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if err := s.mcpHost.DisconnectServer(serverID); err != nil {
		s.respondError(w, http.StatusBadRequest, "MCP_DISCONNECT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "disconnected",
		"server_id": serverID,
	})
}

func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string          `json:"name"`
		AgentID string          `json:"agent_id"`
		Input   json.RawMessage `json:"input"`
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
		req.AgentID = "api_user"
	}

	res, err := s.toolReg.Execute(r.Context(), req.AgentID, req.Name, req.Input)
	if err != nil {
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

	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "skill name is required")
		return
	}

	skillDir := filepath.Join("./data/skills", req.Name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		s.respondError(w, http.StatusInternalServerError, "MKDIR_FAILED", err.Error())
		return
	}

	skillMD := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", req.Name, req.Description, req.Content)
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "created",
		"name":   req.Name,
		"path":   skillDir,
	})
}

func (s *Server) handleUploadWASM(w http.ResponseWriter, r *http.Request) {
	wasmDir := "./data/tools/wasm"
	_ = os.MkdirAll(wasmDir, 0755)

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

	destPath := filepath.Join(wasmDir, header.Filename)
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
		return
	}
	defer out.Close()

	var buf [4096]byte
	for {
		n, err := file.Read(buf[:])
		if n > 0 {
			_, _ = out.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":   "uploaded",
		"filename": header.Filename,
	})
}

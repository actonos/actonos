package server

import (
	"encoding/json"
	"net/http"

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

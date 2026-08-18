package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/actonos/actonos/internal/tools"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if s.approvalMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "APPROVALS_NOT_ENABLED", "approval manager is not configured")
		return
	}
	items, err := s.approvalMgr.List(r.Context(), r.URL.Query().Get("status"), 100)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_LIST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"approvals": items})
}

func (s *Server) handleApproveAction(w http.ResponseWriter, r *http.Request) {
	item, ok := s.decideApproval(w, r, "approved")
	if !ok {
		return
	}
	if s.toolReg == nil {
		s.respondError(w, http.StatusNotImplemented, "TOOLS_NOT_ENABLED", "tool registry is not configured")
		return
	}
	if item.ToolName == "system_mcp_connect" {
		if s.mcpHost == nil {
			s.respondError(w, http.StatusNotImplemented, "MCP_NOT_ENABLED", "mcp host is not configured")
			return
		}
		var cfg tools.MCPServerConfig
		if err := json.Unmarshal(item.Input, &cfg); err != nil {
			s.respondError(w, http.StatusBadRequest, "INVALID_MCP_APPROVAL", err.Error())
			return
		}
		if err := s.mcpHost.ConnectServer(r.Context(), cfg); err != nil {
			s.respondError(w, http.StatusBadRequest, "MCP_CONNECT_FAILED", err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{
			"approval": item,
			"result": map[string]string{
				"status":    "connected",
				"server_id": cfg.ID,
			},
		})
		return
	}
	if strings.HasPrefix(item.ToolName, "admin_") {
		result, execErr := s.executeAdminAction(r.Context(), strings.TrimPrefix(item.ToolName, "admin_"), item.Input)
		if execErr != nil {
			s.respondError(w, http.StatusBadRequest, "ADMIN_ACTION_FAILED", execErr.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{"approval": item, "result": result})
		return
	}
	if s.runStore != nil && s.engine != nil {
		if _, _, checkpointErr := s.runStore.LoadCheckpointByTrace(r.Context(), item.TraceID); checkpointErr == nil {
			response, resumeErr := s.engine.ResumeApproved(r.Context(), *item)
			if resumeErr != nil {
				s.respondError(w, http.StatusBadRequest, "APPROVED_RESUME_FAILED", resumeErr.Error())
				return
			}
			s.respondJSON(w, http.StatusOK, map[string]any{
				"approval": item,
				"response": response,
			})
			return
		}
	}
	ctx := tools.WithTraceID(r.Context(), item.TraceID)
	ctx = tools.WithApprovalID(ctx, item.ID)
	result, err := s.toolReg.Execute(ctx, item.AgentID, item.ToolName, item.Input)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "APPROVED_EXECUTION_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"approval": item,
		"result":   result,
	})
}

func (s *Server) handleRejectAction(w http.ResponseWriter, r *http.Request) {
	item, ok := s.decideApproval(w, r, "rejected")
	if ok {
		s.respondJSON(w, http.StatusOK, item)
	}
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request, decision string) (*tools.ApprovalRequest, bool) {
	if s.approvalMgr == nil {
		s.respondError(w, http.StatusNotImplemented, "APPROVALS_NOT_ENABLED", "approval manager is not configured")
		return nil, false
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := s.decodeJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return nil, false
		}
	}
	item, err := s.approvalMgr.Decide(r.Context(), chi.URLParam(r, "id"), decision, "system_admin", req.Reason)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, tools.ErrApprovalInvalid) {
			status = http.StatusConflict
		}
		s.respondError(w, status, "APPROVAL_DECISION_FAILED", err.Error())
		return nil, false
	}
	return item, true
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/actonos/actonos/internal/system"
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

// failApproval reports an execution failure and returns the approval to the
// pending state so the operator can retry or reject instead of being stranded
// with a consumed record.
func (s *Server) failApproval(w http.ResponseWriter, r *http.Request, id, code, message string) {
	if s.approvalMgr != nil {
		if reopenErr := s.approvalMgr.Reopen(r.Context(), id); reopenErr != nil {
			message = fmt.Sprintf("%s (could not reopen approval: %v)", message, reopenErr)
		}
	}
	s.respondError(w, http.StatusBadRequest, code, message)
}

func (s *Server) handleApproveAction(w http.ResponseWriter, r *http.Request) {
	item, ok := s.decideApproval(w, r, "approved")
	if !ok {
		return
	}
	if s.toolReg == nil {
		s.failApproval(w, r, item.ID, "TOOLS_NOT_ENABLED", "tool registry is not configured")
		return
	}
	if item.ToolName == "system_mcp_connect" {
		if s.mcpHost == nil {
			s.failApproval(w, r, item.ID, "MCP_NOT_ENABLED", "mcp host is not configured")
			return
		}
		var cfg tools.MCPServerConfig
		if err := json.Unmarshal(item.Input, &cfg); err != nil {
			s.failApproval(w, r, item.ID, "INVALID_MCP_APPROVAL", err.Error())
			return
		}
		if err := s.mcpHost.ConnectServer(r.Context(), cfg); err != nil {
			s.failApproval(w, r, item.ID, "MCP_CONNECT_FAILED", err.Error())
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
			s.failApproval(w, r, item.ID, "ADMIN_ACTION_FAILED", execErr.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{"approval": item, "result": result})
		return
	}
	if s.runStore != nil && s.engine != nil {
		if _, _, checkpointErr := s.runStore.LoadCheckpointByTrace(r.Context(), item.TraceID); checkpointErr == nil {
			// Resume the agent run in a background goroutine so the operator's HTTP request returns immediately (< 50ms)
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						slog.Error("panic in resumed run goroutine", "panic", rec, "approval_id", item.ID)
					}
				}()
				_, resumeErr := s.engine.ResumeApproved(context.Background(), *item)
				if resumeErr != nil {
					var appErr *tools.ApprovalRequiredError
					if errors.As(resumeErr, &appErr) {
						slog.Info("resumed run paused for subsequent approval", "approval_id", appErr.Approval.ID, "tool", appErr.Approval.ToolName)
					} else {
						slog.Error("failed resuming approved run", "approval_id", item.ID, "trace_id", item.TraceID, "error", resumeErr)
						if s.notifMgr != nil {
							_, _ = s.notifMgr.Create(context.Background(), system.Notification{
								Title:    fmt.Sprintf("Resume Failed: %s", item.ToolName),
								Message:  fmt.Sprintf("Agent '%s' failed to complete after approval: %v", item.AgentID, resumeErr),
								Type:     "error",
								Category: "mission",
								Link:     "/missions",
							})
						}
					}
				}
			}()
			s.respondJSON(w, http.StatusOK, map[string]any{
				"approval": item,
				"status":   "resumed",
				"message":  "Approval recorded and agent run resumed in background",
			})
			return
		}
	}
	ctx := tools.WithTraceID(r.Context(), item.TraceID)
	ctx = tools.WithApprovalID(ctx, item.ID)
	result, err := s.toolReg.Execute(ctx, item.AgentID, item.ToolName, item.Input)
	if err != nil {
		s.failApproval(w, r, item.ID, "APPROVED_EXECUTION_FAILED", err.Error())
		return
	}
	if s.heartbeat != nil {
		s.heartbeat.TriggerWakeup()
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"approval": item,
		"result":   result,
	})
}

func (s *Server) handleRejectAction(w http.ResponseWriter, r *http.Request) {
	item, ok := s.decideApproval(w, r, "rejected")
	if ok {
		if s.heartbeat != nil {
			s.heartbeat.TriggerWakeup()
		}
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
		if errors.Is(err, tools.ErrApprovalInvalid) || errors.Is(err, tools.ErrApprovalNotPending) {
			status = http.StatusConflict
		}
		s.respondError(w, status, "APPROVAL_DECISION_FAILED", err.Error())
		return nil, false
	}
	return item, true
}

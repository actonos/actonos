package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPMutationsRequireApprovalBeforeExecution(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		action string
	}{
		{name: "toggle", method: http.MethodPut, path: "/api/tools/mcp/example", body: `{"enabled":false}`, action: "admin_mcp_toggle"},
		{name: "disconnect", method: http.MethodDelete, path: "/api/tools/mcp/example", action: "admin_mcp_disconnect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.Router().ServeHTTP(rec, req)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
			}

			var response struct {
				Data struct {
					Status   string `json:"status"`
					Approval struct {
						ID       string          `json:"id"`
						ToolName string          `json:"tool_name"`
						Input    json.RawMessage `json:"input"`
					} `json:"approval"`
				} `json:"data"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if response.Data.Status != "approval_required" {
				t.Fatalf("status = %q, want approval_required", response.Data.Status)
			}
			if response.Data.Approval.ToolName != tt.action {
				t.Fatalf("tool = %q, want %q", response.Data.Approval.ToolName, tt.action)
			}

			items, err := srv.approvalMgr.List(context.Background(), "pending", 10)
			if err != nil {
				t.Fatalf("listing approvals: %v", err)
			}
			if len(items) != 1 || items[0].ID != response.Data.Approval.ID {
				t.Fatalf("pending approvals = %+v, want exactly created approval", items)
			}

			servers, err := srv.mcpHost.ListServers(context.Background())
			if err != nil {
				t.Fatalf("listing MCP servers: %v", err)
			}
			if len(servers) != 0 {
				t.Fatalf("MCP mutation executed before approval: %+v", servers)
			}
		})
	}
}

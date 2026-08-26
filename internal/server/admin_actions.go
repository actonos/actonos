package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

const adminAgentID = "agent_system_core"

func (s *Server) requestAdminAction(ctx context.Context, action string, input any) (any, error) {
	if s.approvalMgr == nil {
		return nil, errors.New("approval manager is not configured")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshalling administrative action: %w", err)
	}
	return s.approvalMgr.Request(ctx, "", adminAgentID, "admin_"+action, "High", raw)
}

func (s *Server) executeAdminAction(ctx context.Context, action string, raw json.RawMessage) (any, error) {
	switch action {
	case "workspace_write", "workspace_upload":
		return s.executeWorkspaceWrite(ctx, raw)
	case "workspace_delete":
		var input struct {
			FileID          string `json:"file_id"`
			ExpectedVersion int64  `json:"expected_version"`
			Recursive       bool   `json:"recursive"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if s.workspaceStore == nil {
			return nil, errors.New("workspace store is unavailable")
		}
		node, err := s.workspaceStore.Get(ctx, input.FileID)
		if err != nil {
			return nil, err
		}
		if err := s.workspaceStore.Delete(ctx, input.FileID, input.ExpectedVersion, input.Recursive); err != nil {
			return nil, err
		}
		if s.embedding != nil {
			_ = s.embedding.NotifyWorkspaceMutation(context.Background(), node.ID, adminAgentID, true)
		}
		return map[string]any{"status": "deleted", "id": node.ID, "file_id": node.ID, "name": node.Name}, nil
	case "workspace_mkdir":
		var input struct {
			ParentID string `json:"parent_id"`
			Name     string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if s.workspaceStore == nil {
			return nil, errors.New("workspace store is unavailable")
		}
		node, err := s.workspaceStore.CreateDirectory(ctx, input.ParentID, input.Name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": "created", "id": node.ID, "parent_id": node.ParentID, "name": node.Name, "virtual_path": node.VirtualPath}, nil
	case "workspace_rename":
		var input struct {
			FileID          string `json:"file_id"`
			ParentID        string `json:"parent_id"`
			Name            string `json:"name"`
			ExpectedVersion int64  `json:"expected_version"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if s.workspaceStore == nil {
			return nil, errors.New("workspace store is unavailable")
		}
		node, err := s.workspaceStore.Rename(ctx, input.FileID, input.ParentID, input.Name, input.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": "renamed", "id": node.ID, "name": node.Name, "virtual_path": node.VirtualPath, "version": node.Version}, nil
	case "workspace_duplicate":
		var input struct {
			FileID   string `json:"file_id"`
			ParentID string `json:"parent_id"`
			Name     string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if s.workspaceStore == nil {
			return nil, errors.New("workspace store is unavailable")
		}
		node, err := s.workspaceStore.Duplicate(ctx, input.FileID, input.ParentID, input.Name, adminAgentID)
		if err != nil {
			return nil, err
		}
		if s.embedding != nil {
			_ = s.embedding.NotifyWorkspaceMutation(context.Background(), node.ID, adminAgentID, false)
		}
		return map[string]any{"status": "duplicated", "id": node.ID, "name": node.Name, "virtual_path": node.VirtualPath, "version": node.Version}, nil
	case "skill_create":
		var input struct {
			Name, Description, Content string
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if strings.ContainsAny(input.Name, `/\`) || input.Name == "" {
			return nil, errors.New("invalid skill name")
		}
		dir := filepath.Join(s.skillsDir, input.Name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", input.Name, input.Description, input.Content)
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
			return nil, err
		}
		return map[string]string{"status": "created", "name": input.Name, "path": dir}, nil
	case "wasm_upload":
		var input struct {
			Filename string `json:"filename"`
			Data     string `json:"data"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		name := filepath.Base(input.Filename)
		if filepath.Ext(name) != ".wasm" {
			return nil, errors.New("uploaded plugin must use .wasm extension")
		}
		data, err := base64.StdEncoding.DecodeString(input.Data)
		if err != nil {
			return nil, err
		}
		dir := s.wasmDir
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			return nil, err
		}
		return map[string]string{"status": "uploaded", "filename": name}, nil
	case "hub_install", "hub_uninstall":
		var input struct {
			SkillID string `json:"skill_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if s.hubMgr == nil {
			return nil, errors.New("hub manager is not configured")
		}
		var err error
		if action == "hub_install" {
			err = s.hubMgr.InstallSkill(input.SkillID)
		} else {
			err = s.hubMgr.UninstallSkill(input.SkillID)
		}
		if err != nil {
			return nil, err
		}
		return map[string]string{"status": strings.TrimPrefix(action, "hub_") + "ed", "skill_id": input.SkillID}, nil
	case "mcp_toggle":
		var input struct {
			ServerID string `json:"server_id"`
			Enabled  bool   `json:"enabled"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, fmt.Errorf("decoding MCP toggle input: %w", err)
		}
		if s.mcpHost == nil {
			return nil, errors.New("mcp host is not configured")
		}
		if err := s.mcpHost.SetServerEnabled(ctx, input.ServerID, input.Enabled); err != nil {
			return nil, fmt.Errorf("updating MCP server: %w", err)
		}
		return map[string]any{"status": "updated", "server_id": input.ServerID, "enabled": input.Enabled}, nil
	case "mcp_disconnect":
		var input struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, fmt.Errorf("decoding MCP disconnect input: %w", err)
		}
		if s.mcpHost == nil {
			return nil, errors.New("mcp host is not configured")
		}
		if err := s.mcpHost.DisconnectServer(input.ServerID); err != nil {
			return nil, fmt.Errorf("disconnecting MCP server: %w", err)
		}
		return map[string]string{"status": "disconnected", "server_id": input.ServerID}, nil
	case "system_restart":
		if s.hal == nil {
			return nil, errors.New("HAL is not configured")
		}
		go func() {
			time.Sleep(time.Second)
			_ = s.hal.RestartDaemon(context.Background())
		}()
		return map[string]string{"status": "restarting"}, nil
	case "ota_apply":
		if s.ota == nil {
			return nil, errors.New("ota engine is not configured")
		}
		if err := s.ota.EnqueueApply(ctx, s.version, s.embeddingdRequired()); err != nil {
			return nil, err
		}
		return map[string]any{"status": "applying"}, nil
	case "ota_rollback":
		if s.ota == nil {
			return nil, errors.New("ota engine is not configured")
		}
		if err := s.ota.EnqueueRollback(); err != nil {
			return nil, err
		}
		return map[string]any{"status": "rolling_back"}, nil
	default:
		return nil, fmt.Errorf("unknown administrative action %q", action)
	}
}

func (s *Server) executeWorkspaceWrite(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.workspaceStore == nil {
		return nil, errors.New("workspace store is unavailable")
	}
	var input workspaceWriteAdminInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("decoding workspace write: %w", err)
	}
	var content []byte
	switch input.Encoding {
	case "base64":
		cleaned := strings.TrimSpace(input.ContentBase64)
		if idx := strings.Index(cleaned, ";base64,"); idx != -1 {
			cleaned = cleaned[idx+8:]
		}
		cleaned = strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, cleaned)
		var decoded []byte
		var err error
		if d, e := base64.StdEncoding.DecodeString(cleaned); e == nil {
			decoded = d
		} else if d, e := base64.RawStdEncoding.DecodeString(cleaned); e == nil {
			decoded = d
		} else if d, e := base64.URLEncoding.DecodeString(cleaned); e == nil {
			decoded = d
		} else if d, e := base64.RawURLEncoding.DecodeString(cleaned); e == nil {
			decoded = d
		} else {
			err = e
		}
		if err != nil && len(decoded) == 0 {
			return nil, fmt.Errorf("decoding workspace content: %w", err)
		}
		content = decoded
	case "utf8", "":
		content = []byte(input.Content)
	default:
		return nil, errors.New("unsupported workspace content encoding")
	}
	node, err := s.workspaceStore.Write(ctx, workspacepkg.WriteRequest{
		ID: input.FileID, ParentID: input.ParentID, Name: input.Name, Content: content,
		MIMEType: input.MIMEType, ExpectedVersion: input.ExpectedVersion, ActorID: adminAgentID,
	})
	if err != nil {
		return nil, err
	}
	if s.embedding != nil {
		_ = s.embedding.NotifyWorkspaceMutation(context.Background(), node.ID, adminAgentID, false)
	}
	return map[string]any{
		"status": "saved", "id": node.ID, "file_id": node.ID, "parent_id": node.ParentID,
		"name": node.Name, "virtual_path": node.VirtualPath, "version": node.Version,
		"written": node.SizeBytes,
	}, nil
}

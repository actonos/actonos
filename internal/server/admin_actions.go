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

	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/security"
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
	case "workspace_write":
		var input struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		target, err := security.ResolvePath(s.workspaceDir, input.Path, true)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(input.Content), 0644); err != nil {
			return nil, err
		}
		if s.embedding != nil {
			_ = s.embedding.EnqueueFile(context.Background(), target, "", "shared", memory.EmbeddingUpsert)
		}
		return map[string]any{"path": filepath.Clean(input.Path), "written": len(input.Content)}, nil
	case "workspace_upload":
		var input struct {
			Path string `json:"path"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		data, err := base64.StdEncoding.DecodeString(input.Data)
		if err != nil {
			return nil, err
		}
		target, err := security.ResolvePath(s.workspaceDir, input.Path, true)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return nil, err
		}
		if s.embedding != nil {
			_ = s.embedding.EnqueueFile(context.Background(), target, "", "shared", memory.EmbeddingUpsert)
		}
		return map[string]any{"path": input.Path, "written": len(data)}, nil
	case "workspace_delete":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if clean := filepath.Clean(input.Path); clean == "." || clean == "" {
			return nil, errors.New("workspace root cannot be deleted")
		}
		target, err := security.ResolvePath(s.workspaceDir, input.Path, false)
		if err != nil {
			return nil, err
		}
		if err := os.RemoveAll(target); err != nil {
			return nil, err
		}
		if s.embedding != nil {
			_ = s.embedding.EnqueueFile(context.Background(), target, "", "shared", memory.EmbeddingDelete)
		}
		return map[string]string{"status": "deleted"}, nil
	case "workspace_mkdir":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		target, err := security.ResolvePath(s.workspaceDir, input.Path, true)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(target, 0755); err != nil {
			return nil, err
		}
		return map[string]string{"status": "created", "path": filepath.Clean(input.Path)}, nil
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
	default:
		return nil, fmt.Errorf("unknown administrative action %q", action)
	}
}

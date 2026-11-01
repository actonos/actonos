package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/actonos/actonos/internal/workspace"
	"github.com/tetratelabs/wazero/api"
)

// WorkspaceSavePayload represents a request to save a file into the User Workspace.
type WorkspaceSavePayload struct {
	Path          string `json:"path"`
	Name          string `json:"name,omitempty"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
}

// WorkspaceFileResponse represents the result of saving or reading a workspace file.
type WorkspaceFileResponse struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	URL           string `json:"url"`
	SizeBytes     int64  `json:"size_bytes"`
	MIMEType      string `json:"mime_type"`
	ContentBase64 string `json:"content_base64,omitempty"`
	Error         string `json:"error,omitempty"`
}

func ensurePluginWorkspaceDirectory(ctx context.Context, store *workspace.Store, dirPath string) (string, error) {
	if store == nil || dirPath == "" || dirPath == "." || dirPath == "/" {
		return "", nil
	}
	clean := strings.Trim(strings.ReplaceAll(dirPath, `\`, "/"), "/")
	if clean == "" {
		return "", nil
	}
	existing, err := store.ResolveLegacyPath(ctx, clean)
	if err == nil && existing.Type == "directory" {
		return existing.ID, nil
	}

	segments := strings.Split(clean, "/")
	currentParent := ""
	currentPath := ""
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." {
			continue
		}
		if currentPath == "" {
			currentPath = seg
		} else {
			currentPath = currentPath + "/" + seg
		}
		node, rErr := store.ResolveLegacyPath(ctx, currentPath)
		if rErr == nil && node.Type == "directory" {
			currentParent = node.ID
			continue
		}
		created, cErr := store.CreateDirectory(ctx, currentParent, seg)
		if cErr == nil {
			currentParent = created.ID
		} else {
			if node, gErr := store.ResolveLegacyPath(ctx, currentPath); gErr == nil && node.Type == "directory" {
				currentParent = node.ID
			} else {
				return "", fmt.Errorf("creating directory %q: %w", seg, cErr)
			}
		}
	}
	return currentParent, nil
}

func workspaceSaveFile(ctx context.Context, m api.Module, reqPtr, reqLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.Workspace == nil {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: "workspace store not configured on host"})
		if h != nil {
			h.mu.Lock()
			h.LastResponse = errResp
			h.mu.Unlock()
		}
		return int32(len(errResp))
	}

	if err := h.Gate.CheckWorkspaceAccess(); err != nil {
		h.Record("WARN", fmt.Sprintf("workspace access denied: %v", err))
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: err.Error()})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		h.Record("ERROR", fmt.Sprintf("workspace_save: read memory failed: %v", err))
		return -1
	}

	var req WorkspaceSavePayload
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: fmt.Sprintf("invalid json payload: %v", err)})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	targetPath := strings.Trim(strings.ReplaceAll(req.Path, `\`, "/"), "/")
	if targetPath == "" {
		targetPath = req.Name
	}
	if targetPath == "" {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: "missing path or name in request"})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	var rawData []byte
	if req.ContentBase64 != "" {
		cleanB64 := strings.TrimSpace(req.ContentBase64)
		if idx := strings.Index(cleanB64, ";base64,"); idx != -1 {
			cleanB64 = cleanB64[idx+8:]
		}
		var decodeErr error
		rawData, decodeErr = base64.StdEncoding.DecodeString(cleanB64)
		if decodeErr != nil {
			rawData, decodeErr = base64.URLEncoding.DecodeString(cleanB64)
		}
		if decodeErr != nil {
			errResp, _ := json.Marshal(WorkspaceFileResponse{Error: fmt.Sprintf("invalid base64 content: %v", decodeErr)})
			h.mu.Lock()
			h.LastResponse = errResp
			h.mu.Unlock()
			return int32(len(errResp))
		}
	} else {
		rawData = []byte(req.Content)
	}

	segments := strings.Split(targetPath, "/")
	filename := segments[len(segments)-1]
	parentID := ""
	if len(segments) > 1 {
		dirPath := strings.Join(segments[:len(segments)-1], "/")
		pID, dirErr := ensurePluginWorkspaceDirectory(ctx, h.Workspace, dirPath)
		if dirErr != nil {
			h.Record("WARN", fmt.Sprintf("workspace create directory error: %v", dirErr))
		}
		parentID = pID
	}

	node, wErr := h.Workspace.Write(ctx, workspace.WriteRequest{
		ParentID: parentID,
		Name:     filename,
		Content:  rawData,
		MIMEType: req.MIMEType,
		ActorID:  "plugin:" + h.PluginID,
	})
	if wErr != nil {
		h.Record("ERROR", fmt.Sprintf("workspace write failed: %v", wErr))
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: fmt.Sprintf("workspace write error: %v", wErr)})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	rawURL := fmt.Sprintf("/api/workspace/raw?path=%s", url.QueryEscape(targetPath))
	resPayload := WorkspaceFileResponse{
		ID:        node.ID,
		Name:      node.Name,
		Path:      targetPath,
		URL:       rawURL,
		SizeBytes: node.SizeBytes,
		MIMEType:  node.MIMEType,
	}

	resBytes, _ := json.Marshal(resPayload)
	h.mu.Lock()
	h.LastResponse = resBytes
	h.mu.Unlock()

	h.Record("INFO", fmt.Sprintf("saved file to workspace: %s (%d bytes)", targetPath, len(rawData)))
	return int32(len(resBytes))
}

func workspaceReadFile(ctx context.Context, m api.Module, reqPtr, reqLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.Workspace == nil {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: "workspace store not configured on host"})
		if h != nil {
			h.mu.Lock()
			h.LastResponse = errResp
			h.mu.Unlock()
		}
		return int32(len(errResp))
	}

	if err := h.Gate.CheckWorkspaceAccess(); err != nil {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: err.Error()})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		return -1
	}

	var req struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	_ = json.Unmarshal(reqBytes, &req)

	var node workspace.Node
	var fileErr error
	if req.ID != "" {
		node, fileErr = h.Workspace.Get(ctx, req.ID)
	} else if req.Path != "" {
		clean := strings.Trim(strings.ReplaceAll(req.Path, `\`, "/"), "/")
		node, fileErr = h.Workspace.ResolveLegacyPath(ctx, clean)
	} else {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: "missing file id or path"})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	if fileErr != nil {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: fileErr.Error()})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}

	_, file, readErr := h.Workspace.Open(ctx, node.ID)
	if readErr != nil {
		errResp, _ := json.Marshal(WorkspaceFileResponse{Error: readErr.Error()})
		h.mu.Lock()
		h.LastResponse = errResp
		h.mu.Unlock()
		return int32(len(errResp))
	}
	defer file.Close()

	contentBytes, _ := io.ReadAll(file)
	resPayload := WorkspaceFileResponse{
		ID:            node.ID,
		Name:          node.Name,
		Path:          req.Path,
		URL:           fmt.Sprintf("/api/workspace/raw?path=%s", url.QueryEscape(node.Name)),
		SizeBytes:     node.SizeBytes,
		MIMEType:      node.MIMEType,
		ContentBase64: base64.StdEncoding.EncodeToString(contentBytes),
	}

	resBytes, _ := json.Marshal(resPayload)
	h.mu.Lock()
	h.LastResponse = resBytes
	h.mu.Unlock()
	return int32(len(resBytes))
}

func hostWorkspaceSave(ctx context.Context, m api.Module, reqPtr, reqLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}
	workspaceSaveFile(ctx, m, reqPtr, reqLen)
	h.mu.Lock()
	resBytes := h.LastResponse
	h.mu.Unlock()
	res, _ := writeBufferToGuest(ctx, h, resBytes)
	return res
}

func hostWorkspaceRead(ctx context.Context, m api.Module, reqPtr, reqLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}
	workspaceReadFile(ctx, m, reqPtr, reqLen)
	h.mu.Lock()
	resBytes := h.LastResponse
	h.mu.Unlock()
	res, _ := writeBufferToGuest(ctx, h, resBytes)
	return res
}

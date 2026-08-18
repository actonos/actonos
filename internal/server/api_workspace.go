package server

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/actonos/actonos/internal/security"
)

type WorkspaceFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	workspaceDir := s.workspaceDir
	_ = os.MkdirAll(workspaceDir, 0755)

	relDir := r.URL.Query().Get("dir")
	targetDir, err := security.ResolvePath(workspaceDir, relDir, true)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}
	cleanRel := filepath.Clean(relDir)
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "READ_DIR_FAILED", err.Error())
		return
	}

	var files []WorkspaceFile
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, WorkspaceFile{
			Name:    entry.Name(),
			Path:    filepath.ToSlash(filepath.Join(cleanRel, entry.Name())),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format("2006-01-02 15:04:05"),
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"files": files,
		"dir":   cleanRel,
		"count": len(files),
	})
}

func (s *Server) handleGetWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceDir := s.workspaceDir
	filePath := r.URL.Query().Get("path")
	targetFile, err := security.ResolvePath(workspaceDir, filePath, false)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}
	cleanRel := filepath.Clean(filePath)
	data, err := os.ReadFile(targetFile)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "FILE_NOT_FOUND", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"path":    cleanRel,
		"content": string(data),
		"size":    len(data),
	})
}

func (s *Server) handleSaveWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	approval, err := s.requestAdminAction(r.Context(), "workspace_write", req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleDeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	cleanRel := filepath.Clean(filePath)
	if cleanRel == "." || cleanRel == "" {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "workspace root cannot be deleted")
		return
	}
	approval, err := s.requestAdminAction(r.Context(), "workspace_delete", map[string]string{"path": filePath})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleMkdirWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	approval, err := s.requestAdminAction(r.Context(), "workspace_mkdir", req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleUploadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceDir := s.workspaceDir
	_ = os.MkdirAll(workspaceDir, 0755)

	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "PARSE_FORM_FAILED", err.Error())
		return
	}

	relDir := r.FormValue("dir")
	_, err = security.ResolvePath(workspaceDir, relDir, true)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}
	cleanRel := filepath.Clean(relDir)

	file, header, err := r.FormFile("file")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "FILE_MISSING", err.Error())
		return
	}
	defer file.Close()

	fileName := filepath.Base(header.Filename)
	if fileName == "." || fileName == string(filepath.Separator) || fileName == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_FILENAME", "invalid upload filename")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 8<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_FAILED", err.Error())
		return
	}
	approval, err := s.requestAdminAction(r.Context(), "workspace_upload", map[string]string{
		"path": filepath.ToSlash(filepath.Join(cleanRel, fileName)),
		"data": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

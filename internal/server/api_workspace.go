package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type WorkspaceFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	workspaceDir := "./data/workspace"
	_ = os.MkdirAll(workspaceDir, 0755)

	relDir := r.URL.Query().Get("dir")
	cleanRel := filepath.Clean(relDir)
	if strings.HasPrefix(cleanRel, "..") {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	targetDir := filepath.Join(workspaceDir, cleanRel)
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
	workspaceDir := "./data/workspace"
	filePath := r.URL.Query().Get("path")
	cleanRel := filepath.Clean(filePath)
	if strings.HasPrefix(cleanRel, "..") {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	targetFile := filepath.Join(workspaceDir, cleanRel)
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
	workspaceDir := "./data/workspace"
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	cleanRel := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanRel, "..") {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	targetFile := filepath.Join(workspaceDir, cleanRel)
	_ = os.MkdirAll(filepath.Dir(targetFile), 0755)

	if err := os.WriteFile(targetFile, []byte(req.Content), 0644); err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"path":    cleanRel,
		"written": len(req.Content),
	})
}

func (s *Server) handleDeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceDir := "./data/workspace"
	filePath := r.URL.Query().Get("path")
	cleanRel := filepath.Clean(filePath)
	if strings.HasPrefix(cleanRel, "..") {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	targetFile := filepath.Join(workspaceDir, cleanRel)
	if err := os.RemoveAll(targetFile); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

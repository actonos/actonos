package server

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	if cleanRel == "." || cleanRel == "/" || cleanRel == "\\" {
		cleanRel = ""
	}
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
		var relPath string
		if cleanRel == "" {
			relPath = entry.Name()
		} else {
			relPath = filepath.ToSlash(filepath.Join(cleanRel, entry.Name()))
		}
		files = append(files, WorkspaceFile{
			Name:    entry.Name(),
			Path:    relPath,
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

func fileKind(name string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp", ".ico", ".svg":
		return "image"
	case ".pdf":
		return "pdf"
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	case ".csv", ".tsv":
		return "csv"
	case ".txt", ".yaml", ".yml", ".xml", ".html", ".css", ".js", ".ts", ".jsx", ".tsx", ".py", ".go", ".sh", ".env", ".sql", ".rs", ".c", ".cpp", ".h", ".java", ".dockerfile", ".toml", ".ini", ".conf":
		return "text"
	}
	if len(data) == 0 {
		return "text"
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	detected := http.DetectContentType(sniff)
	if strings.HasPrefix(detected, "image/") {
		return "image"
	}
	if strings.Contains(detected, "pdf") {
		return "pdf"
	}
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return "binary"
		}
	}
	return "text"
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

	kind := fileKind(filepath.Base(filePath), data)
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		sniff := data
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		mimeType = http.DetectContentType(sniff)
	}

	resp := map[string]any{
		"path": cleanRel,
		"size": len(data),
		"kind": kind,
		"mime": mimeType,
	}
	if kind != "image" && kind != "pdf" && kind != "binary" {
		resp["content"] = string(data)
	} else if kind == "image" || kind == "pdf" {
		b64 := base64.StdEncoding.EncodeToString(data)
		resp["data_url"] = fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
	}
	s.respondJSON(w, http.StatusOK, resp)
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

	targetFile, err := security.ResolvePath(s.workspaceDir, req.Path, true)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
		return
	}

	if err := os.WriteFile(targetFile, []byte(req.Content), 0644); err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
		return
	}

	if s.auditLogger != nil {
		s.auditLogger.LogAudit("", "admin", "admin_workspace_write", "Low", "Success", "", 1)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "saved",
		"path":    filepath.ToSlash(req.Path),
		"written": len(req.Content),
	})
}

func (s *Server) handleDeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	cleanRel := filepath.Clean(filePath)
	if cleanRel == "." || cleanRel == "" || cleanRel == "/" || cleanRel == "\\" {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "workspace root cannot be deleted")
		return
	}

	targetFile, err := security.ResolvePath(s.workspaceDir, filePath, false)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	if err := os.RemoveAll(targetFile); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	if s.auditLogger != nil {
		s.auditLogger.LogAudit("", "admin", "admin_workspace_delete", "Low", "Success", "", 1)
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"path":   filepath.ToSlash(filePath),
	})
}

func (s *Server) handleMkdirWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	targetDir, err := security.ResolvePath(s.workspaceDir, req.Path, true)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		s.respondError(w, http.StatusInternalServerError, "MKDIR_FAILED", err.Error())
		return
	}

	if s.auditLogger != nil {
		s.auditLogger.LogAudit("", "admin", "admin_workspace_mkdir", "Low", "Success", "", 1)
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "created",
		"path":   filepath.ToSlash(req.Path),
	})
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
	targetDir, err := security.ResolvePath(workspaceDir, relDir, true)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "path escapes workspace")
		return
	}
	_ = os.MkdirAll(targetDir, 0755)

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

	targetFile, err := security.ResolvePath(targetDir, fileName, true)
	if err != nil {
		s.respondError(w, http.StatusForbidden, "ACCESS_DENIED", "file path escapes directory")
		return
	}

	out, err := os.Create(targetFile)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
		return
	}
	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
		return
	}

	uploadRel := filepath.ToSlash(filepath.Join(relDir, fileName))
	if s.auditLogger != nil {
		s.auditLogger.LogAudit("", "admin", "admin_workspace_upload", "Low", "Success", "", 1)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "uploaded",
		"path":    uploadRel,
		"written": written,
	})
}

func (s *Server) handleRawWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceDir := s.workspaceDir
	filePath := r.URL.Query().Get("path")
	targetFile, err := security.ResolvePath(workspaceDir, filePath, false)
	if err != nil {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(targetFile)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	inlineSafe := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".webp": "image/webp",
		".avif": "image/avif",
		".bmp":  "image/bmp",
		".ico":  "image/x-icon",
		".svg":  "image/svg+xml",
		".pdf":  "application/pdf",
		".txt":  "text/plain; charset=utf-8",
		".md":   "text/markdown; charset=utf-8",
		".json": "application/json",
		".html": "text/html; charset=utf-8",
		".csv":  "text/csv; charset=utf-8",
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	if ct, ok := inlineSafe[ext]; ok {
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "private, max-age=300")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filepath.Base(filePath)))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

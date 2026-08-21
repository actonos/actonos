package server

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

const maxWorkspaceUploadBytes = 64 << 20

type databaseWorkspaceItem struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	VirtualPath string `json:"virtual_path"`
	IsDir       bool   `json:"is_dir"`
	Size        int64  `json:"size"`
	ModTime     string `json:"mod_time"`
	Kind        string `json:"kind"`
	MIMEType    string `json:"mime_type"`
	Version     int64  `json:"version"`
	ContentHash string `json:"content_hash,omitempty"`
	AIIndexed   bool   `json:"ai_indexed"`
	AIState     string `json:"ai_state"`
}

func (s *Server) sqlDB() *sql.DB {
	if s.memory == nil || s.memory.DB() == nil {
		return nil
	}
	return s.memory.DB().SQLDB()
}

func (s *Server) requireWorkspaceStore(w http.ResponseWriter) (*workspacepkg.Store, bool) {
	if s.workspaceStore == nil {
		s.respondError(w, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "database-backed workspace is unavailable")
		return nil, false
	}
	return s.workspaceStore, true
}

func (s *Server) resolveWorkspaceNode(ctx context.Context, id, legacyPath string) (workspacepkg.Node, error) {
	if s.workspaceStore == nil {
		return workspacepkg.Node{}, errors.New("workspace store is unavailable")
	}
	if id != "" {
		return s.workspaceStore.Get(ctx, id)
	}
	if legacyPath != "" {
		return s.workspaceStore.ResolveLegacyPath(ctx, legacyPath)
	}
	return workspacepkg.Node{}, workspacepkg.ErrNotFound
}

func workspaceIDFromRequest(r *http.Request) string {
	for _, key := range []string{"id", "file_id"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) workspaceIndexStates(ctx context.Context) map[string]string {
	states := make(map[string]string)
	if db := s.sqlDB(); db != nil {
		rows, err := db.QueryContext(ctx, `SELECT source_key, state FROM semantic_sources WHERE source_type = 'workspace_file'`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, state string
				if rows.Scan(&id, &state) == nil {
					states[id] = state
				}
			}
		}
	}
	return states
}

func workspaceItem(node workspacepkg.Node, aiState string) databaseWorkspaceItem {
	if aiState == "" {
		aiState = "none"
	}
	return databaseWorkspaceItem{
		ID: node.ID, ParentID: node.ParentID, Name: node.Name,
		Path: node.VirtualPath, VirtualPath: node.VirtualPath,
		IsDir: node.Type == "directory", Size: node.SizeBytes,
		ModTime: node.UpdatedAt, Kind: workspacepkg.MediaKind(node.MIMEType),
		MIMEType: node.MIMEType, Version: node.Version, ContentHash: node.ContentHash,
		AIIndexed: aiState == "active", AIState: aiState,
	}
}

func (s *Server) handleDBListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	store, ok := s.requireWorkspaceStore(w)
	if !ok {
		return
	}
	parentID := strings.TrimSpace(r.URL.Query().Get("parent_id"))
	if parentID == "" {
		if legacyDir := strings.TrimSpace(r.URL.Query().Get("dir")); legacyDir != "" {
			parent, err := store.ResolveLegacyPath(r.Context(), legacyDir)
			if err != nil || parent.Type != "directory" {
				s.respondWorkspaceError(w, err)
				return
			}
			parentID = parent.ID
		}
	}
	nodes, err := store.List(r.Context(), parentID)
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	states := s.workspaceIndexStates(r.Context())
	items := make([]databaseWorkspaceItem, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, workspaceItem(node, states[node.ID]))
	}
	ancestors, err := store.Breadcrumbs(r.Context(), parentID)
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	breadcrumbs := make([]map[string]string, 0, len(ancestors))
	for _, ancestor := range ancestors {
		breadcrumbs = append(breadcrumbs, map[string]string{
			"id": ancestor.ID, "name": ancestor.Name, "virtual_path": ancestor.VirtualPath,
		})
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"files": items, "parent_id": parentID, "dir": parentID, "count": len(items),
		"virtual_root": workspacepkg.VirtualRoot, "breadcrumbs": breadcrumbs,
	})
}

func (s *Server) handleDBGetWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	store, ok := s.requireWorkspaceStore(w)
	if !ok {
		return
	}
	fileID := workspaceIDFromRequest(r)
	var node workspacepkg.Node
	var content []byte
	var err error
	if fileID == "" {
		if resolved, resolveErr := s.resolveWorkspaceNode(r.Context(), "", r.URL.Query().Get("path")); resolveErr == nil {
			node, content, err = store.Read(r.Context(), resolved.ID, 0, 0)
		} else {
			err = resolveErr
		}
	} else {
		node, content, err = store.Read(r.Context(), fileID, 0, 0)
	}
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	kind := workspacepkg.MediaKind(node.MIMEType)
	response := map[string]any{
		"id": node.ID, "file_id": node.ID, "parent_id": node.ParentID,
		"name": node.Name, "path": node.VirtualPath, "virtual_path": node.VirtualPath,
		"size": node.SizeBytes, "kind": kind, "mime": node.MIMEType,
		"mime_type": node.MIMEType, "version": node.Version, "content_hash": node.ContentHash,
	}
	if kind == "text" || kind == "json" || kind == "csv" {
		response["content"] = string(content)
	} else {
		encoded := base64.StdEncoding.EncodeToString(content)
		response["content_base64"] = encoded
		if kind == "image" || kind == "pdf" || kind == "audio" || kind == "video" {
			response["data_url"] = "data:" + node.MIMEType + ";base64," + encoded
		}
	}
	s.respondJSON(w, http.StatusOK, response)
}

type workspaceWriteAdminInput struct {
	FileID          string `json:"file_id,omitempty"`
	ParentID        string `json:"parent_id,omitempty"`
	Name            string `json:"name,omitempty"`
	Content         string `json:"content,omitempty"`
	ContentBase64   string `json:"content_base64,omitempty"`
	Encoding        string `json:"encoding"`
	MIMEType        string `json:"mime_type,omitempty"`
	ExpectedVersion int64  `json:"expected_version,omitempty"`
}

func (s *Server) shouldBypassWorkspaceApproval(ctx context.Context) bool {
	return tools.IsApprovalBypassed(ctx)
}

func (s *Server) requestWorkspaceApproval(w http.ResponseWriter, r *http.Request, action string, input any) {
	if s.shouldBypassWorkspaceApproval(r.Context()) {
		raw, err := json.Marshal(input)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
			return
		}
		result, execErr := s.executeAdminAction(r.Context(), action, raw)
		if execErr != nil {
			s.respondWorkspaceError(w, execErr)
			return
		}
		s.respondJSON(w, http.StatusOK, result)
		return
	}

	approval, err := s.requestAdminAction(r.Context(), action, input)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleDBSaveWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireWorkspaceStore(w); !ok {
		return
	}
	var request struct {
		ID              string  `json:"id"`
		FileID          string  `json:"file_id"`
		ParentID        string  `json:"parent_id"`
		Name            string  `json:"name"`
		Path            string  `json:"path"`
		Content         *string `json:"content"`
		ContentBase64   *string `json:"content_base64"`
		MIMEType        string  `json:"mime_type"`
		ExpectedVersion int64   `json:"expected_version"`
	}
	if err := s.decodeJSON(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	fileID := request.FileID
	if fileID == "" {
		fileID = request.ID
	}
	if fileID == "" && request.Path != "" {
		if existing, err := s.workspaceStore.ResolveLegacyPath(r.Context(), request.Path); err == nil {
			fileID = existing.ID
		} else if request.Name == "" {
			segments := strings.Split(strings.Trim(strings.ReplaceAll(request.Path, `\`, "/"), "/"), "/")
			request.Name = segments[len(segments)-1]
			if len(segments) > 1 {
				parent, parentErr := s.workspaceStore.ResolveLegacyPath(r.Context(), strings.Join(segments[:len(segments)-1], "/"))
				if parentErr != nil {
					s.respondWorkspaceError(w, parentErr)
					return
				}
				request.ParentID = parent.ID
			}
		}
	}
	if request.Content == nil && request.ContentBase64 == nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "content or content_base64 is required")
		return
	}
	if request.Content != nil && request.ContentBase64 != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "provide only one content encoding")
		return
	}
	adminInput := workspaceWriteAdminInput{
		FileID: fileID, ParentID: request.ParentID, Name: request.Name,
		MIMEType: request.MIMEType, ExpectedVersion: request.ExpectedVersion,
	}
	if request.ContentBase64 != nil {
		adminInput.ContentBase64 = *request.ContentBase64
		adminInput.Encoding = "base64"
	} else {
		adminInput.Content = *request.Content
		adminInput.Encoding = "utf8"
	}
	s.requestWorkspaceApproval(w, r, "workspace_write", adminInput)
}

func (s *Server) handleDBDeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	node, err := s.resolveWorkspaceNode(r.Context(), workspaceIDFromRequest(r), r.URL.Query().Get("path"))
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	expectedVersion, _ := strconv.ParseInt(r.URL.Query().Get("expected_version"), 10, 64)
	recursive, _ := strconv.ParseBool(r.URL.Query().Get("recursive"))
	s.requestWorkspaceApproval(w, r, "workspace_delete", map[string]any{
		"file_id": node.ID, "expected_version": expectedVersion, "recursive": recursive,
	})
}

func (s *Server) handleDBRenameWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID              string `json:"id"`
		FileID          string `json:"file_id"`
		ParentID        string `json:"parent_id"`
		Name            string `json:"name"`
		OldPath         string `json:"old_path"`
		NewPath         string `json:"new_path"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := s.decodeJSON(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	fileID := request.FileID
	if fileID == "" {
		fileID = request.ID
	}
	node, err := s.resolveWorkspaceNode(r.Context(), fileID, request.OldPath)
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	if request.Name == "" && request.NewPath != "" {
		segments := strings.Split(strings.Trim(strings.ReplaceAll(request.NewPath, `\`, "/"), "/"), "/")
		request.Name = segments[len(segments)-1]
		if len(segments) > 1 {
			parent, parentErr := s.workspaceStore.ResolveLegacyPath(r.Context(), strings.Join(segments[:len(segments)-1], "/"))
			if parentErr != nil {
				s.respondWorkspaceError(w, parentErr)
				return
			}
			request.ParentID = parent.ID
		}
	}
	if request.ParentID == "" {
		request.ParentID = node.ParentID
	}
	if request.ExpectedVersion == 0 {
		request.ExpectedVersion = node.Version
	}
	if err := workspacepkg.ValidateName(request.Name); err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	s.requestWorkspaceApproval(w, r, "workspace_rename", map[string]any{
		"file_id": node.ID, "parent_id": request.ParentID, "name": request.Name,
		"expected_version": request.ExpectedVersion,
	})
}

func (s *Server) handleDBDuplicateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID       string `json:"id"`
		FileID   string `json:"file_id"`
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
		Path     string `json:"path"`
	}
	if err := s.decodeJSON(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if request.FileID == "" {
		request.FileID = request.ID
	}
	node, err := s.resolveWorkspaceNode(r.Context(), request.FileID, request.Path)
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	if request.ParentID == "" {
		request.ParentID = node.ParentID
	}
	if request.Name == "" {
		request.Name = node.Name + " copy"
	}
	s.requestWorkspaceApproval(w, r, "workspace_duplicate", map[string]string{
		"file_id": node.ID, "parent_id": request.ParentID, "name": request.Name,
	})
}

func (s *Server) handleDBGetWorkspaceStats(w http.ResponseWriter, r *http.Request) {
	store, ok := s.requireWorkspaceStore(w)
	if !ok {
		return
	}
	stats, err := store.Stats(r.Context())
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	indexed := 0
	if db := s.sqlDB(); db != nil {
		_ = db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM semantic_sources WHERE source_type = 'workspace_file' AND state = 'active'`).Scan(&indexed)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"total_size": stats.TotalSize, "total_files": stats.TotalFiles,
		"total_directories": stats.TotalDirectories, "indexed_files": indexed,
		"breakdown": stats.Breakdown,
	})
}

func (s *Server) handleDBDownloadWorkspaceZip(w http.ResponseWriter, r *http.Request) {
	store, ok := s.requireWorkspaceStore(w)
	if !ok {
		return
	}
	ids := strings.FieldsFunc(r.URL.Query().Get("ids"), func(char rune) bool { return char == ',' })
	if len(ids) == 0 {
		if id := workspaceIDFromRequest(r); id != "" {
			ids = []string{id}
		} else if legacyPaths := r.URL.Query().Get("paths"); legacyPaths != "" {
			for _, path := range strings.Split(legacyPaths, ",") {
				node, err := store.ResolveLegacyPath(r.Context(), path)
				if err != nil {
					s.respondWorkspaceError(w, err)
					return
				}
				ids = append(ids, node.ID)
			}
		} else if path := r.URL.Query().Get("path"); path != "" {
			node, err := store.ResolveLegacyPath(r.Context(), path)
			if err != nil {
				s.respondWorkspaceError(w, err)
				return
			}
			ids = []string{node.ID}
		}
	}
	nodes, err := store.Walk(r.Context(), ids)
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="workspace-export.zip"`)
	writer := zip.NewWriter(w)
	defer writer.Close()
	for _, node := range nodes {
		entryName := strings.TrimPrefix(node.VirtualPath, workspacepkg.VirtualRoot+"/")
		if node.Type == "directory" {
			if !strings.HasSuffix(entryName, "/") {
				entryName += "/"
			}
			_, _ = writer.CreateHeader(&zip.FileHeader{Name: entryName, Method: zip.Store})
			continue
		}
		_, content, readErr := store.Read(r.Context(), node.ID, 0, 0)
		if readErr != nil {
			return
		}
		entry, createErr := writer.CreateHeader(&zip.FileHeader{Name: entryName, Method: zip.Deflate})
		if createErr != nil {
			return
		}
		_, _ = entry.Write(content)
	}
}

func (s *Server) handleDBReindexWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     string `json:"id"`
		FileID string `json:"file_id"`
		Path   string `json:"path"`
	}
	if err := s.decodeJSON(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if request.FileID == "" {
		request.FileID = request.ID
	}
	node, err := s.resolveWorkspaceNode(r.Context(), request.FileID, request.Path)
	if err != nil || node.Type != "file" {
		s.respondWorkspaceError(w, err)
		return
	}
	if s.embedding == nil {
		s.respondError(w, http.StatusServiceUnavailable, "EMBEDDING_UNAVAILABLE", "embedding service is unavailable")
		return
	}
	if err := s.embedding.EnqueueWorkspaceFile(r.Context(), node.ID, adminAgentID, memory.EmbeddingUpsert); err != nil {
		s.respondError(w, http.StatusInternalServerError, "REINDEX_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "id": node.ID, "file_id": node.ID})
}

func (s *Server) handleDBGetFileEmbeddingChunks(w http.ResponseWriter, r *http.Request) {
	node, err := s.resolveWorkspaceNode(r.Context(), workspaceIDFromRequest(r), r.URL.Query().Get("path"))
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	db := s.sqlDB()
	if db == nil {
		s.respondError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "database is unavailable")
		return
	}
	var sourceID, state, modelID, modelRevision, chunkerVersion string
	var generation int
	var indexedAt *string
	err = db.QueryRowContext(r.Context(), `SELECT id, state, model_id, model_revision, chunker_version,
		active_generation, indexed_at FROM semantic_sources WHERE source_type = 'workspace_file' AND source_key = ?`, node.ID).
		Scan(&sourceID, &state, &modelID, &modelRevision, &chunkerVersion, &generation, &indexedAt)
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		s.respondJSON(w, http.StatusOK, map[string]any{"state": "none", "chunk_count": 0, "chunks": []any{}})
		return
	}
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CHUNKS_FAILED", err.Error())
		return
	}
	rows, err := db.QueryContext(r.Context(), `SELECT id, ordinal, content, token_count, active, created_at
		FROM semantic_chunks WHERE source_id = ? AND generation = ? ORDER BY ordinal`, sourceID, generation)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CHUNKS_FAILED", err.Error())
		return
	}
	defer rows.Close()
	type chunk struct {
		ID         string `json:"id"`
		Ordinal    int    `json:"ordinal"`
		Content    string `json:"content"`
		TokenCount int    `json:"token_count"`
		Active     bool   `json:"active"`
		CreatedAt  string `json:"created_at"`
	}
	chunks := make([]chunk, 0)
	for rows.Next() {
		var item chunk
		if rows.Scan(&item.ID, &item.Ordinal, &item.Content, &item.TokenCount, &item.Active, &item.CreatedAt) == nil {
			chunks = append(chunks, item)
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"state": state, "model_id": modelID, "model_revision": modelRevision,
		"chunker_version": chunkerVersion, "active_generation": generation,
		"indexed_at": indexedAt, "chunk_count": len(chunks), "chunks": chunks,
	})
}

func (s *Server) handleDBMkdirWorkspace(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireWorkspaceStore(w); !ok {
		return
	}
	var request struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
		Path     string `json:"path"`
	}
	if err := s.decodeJSON(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if request.Name == "" && request.Path != "" {
		segments := strings.Split(strings.Trim(strings.ReplaceAll(request.Path, `\`, "/"), "/"), "/")
		request.Name = segments[len(segments)-1]
		if len(segments) > 1 {
			parent, err := s.workspaceStore.ResolveLegacyPath(r.Context(), strings.Join(segments[:len(segments)-1], "/"))
			if err != nil {
				s.respondWorkspaceError(w, err)
				return
			}
			request.ParentID = parent.ID
		}
	}
	if err := workspacepkg.ValidateName(request.Name); err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	s.requestWorkspaceApproval(w, r, "workspace_mkdir", map[string]string{"parent_id": request.ParentID, "name": request.Name})
}

func (s *Server) handleDBUploadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireWorkspaceStore(w); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkspaceUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxWorkspaceUploadBytes); err != nil {
		s.respondError(w, http.StatusBadRequest, "PARSE_FORM_FAILED", err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "FILE_MISSING", err.Error())
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxWorkspaceUploadBytes+1))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "READ_FAILED", err.Error())
		return
	}
	if len(content) > maxWorkspaceUploadBytes {
		s.respondError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "workspace upload exceeds 64 MiB")
		return
	}
	name := header.Filename
	if override := r.FormValue("name"); override != "" {
		name = override
	}
	if err := workspacepkg.ValidateName(name); err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	parentID := r.FormValue("parent_id")
	if parentID == "" && r.FormValue("dir") != "" {
		parent, err := s.workspaceStore.ResolveLegacyPath(r.Context(), r.FormValue("dir"))
		if err != nil {
			s.respondWorkspaceError(w, err)
			return
		}
		parentID = parent.ID
	}
	s.requestWorkspaceApproval(w, r, "workspace_upload", workspaceWriteAdminInput{
		ParentID: parentID, Name: name, ContentBase64: base64.StdEncoding.EncodeToString(content),
		Encoding: "base64", MIMEType: header.Header.Get("Content-Type"),
	})
}

func (s *Server) handleDBRawWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	store, ok := s.requireWorkspaceStore(w)
	if !ok {
		return
	}
	node, err := s.resolveWorkspaceNode(r.Context(), workspaceIDFromRequest(r), r.URL.Query().Get("path"))
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	node, content, err := store.Read(r.Context(), node.ID, 0, 0)
	if err != nil {
		s.respondWorkspaceError(w, err)
		return
	}
	w.Header().Set("Content-Type", node.MIMEType)
	disposition := mime.FormatMediaType("inline", map[string]string{"filename": node.Name})
	w.Header().Set("Content-Disposition", disposition)
	http.ServeContent(w, r, node.Name, parseWorkspaceTime(node.UpdatedAt), bytes.NewReader(content))
}

func parseWorkspaceTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func (s *Server) respondWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workspacepkg.ErrNotFound):
		s.respondError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "workspace item was not found")
	case errors.Is(err, workspacepkg.ErrConflict), errors.Is(err, workspacepkg.ErrVersion):
		s.respondError(w, http.StatusConflict, "WORKSPACE_CONFLICT", err.Error())
	case errors.Is(err, workspacepkg.ErrInvalidName), errors.Is(err, workspacepkg.ErrInvalidNode):
		s.respondError(w, http.StatusBadRequest, "INVALID_WORKSPACE_ITEM", err.Error())
	default:
		s.respondError(w, http.StatusInternalServerError, "WORKSPACE_FAILED", fmt.Sprint(err))
	}
}

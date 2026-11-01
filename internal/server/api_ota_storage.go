package server

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/actonos/actonos/internal/system"
)

func getDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info fs.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func (s *Server) handleGetStorageInfo(w http.ResponseWriter, r *http.Request) {
	dataDir := s.dataDir
	storageSize := getDirSize(filepath.Join(dataDir, "storage"))
	vectorsSize := getDirSize(filepath.Join(dataDir, "vectors"))
	workspaceSize := int64(0)
	if s.workspaceStore != nil {
		if stats, err := s.workspaceStore.Stats(r.Context()); err == nil {
			workspaceSize = stats.TotalSize
		}
	}
	agentWorkspaceSize := getDirSize(filepath.Join(dataDir, "agents"))
	logsSize := getDirSize(filepath.Join(dataDir, "logs"))

	totalSize := storageSize + vectorsSize + agentWorkspaceSize + logsSize

	s.respondJSON(w, http.StatusOK, map[string]any{
		"storage_bytes":         storageSize,
		"vectors_bytes":         vectorsSize,
		"workspace_bytes":       workspaceSize,
		"agent_workspace_bytes": agentWorkspaceSize,
		"logs_bytes":            logsSize,
		"total_bytes":           totalSize,
	})
}

func (s *Server) handleCheckOTA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	_ = s.decodeJSON(r, &req)
	if s.ota == nil {
		s.respondError(w, http.StatusServiceUnavailable, "OTA_UNAVAILABLE", "ota engine is not configured")
		return
	}
	result := s.ota.Check(r.Context(), s.version, req.Force, s.embeddingdRequired())
	s.respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleOTAStatus(w http.ResponseWriter, r *http.Request) {
	if s.ota == nil {
		s.respondError(w, http.StatusServiceUnavailable, "OTA_UNAVAILABLE", "ota engine is not configured")
		return
	}
	s.respondJSON(w, http.StatusOK, s.ota.Status())
}

func (s *Server) handleApplyOTA(w http.ResponseWriter, r *http.Request) {
	approval, err := s.requestAdminAction(r.Context(), "ota_apply", map[string]any{})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) handleRollbackOTA(w http.ResponseWriter, r *http.Request) {
	approval, err := s.requestAdminAction(r.Context(), "ota_rollback", map[string]any{})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPROVAL_REQUEST_FAILED", err.Error())
		return
	}
	s.respondJSON(w, http.StatusAccepted, map[string]any{"status": "approval_required", "approval": approval})
}

func (s *Server) embeddingdRequired() bool {
	in := system.EmbeddingRequiredInput{EnvForce: os.Getenv("ACTONOS_OTA_EMBEDDINGD")}
	if s.ota != nil {
		active, _ := s.ota.EmbeddingState()
		in.PriorEmbeddingActive = active
	}
	if s.embedding != nil {
		st, err := s.embedding.Status(context.Background())
		if err == nil {
			in.ServiceReady = st.ServiceReady
		}
	}
	return system.EmbeddingdRequired(in)
}

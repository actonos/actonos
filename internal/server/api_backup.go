package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/actonos/actonos/internal/system"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetBackup(w http.ResponseWriter, r *http.Request) {
	backupID := strings.TrimSpace(r.URL.Query().Get("id"))
	if backupID != "" {
		if s.backupMgr == nil {
			if s.memory != nil && s.memory.DB() != nil && s.memory.DB().SQLDB() != nil {
				s.backupMgr, _ = system.NewBackupManager(s.dataDir, s.memory.DB().SQLDB(), s.version)
			}
		}
		if s.backupMgr != nil {
			data, manifest, err := s.backupMgr.GetBackupArchive(backupID)
			if err == nil && manifest != nil {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", manifest.FileName))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}
	}

	if s.memory == nil || s.memory.DB() == nil || s.memory.DB().SQLDB() == nil {
		s.respondError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database is not ready")
		return
	}
	tempDir, err := os.MkdirTemp("", "actonos-backup-*")
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}
	defer os.RemoveAll(tempDir)
	tempBackupFile := filepath.Join(tempDir, "actonos-backup.db")

	// VACUUM INTO creates a transactionally consistent standalone database,
	// including committed WAL content, without blocking normal readers.
	if _, err := s.memory.DB().SQLDB().ExecContext(r.Context(), `VACUUM INTO ?`, tempBackupFile); err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}
	data, err := os.ReadFile(tempBackupFile)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", "attachment; filename=\"actonos-backup.db\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type createBackupRequest struct {
	IncludeWorkspace bool   `json:"include_workspace"`
	Notes            string `json:"notes"`
	Download         bool   `json:"download"`
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupMgr == nil {
		if s.memory != nil && s.memory.DB() != nil && s.memory.DB().SQLDB() != nil {
			s.backupMgr, _ = system.NewBackupManager(s.dataDir, s.memory.DB().SQLDB(), s.version)
		}
	}
	if s.backupMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "backup manager is not configured")
		return
	}

	var req createBackupRequest
	_ = s.decodeJSON(r, &req)

	manifest, _, err := s.backupMgr.CreateBackup(r.Context(), req.IncludeWorkspace, req.Notes)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"manifest": manifest,
		"message":  "Backup created successfully",
	})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if s.backupMgr == nil {
		if s.memory != nil && s.memory.DB() != nil && s.memory.DB().SQLDB() != nil {
			s.backupMgr, _ = system.NewBackupManager(s.dataDir, s.memory.DB().SQLDB(), s.version)
		}
	}
	if s.backupMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"backups": []any{}, "count": 0})
		return
	}

	backups, err := s.backupMgr.ListBackups()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "LIST_BACKUPS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"backups": backups,
		"count":   len(backups),
	})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupMgr == nil {
		if s.memory != nil && s.memory.DB() != nil && s.memory.DB().SQLDB() != nil {
			s.backupMgr, _ = system.NewBackupManager(s.dataDir, s.memory.DB().SQLDB(), s.version)
		}
	}
	if s.backupMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "backup manager is not configured")
		return
	}

	backupID := strings.TrimSpace(r.URL.Query().Get("id"))
	if backupID == "" {
		backupID = strings.TrimSpace(r.URL.Query().Get("backup_id"))
	}

	var archiveReader io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err == nil {
			file, _, fileErr := r.FormFile("backup_file")
			if fileErr != nil {
				file, _, fileErr = r.FormFile("backup")
			}
			if fileErr == nil {
				defer file.Close()
				archiveReader = file
			}
		}
	} else if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			BackupID string `json:"backup_id"`
		}
		if err := s.decodeJSON(r, &body); err == nil && body.BackupID != "" {
			backupID = body.BackupID
		}
	}

	var manifest *system.BackupManifest
	var err error

	if backupID != "" {
		manifest, err = s.backupMgr.RestoreBackupByID(r.Context(), backupID)
	} else {
		manifest, err = s.backupMgr.RestoreBackup(r.Context(), archiveReader)
	}

	if err != nil {
		s.respondError(w, http.StatusBadRequest, "RESTORE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"manifest": manifest,
		"message":  "Database restored successfully",
	})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupMgr == nil {
		if s.memory != nil && s.memory.DB() != nil && s.memory.DB().SQLDB() != nil {
			s.backupMgr, _ = system.NewBackupManager(s.dataDir, s.memory.DB().SQLDB(), s.version)
		}
	}
	if s.backupMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "backup manager is not configured")
		return
	}

	backupID := strings.TrimSpace(chi.URLParam(r, "id"))
	if backupID == "" {
		backupID = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if backupID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "backup id is required")
		return
	}

	if err := s.backupMgr.DeleteBackup(backupID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DELETE_BACKUP_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "deleted",
		"message": "Backup deleted successfully",
	})
}

type factoryResetRequest struct {
	ConfirmToken string `json:"confirm_token"`
}

func (s *Server) handleFactoryReset(w http.ResponseWriter, r *http.Request) {
	if s.backupMgr == nil {
		if s.memory != nil && s.memory.DB() != nil && s.memory.DB().SQLDB() != nil {
			s.backupMgr, _ = system.NewBackupManager(s.dataDir, s.memory.DB().SQLDB(), s.version)
		}
	}
	if s.backupMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "backup manager is not configured")
		return
	}

	var req factoryResetRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.backupMgr.FactoryReset(r.Context(), req.ConfirmToken); err != nil {
		s.respondError(w, http.StatusForbidden, "FACTORY_RESET_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "reset_complete",
		"message": "ActonOS database state has been factory reset.",
	})
}

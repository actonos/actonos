package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/memory"
	"github.com/go-chi/chi/v5"
)

type setSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type secretResponseItem struct {
	Name       string    `json:"name"`
	UpdatedAt  time.Time `json:"updated_at"`
	IsProvider bool      `json:"is_provider"`
}

func (s *Server) handleListVaultSecrets(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is not configured")
		return
	}

	rawList, err := s.vault.ListSecrets(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "VAULT_READ_FAILED", err.Error())
		return
	}

	items := make([]secretResponseItem, 0, len(rawList))
	for _, meta := range rawList {
		items = append(items, secretResponseItem{
			Name:       meta.Name,
			UpdatedAt:  meta.UpdatedAt,
			IsProvider: strings.HasPrefix(meta.Name, "provider_key_"),
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"secrets": items,
		"count":   len(items),
	})
}

func (s *Server) handleGetVaultSecret(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is not configured")
		return
	}

	secretName := chi.URLParam(r, "name")
	if secretName == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_SECRET_NAME", "secret name is required")
		return
	}

	val, err := s.vault.GetSecret(r.Context(), secretName)
	if err != nil {
		if errors.Is(err, memory.ErrSecretNotFound) {
			s.respondError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "secret not found in vault")
			return
		}
		s.respondError(w, http.StatusInternalServerError, "VAULT_READ_FAILED", err.Error())
		return
	}

	// Mask secret value for safe response
	masked := "••••••••"
	if len(val) > 8 {
		masked = val[:3] + "••••••••" + val[len(val)-3:]
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"name":        secretName,
		"configured":  true,
		"masked":      masked,
		"length":      len(val),
		"is_provider": strings.HasPrefix(secretName, "provider_key_"),
	})
}

func (s *Server) handleSetVaultSecret(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is not configured")
		return
	}

	var req setSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "failed to parse JSON body")
		return
	}

	urlName := chi.URLParam(r, "name")
	if urlName != "" {
		req.Name = urlName
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_SECRET_NAME", "secret name is required")
		return
	}

	if req.Value == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_SECRET_VALUE", "secret value cannot be empty")
		return
	}

	if err := s.vault.SetSecret(r.Context(), req.Name, req.Value); err != nil {
		s.respondError(w, http.StatusInternalServerError, "VAULT_WRITE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":      "stored",
		"name":        req.Name,
		"updated_at":  time.Now().UTC(),
		"is_provider": strings.HasPrefix(req.Name, "provider_key_"),
	})
}

func (s *Server) handleDeleteVaultSecret(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		s.respondError(w, http.StatusServiceUnavailable, "VAULT_UNAVAILABLE", "encrypted vault is not configured")
		return
	}

	secretName := chi.URLParam(r, "name")
	if secretName == "" {
		s.respondError(w, http.StatusBadRequest, "MISSING_SECRET_NAME", "secret name is required")
		return
	}

	if err := s.vault.DeleteSecret(r.Context(), secretName); err != nil {
		s.respondError(w, http.StatusInternalServerError, "VAULT_DELETE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "deleted",
		"name":   secretName,
	})
}

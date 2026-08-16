package server

import (
	"net/http"
)

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	metrics, err := s.hal.GetMetrics(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "METRICS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleGetTailscale(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"enabled":   false,
		})
		return
	}

	status, err := s.tailscale.GetStatus(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "TAILSCALE_STATUS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleWifiScan(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	networks, err := s.hal.ScanWifi(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "WIFI_SCAN_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"networks": networks,
		"count":    len(networks),
	})
}

func (s *Server) handleWifiConnect(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.SSID == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "ssid is required")
		return
	}

	if err := s.hal.ConnectWifi(r.Context(), req.SSID, req.Password); err != nil {
		s.respondError(w, http.StatusBadRequest, "WIFI_CONNECT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "connected",
		"ssid":   req.SSID,
	})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if s.hal == nil {
		s.respondError(w, http.StatusNotImplemented, "HAL_NOT_CONFIGURED", "hal is not configured")
		return
	}

	go func() {
		_ = s.hal.RestartDaemon(r.Context())
	}()

	s.respondJSON(w, http.StatusOK, map[string]any{"status": "restarting"})
}

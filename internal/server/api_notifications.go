package server

import (
	"net/http"
	"strconv"
)

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"notifications": []any{},
			"total":         0,
			"page":          1,
			"limit":         20,
			"unread_count":  0,
		})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	filterType := r.URL.Query().Get("type")
	unreadOnly := r.URL.Query().Get("unread_only") == "true" || r.URL.Query().Get("unread") == "1"

	items, total, unreadCount, err := s.notifMgr.List(r.Context(), page, limit, filterType, unreadOnly)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "LIST_NOTIFICATIONS_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"notifications": items,
		"total":         total,
		"page":          page,
		"limit":         limit,
		"unread_count":  unreadCount,
	})
}

func (s *Server) handleGetUnreadNotificationsCount(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"unread_count": 0})
		return
	}

	count, err := s.notifMgr.GetUnreadCount(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "GET_UNREAD_COUNT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"unread_count": count})
}

func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"status": "ok", "unread_count": 0})
		return
	}

	var req struct {
		ID  string `json:"id"`
		All bool   `json:"all"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.All || req.ID == "all" || req.ID == "" {
		if err := s.notifMgr.MarkAllAsRead(r.Context()); err != nil {
			s.respondError(w, http.StatusInternalServerError, "MARK_READ_FAILED", err.Error())
			return
		}
	} else {
		if err := s.notifMgr.MarkAsRead(r.Context(), req.ID); err != nil {
			s.respondError(w, http.StatusInternalServerError, "MARK_READ_FAILED", err.Error())
			return
		}
	}

	count, _ := s.notifMgr.GetUnreadCount(r.Context())
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"unread_count": count,
	})
}

func (s *Server) handleDeleteNotifications(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}

	id := r.URL.Query().Get("id")
	clearAll := r.URL.Query().Get("all") == "true" || r.URL.Query().Get("all") == "1"

	if clearAll || id == "all" {
		if err := s.notifMgr.ClearAll(r.Context()); err != nil {
			s.respondError(w, http.StatusInternalServerError, "CLEAR_NOTIFICATIONS_FAILED", err.Error())
			return
		}
	} else if id != "" {
		if err := s.notifMgr.Delete(r.Context(), id); err != nil {
			s.respondError(w, http.StatusInternalServerError, "DELETE_NOTIFICATION_FAILED", err.Error())
			return
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

package server

import (
	"net/http"
	"strconv"

	"github.com/actonos/actonos/internal/system"
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

func (s *Server) handleGetVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"public_key": ""})
		return
	}
	pubKey := s.notifMgr.GetVAPIDPublicKey()
	s.respondJSON(w, http.StatusOK, map[string]any{"public_key": pubKey})
}

func (s *Server) handleSubscribePush(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "NOTIF_MANAGER_NOT_READY", "notification manager unavailable")
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		UserAgent string `json:"user_agent"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		s.respondError(w, http.StatusBadRequest, "INVALID_SUBSCRIPTION", "endpoint and keys are required")
		return
	}

	sub := system.PushSubscription{
		Endpoint:  req.Endpoint,
		P256dh:    req.Keys.P256dh,
		Auth:      req.Keys.Auth,
		UserAgent: req.UserAgent,
	}
	if sub.UserAgent == "" {
		sub.UserAgent = r.UserAgent()
	}

	if err := s.notifMgr.SubscribePush(r.Context(), sub); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SUBSCRIBE_PUSH_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "push subscription registered successfully",
	})
}

func (s *Server) handleUnsubscribePush(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Endpoint != "" {
		if err := s.notifMgr.UnsubscribePush(r.Context(), req.Endpoint); err != nil {
			s.respondError(w, http.StatusInternalServerError, "UNSUBSCRIBE_PUSH_FAILED", err.Error())
			return
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleTestPushNotification(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "NOTIF_MANAGER_NOT_READY", "notification manager unavailable")
		return
	}

	var req struct {
		Title   string `json:"title"`
		Message string `json:"message"`
		Link    string `json:"link"`
	}
	_ = s.decodeJSON(r, &req)

	title := req.Title
	if title == "" {
		title = "ActonOS Background Alert"
	}
	message := req.Message
	if message == "" {
		message = "Service Worker background push is working properly!"
	}
	link := req.Link
	if link == "" {
		link = "/notifications"
	}

	notif, err := s.notifMgr.Create(r.Context(), system.Notification{
		Title:    title,
		Message:  message,
		Type:     "info",
		Category: "system",
		Link:     link,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "CREATE_TEST_NOTIF_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"notification": notif,
	})
}

func (s *Server) handleGetNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondJSON(w, http.StatusOK, system.NotificationPreferences{
			QuietHoursEnabled:  false,
			QuietHoursStart:    "22:00",
			QuietHoursEnd:      "07:00",
			QuietHoursTimezone: "UTC",
			DailyDigestEnabled: false,
			DailyDigestTime:    "08:00",
			MinPushSeverity:    "info",
		})
		return
	}

	prefs, err := s.notifMgr.GetPreferences(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "GET_PREFERENCES_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, prefs)
}

func (s *Server) handleSaveNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "NOTIF_MANAGER_NOT_READY", "notification manager unavailable")
		return
	}

	var prefs system.NotificationPreferences
	if err := s.decodeJSON(r, &prefs); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := s.notifMgr.SavePreferences(r.Context(), prefs); err != nil {
		s.respondError(w, http.StatusInternalServerError, "SAVE_PREFERENCES_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"preferences": prefs,
	})
}

func (s *Server) handleTriggerDailyDigest(w http.ResponseWriter, r *http.Request) {
	if s.notifMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "NOTIF_MANAGER_NOT_READY", "notification manager unavailable")
		return
	}

	digestNotif, err := s.notifMgr.GenerateDailyDigest(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "DIGEST_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"notification": digestNotif,
	})
}

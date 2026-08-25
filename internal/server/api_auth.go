package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/auth"
)

type AuthStatusResponse struct {
	Initialized   bool   `json:"initialized"`
	Authenticated bool   `json:"authenticated"`
	UserName      string `json:"user_name,omitempty"`
}

type SetupAuthRequest struct {
	Password           string `json:"password"`
	UserName           string `json:"user_name,omitempty"`
	UserRole           string `json:"user_role,omitempty"`
	Language           string `json:"language,omitempty"`
	Timezone           string `json:"timezone,omitempty"`
	CommunicationStyle string `json:"communication_style,omitempty"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
}

type LoginRequest struct {
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}
	if cookie, err := r.Cookie("actonos_token"); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "actonos_token",
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) handleGetAuthStatus(w http.ResponseWriter, r *http.Request) {
	if s.sysAuth == nil {
		s.respondJSON(w, http.StatusOK, AuthStatusResponse{
			Initialized:   true,
			Authenticated: true,
			UserName:      "Operator",
		})
		return
	}

	isInit, err := s.sysAuth.IsInitialized(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "AUTH_CHECK_FAILED", err.Error())
		return
	}

	token := s.extractToken(r)
	isAuth := isInit && s.sysAuth.ValidateToken(token)

	userName := "Operator"
	if s.profileMgr != nil {
		userName = s.profileMgr.GetProfile().UserName
	}

	s.respondJSON(w, http.StatusOK, AuthStatusResponse{
		Initialized:   isInit,
		Authenticated: isAuth,
		UserName:      userName,
	})
}

func (s *Server) handleSetupAuth(w http.ResponseWriter, r *http.Request) {
	if s.sysAuth == nil {
		s.respondError(w, http.StatusInternalServerError, "AUTH_UNAVAILABLE", "system auth manager is not configured")
		return
	}

	var req SetupAuthRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	token, err := s.sysAuth.SetupAdmin(r.Context(), req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrAlreadyInitialized) {
			s.respondError(w, http.StatusConflict, "ALREADY_INITIALIZED", "system is already initialized")
			return
		}
		if errors.Is(err, auth.ErrPasswordTooShort) {
			s.respondError(w, http.StatusBadRequest, "INVALID_PASSWORD", "password must be at least 8 characters")
			return
		}
		s.respondError(w, http.StatusInternalServerError, "SETUP_FAILED", err.Error())
		return
	}

	// Update user profile if specified
	if s.profileMgr != nil {
		profile := s.profileMgr.GetProfile()
		if req.UserName != "" {
			profile.UserName = req.UserName
		}
		if req.UserRole != "" {
			profile.UserRole = req.UserRole
		}
		if req.Language != "" {
			profile.Language = req.Language
		}
		if req.Timezone != "" {
			profile.Timezone = req.Timezone
		}
		if req.CommunicationStyle != "" {
			profile.CommunicationStyle = req.CommunicationStyle
		}
		if req.CustomInstructions != "" {
			profile.CustomInstructions = req.CustomInstructions
		}
		profile.UpdatedAt = time.Now().UTC()
		_ = s.profileMgr.UpdateProfile(r.Context(), profile)
	}

	setSessionCookie(w, r, token, int((24 * time.Hour).Seconds()))
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "initialized",
		"token":   token,
		"message": "ActonOS appliance initialized successfully",
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.sysAuth == nil {
		s.respondError(w, http.StatusInternalServerError, "AUTH_UNAVAILABLE", "system auth manager is not configured")
		return
	}

	var req LoginRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	token, err := s.sysAuth.Login(r.Context(), req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "incorrect administrator password")
			return
		}
		if errors.Is(err, auth.ErrTooManyAttempts) {
			s.respondError(w, http.StatusTooManyRequests, "LOGIN_LOCKED", "too many failed login attempts, try again later")
			return
		}
		if errors.Is(err, auth.ErrNotInitialized) {
			s.respondError(w, http.StatusForbidden, "SETUP_REQUIRED", "system onboarding has not been completed")
			return
		}
		s.respondError(w, http.StatusInternalServerError, "LOGIN_FAILED", err.Error())
		return
	}

	setSessionCookie(w, r, token, int((24 * time.Hour).Seconds()))
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":  "authenticated",
		"token":   token,
		"message": "Logged in successfully",
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.sysAuth != nil {
		token := s.extractToken(r)
		s.sysAuth.Logout(token)
	}
	setSessionCookie(w, r, "", -1)

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "logged_out",
	})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if s.sysAuth == nil {
		s.respondError(w, http.StatusInternalServerError, "AUTH_UNAVAILABLE", "system auth manager is not configured")
		return
	}

	var req ChangePasswordRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	err := s.sysAuth.ChangePassword(r.Context(), req.CurrentPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.respondError(w, http.StatusUnauthorized, "INVALID_CURRENT_PASSWORD", "incorrect current password")
			return
		}
		if errors.Is(err, auth.ErrPasswordTooShort) {
			s.respondError(w, http.StatusBadRequest, "INVALID_NEW_PASSWORD", "new password must be at least 8 characters")
			return
		}
		s.respondError(w, http.StatusInternalServerError, "CHANGE_PASSWORD_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "password_updated",
		"message": "Administrator password changed successfully",
	})
}

// RequireAuthMiddleware guards protected API routes.
func (s *Server) RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.sysAuth == nil {
			if s.allowMissingAuth {
				next.ServeHTTP(w, r)
				return
			}
			s.respondError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "system authentication is not configured")
			return
		}

		isInit, err := s.sysAuth.IsInitialized(r.Context())
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "AUTH_CHECK_FAILED", err.Error())
			return
		}

		// If not yet initialized, block protected endpoints with SETUP_REQUIRED
		if !isInit {
			s.respondError(w, http.StatusForbidden, "SETUP_REQUIRED", "ActonOS initial setup is required")
			return
		}

		token := s.extractToken(r)
		if !s.sysAuth.ValidateToken(token) {
			s.respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required. Please unlock ActonOS.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

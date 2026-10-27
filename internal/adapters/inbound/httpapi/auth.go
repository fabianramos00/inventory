package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
)

type AuthHandler struct {
	service *application.AuthService
	logger  *slog.Logger
	limiter *loginLimiter
}

type loginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	User         *domain.User `json:"user"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func NewAuthHandler(authService *application.AuthService, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		service: authService,
		logger:  logger,
		limiter: newLoginLimiter(5, time.Minute),
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !h.limiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many login attempts", "RATE_LIMITED")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.writeRequestError(w, err)
		return
	}
	email := r.PostFormValue("username")
	password := r.PostFormValue("password")
	if email == "" || password == "" {
		writeError(w, http.StatusUnprocessableEntity, "username and password are required", "VALIDATION_ERROR")
		return
	}

	result, err := h.service.Login(r.Context(), email, password)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "bearer",
		User:         result.User,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	var body refreshRequest
	if err := decodeJSON(w, r, &body); err != nil {
		h.writeRequestError(w, err)
		return
	}

	pair, err := h.service.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, refreshResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    "bearer",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	var body refreshRequest
	if err := decodeJSON(w, r, &body); err != nil {
		h.writeRequestError(w, err)
		return
	}

	if err := h.service.Logout(r.Context(), body.RefreshToken); err != nil {
		h.logger.Error("logout failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"detail": "Logged out"})
}

func (h *AuthHandler) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "Incorrect email or password", "INVALID_CREDENTIALS")
	case errors.Is(err, application.ErrInactiveUser):
		writeError(w, http.StatusUnauthorized, "Inactive user", "INACTIVE_USER")
	case errors.Is(err, application.ErrInvalidRefresh):
		writeError(w, http.StatusUnauthorized, "Invalid or expired refresh token", "INVALID_REFRESH_TOKEN")
	default:
		h.logger.Error("auth request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func (h *AuthHandler) writeRequestError(w http.ResponseWriter, err error) {
	if isBodyTooLarge(err) {
		writeError(w, http.StatusRequestEntityTooLarge, "Request body too large", "BODY_TOO_LARGE")
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "Invalid request body", "VALIDATION_ERROR")
}

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginWindow
	limit   int
	window  time.Duration
}

type loginWindow struct {
	start time.Time
	count int
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		entries: make(map[string]*loginWindow),
		limit:   limit,
		window:  window,
	}
}

// ponytail: fixed window in memory per remote IP; replace with a shared limiter if the API ever leaves loopback.
func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	window := l.entries[key]
	if window == nil || now.Sub(window.start) >= l.window {
		l.entries[key] = &loginWindow{start: now, count: 1}
		return true
	}
	if window.count >= l.limit {
		return false
	}
	window.count++
	return true
}

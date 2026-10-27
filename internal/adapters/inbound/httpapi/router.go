package httpapi

import (
	"log/slog"
	"net/http"

	"inventory-desktop/internal/ports/inbound"
)

func NewRouter(authService inbound.AuthUseCase, userService inbound.UserUseCase, logger *slog.Logger, devMode ...bool) http.Handler {
	return NewRouterWithBackend(authService, userService, logger, "", devMode...)
}

func NewRouterWithBackend(authService inbound.AuthUseCase, userService inbound.UserUseCase, logger *slog.Logger, backend string, devMode ...bool) http.Handler {
	dev := len(devMode) > 0 && devMode[0]
	if backend == "" {
		backend = "unknown"
	}
	mux := http.NewServeMux()
	authHandler := NewAuthHandler(authService, logger)
	usersHandler := NewUsersHandler(userService, logger)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": backend})
	})
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)
	mux.HandleFunc("GET /api/v1/users", requireSuperuser(usersHandler.List))
	mux.HandleFunc("POST /api/v1/users", requireSuperuser(usersHandler.Create))
	mux.HandleFunc("GET /api/v1/users/{id}", requireSuperuser(usersHandler.Get))
	mux.HandleFunc("PUT /api/v1/users/{id}", requireSuperuser(usersHandler.Update))
	mux.HandleFunc("DELETE /api/v1/users/{id}", requireSuperuser(usersHandler.Delete))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Not found", "NOT_FOUND")
	})

	var handler http.Handler = mux
	handler = authMiddleware(authService, handler)
	handler = corsMiddleware(dev, handler)
	handler = loggingMiddleware(logger, handler)
	handler = requestIDMiddleware(handler)
	handler = recoverMiddleware(logger, handler)
	handler = bodyLimitMiddleware(handler)
	return handler
}

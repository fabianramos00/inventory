//go:build !bindings

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"

	"inventory-desktop/internal/bootstrap"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/httpapi"
	"inventory-desktop/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}
	logger := cfg.Logger()

	ctx := context.Background()
	stores, err := bootstrap.Open(ctx, cfg, logger)
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}

	authService := service.NewAuthService(
		stores.Users,
		stores.RefreshTokens,
		cfg.SecretKey,
		stores.Backend,
		cfg.AccessTokenTTL(),
		cfg.RefreshTokenTTL(),
	)
	handler := httpapi.NewRouterWithBackend(authService, logger, stores.Backend, cfg.DevMode)
	server, err := httpapi.NewServer("127.0.0.1:0", handler, logger)
	if err != nil {
		_ = stores.Closer()
		logger.Error("http server failed to start", "error", err)
		os.Exit(1)
	}
	if err := server.Start(); err != nil {
		_ = stores.Closer()
		logger.Error("http server failed to start", "error", err)
		os.Exit(1)
	}

	app := NewApp(server.BaseURL(), func(ctx context.Context) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown failed", "error", err)
		}
		_ = stores.Closer()
	})
	defer app.shutdown(context.Background())

	if err := wails.Run(newWailsOptions(app)); err != nil {
		logger.Error("wails exited with error", "error", err)
	}
}

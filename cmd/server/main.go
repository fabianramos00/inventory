package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"inventory-desktop/internal/config"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/httpapi"
	"inventory-desktop/internal/repository"
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
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	if cfg.AutoMigrate {
		if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
			pool.Close()
			logger.Error("migration failed", "error", err)
			os.Exit(1)
		}
	}

	authService := service.NewAuthService(
		repository.NewUserRepository(pool),
		repository.NewRefreshTokenRepository(pool),
		cfg.SecretKey,
		cfg.AccessTokenTTL(),
		cfg.RefreshTokenTTL(),
	)
	handler := httpapi.NewRouter(authService, logger, true)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8787"
	}
	server, err := httpapi.NewServer(net.JoinHostPort("127.0.0.1", port), handler, logger)
	if err != nil {
		pool.Close()
		logger.Error("http server failed to start", "error", err)
		os.Exit(1)
	}
	if err := server.Start(); err != nil {
		pool.Close()
		logger.Error("http server failed to start", "error", err)
		os.Exit(1)
	}
	logger.Info("api server listening", "addr", server.Addr())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", "error", err)
	}
	pool.Close()
}

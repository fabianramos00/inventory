package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/bootstrap"
	"inventory-desktop/internal/config"
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
	defer stores.Closer()

	passwords := security.NewPasswordHasher(cfg.BcryptRounds)
	accessTokens := security.NewAccessTokenIssuer(cfg.SecretKey, cfg.AccessTokenTTL(), "inventory-desktop/"+stores.Backend)
	refreshTokens := security.NewRefreshTokenService()
	authService := application.NewAuthService(
		stores.Users,
		stores.RefreshTokens,
		passwords,
		accessTokens,
		refreshTokens,
		cfg.RefreshTokenTTL(),
	)
	userService := application.NewUserService(stores.Users, passwords)
	handler := httpapi.NewRouter(httpapi.Deps{
		Auth:             authService,
		Users:            userService,
		Categories:       application.NewCategoryService(stores.Categories),
		Brands:           application.NewBrandService(stores.Brands),
		Materials:        application.NewMaterialService(stores.Materials),
		MeasurementUnits: application.NewMeasurementUnitService(stores.MeasurementUnits),
		Products:         application.NewProductService(stores.Products),
		Clients:          application.NewClientService(stores.Clients),
		Sales:            application.NewSaleService(stores.Sales, stores.UnitOfWork),
		Logger:           logger,
		Backend:          stores.Backend,
		DevMode:          true,
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8787"
	}
	server, err := httpapi.NewServer(net.JoinHostPort("127.0.0.1", port), handler, logger)
	if err != nil {
		logger.Error("http server failed to start", "error", err)
		os.Exit(1)
	}
	if err := server.Start(); err != nil {
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
}

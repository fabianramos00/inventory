//go:build !bindings

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"

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
		Logger:           logger,
		Backend:          stores.Backend,
		DevMode:          cfg.DevMode,
	})
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

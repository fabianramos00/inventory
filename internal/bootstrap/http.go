package bootstrap

import (
	"log/slog"
	"net/http"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/config"
)

func NewHTTPHandler(cfg *config.Config, stores *Stores, logger *slog.Logger) http.Handler {
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
	userService := application.NewUserService(stores.Users, stores.RefreshTokens, passwords)
	return httpapi.NewRouter(httpapi.Deps{
		Auth:             authService,
		Users:            userService,
		Categories:       application.NewCategoryService(stores.Categories),
		Brands:           application.NewBrandService(stores.Brands),
		Materials:        application.NewMaterialService(stores.Materials),
		MeasurementUnits: application.NewMeasurementUnitService(stores.MeasurementUnits),
		Products:         application.NewProductService(stores.Products),
		Clients:          application.NewClientService(stores.Clients),
		Sales:            application.NewSaleService(stores.Sales, stores.UnitOfWork),
		Providers:        application.NewProviderService(stores.Providers),
		SupplierCatalogs: application.NewSupplierCatalogService(stores.SupplierCatalogs),
		PurchaseOrders:   application.NewPurchaseOrderService(stores.PurchaseOrders, stores.UnitOfWork),
		Dashboard:        application.NewDashboardService(stores.Dashboard),
		Logger:           logger,
		Backend:          stores.Backend,
		DevMode:          cfg.DevMode,
	})
}

package httpapi

import (
	"log/slog"
	"net/http"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type Deps struct {
	Auth             inbound.AuthUseCase
	Users            inbound.UserUseCase
	Categories       inbound.CatalogUseCase[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]
	Brands           inbound.CatalogUseCase[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]
	Materials        inbound.CatalogUseCase[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]
	MeasurementUnits inbound.CatalogUseCase[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]
	Products         inbound.ProductUseCase
	Clients          inbound.ClientUseCase
	Logger           *slog.Logger
	Backend          string
	DevMode          bool
}

func NewRouter(deps Deps) http.Handler {
	backend := deps.Backend
	if backend == "" {
		backend = "unknown"
	}
	logger := deps.Logger

	mux := http.NewServeMux()
	authHandler := NewAuthHandler(deps.Auth, logger)
	usersHandler := NewUsersHandler(deps.Users, logger)
	categoriesHandler := newCategoryHandler(deps.Categories, logger)
	brandsHandler := newBrandHandler(deps.Brands, logger)
	materialsHandler := newMaterialHandler(deps.Materials, logger)
	measurementUnitsHandler := newMeasurementUnitHandler(deps.MeasurementUnits, logger)
	productsHandler := NewProductHandler(deps.Products, logger)
	clientsHandler := NewClientHandler(deps.Clients, logger)

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

	registerCatalogRoutes(mux, "/api/v1/inventory/categories", categoriesHandler, true)
	registerCatalogRoutes(mux, "/api/v1/inventory/brands", brandsHandler, true)
	registerCatalogRoutes(mux, "/api/v1/inventory/materials", materialsHandler, false)
	registerCatalogRoutes(mux, "/api/v1/inventory/measurement-units", measurementUnitsHandler, false)

	mux.HandleFunc("GET /api/v1/inventory/products/stats/overview", productsHandler.Stats)
	mux.HandleFunc("GET /api/v1/inventory/products", productsHandler.List)
	mux.HandleFunc("POST /api/v1/inventory/products", productsHandler.Create)
	mux.HandleFunc("GET /api/v1/inventory/products/{id}", productsHandler.Get)
	mux.HandleFunc("PUT /api/v1/inventory/products/{id}", productsHandler.Update)
	mux.HandleFunc("DELETE /api/v1/inventory/products/{id}", productsHandler.Delete)

	mux.HandleFunc("GET /api/v1/sales/clients", clientsHandler.List)
	mux.HandleFunc("POST /api/v1/sales/clients", clientsHandler.Create)
	mux.HandleFunc("GET /api/v1/sales/clients/{id}", clientsHandler.Get)
	mux.HandleFunc("PUT /api/v1/sales/clients/{id}", clientsHandler.Update)
	mux.HandleFunc("DELETE /api/v1/sales/clients/{id}", clientsHandler.Delete)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Not found", "NOT_FOUND")
	})

	var handler http.Handler = mux
	handler = authMiddleware(deps.Auth, handler)
	handler = corsMiddleware(deps.DevMode, handler)
	handler = loggingMiddleware(logger, handler)
	handler = requestIDMiddleware(handler)
	handler = recoverMiddleware(logger, handler)
	handler = bodyLimitMiddleware(handler)
	return handler
}

func registerCatalogRoutes[T any, C any, U any](mux *http.ServeMux, prefix string, handler *catalogHandler[T, C, U], includeGet bool) {
	mux.HandleFunc("GET "+prefix, handler.List)
	mux.HandleFunc("POST "+prefix, handler.Create)
	if includeGet {
		mux.HandleFunc("GET "+prefix+"/{id}", handler.Get)
	}
	mux.HandleFunc("PUT "+prefix+"/{id}", handler.Update)
	mux.HandleFunc("DELETE "+prefix+"/{id}", handler.Delete)
}

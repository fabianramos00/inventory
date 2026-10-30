package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type protectedRoute struct {
	method string
	path   string
	body   any
}

// newFullStubServer wires every handler to a stub so auth tests exercise the
// real routing and middleware without touching a database.
func newFullStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user":    {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
		"regular": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
		"super":   {ID: 1, FullName: "Admin User", Email: "admin@example.com", IsActive: true, IsSuperuser: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:             auth,
		Users:            &stubUserService{},
		Categories:       &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{},
		Brands:           &stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{},
		Materials:        &stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{},
		MeasurementUnits: &stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{},
		Products:         &stubProductService{},
		Clients:          &stubClientService{},
		Sales:            &stubSaleService{},
		Providers:        &stubProviderService{},
		SupplierCatalogs: &stubSupplierCatalogService{},
		PurchaseOrders:   &stubPurchaseOrderService{},
		Dashboard:        &stubDashboardService{},
		Logger:           logger,
		Backend:          "sqlite",
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	server := newFullStubServer(t)

	routes := []protectedRoute{
		// Catalogs.
		{http.MethodGet, "/api/v1/inventory/categories", nil},
		{http.MethodPost, "/api/v1/inventory/categories", map[string]any{"name": "Tools"}},
		{http.MethodGet, "/api/v1/inventory/categories/1", nil},
		{http.MethodPut, "/api/v1/inventory/categories/1", map[string]any{"name": "Tools"}},
		{http.MethodDelete, "/api/v1/inventory/categories/1", nil},
		{http.MethodGet, "/api/v1/inventory/brands", nil},
		{http.MethodPost, "/api/v1/inventory/brands", map[string]any{"name": "Acme"}},
		{http.MethodGet, "/api/v1/inventory/brands/1", nil},
		{http.MethodPut, "/api/v1/inventory/brands/1", map[string]any{"name": "Acme"}},
		{http.MethodDelete, "/api/v1/inventory/brands/1", nil},
		{http.MethodGet, "/api/v1/inventory/materials", nil},
		{http.MethodPost, "/api/v1/inventory/materials", map[string]any{"name": "Steel"}},
		{http.MethodPut, "/api/v1/inventory/materials/1", map[string]any{"name": "Steel"}},
		{http.MethodDelete, "/api/v1/inventory/materials/1", nil},
		{http.MethodGet, "/api/v1/inventory/measurement-units", nil},
		{http.MethodPost, "/api/v1/inventory/measurement-units", map[string]any{"name": "Centimeter", "abbreviation": "cm"}},
		{http.MethodPut, "/api/v1/inventory/measurement-units/1", map[string]any{"abbreviation": "mm"}},
		{http.MethodDelete, "/api/v1/inventory/measurement-units/1", nil},
		// Products.
		{http.MethodGet, "/api/v1/inventory/products", nil},
		{http.MethodPost, "/api/v1/inventory/products", map[string]any{"name": "Hammer"}},
		{http.MethodGet, "/api/v1/inventory/products/1", nil},
		{http.MethodPut, "/api/v1/inventory/products/1", map[string]any{"name": "Hammer"}},
		{http.MethodDelete, "/api/v1/inventory/products/1", nil},
		{http.MethodGet, "/api/v1/inventory/products/stats/overview", nil},
		// Clients.
		{http.MethodGet, "/api/v1/sales/clients", nil},
		{http.MethodPost, "/api/v1/sales/clients", map[string]any{"name": "Ana"}},
		{http.MethodGet, "/api/v1/sales/clients/1", nil},
		{http.MethodPut, "/api/v1/sales/clients/1", map[string]any{"name": "Ana"}},
		{http.MethodDelete, "/api/v1/sales/clients/1", nil},
		// Providers and supplier catalogs.
		{http.MethodGet, "/api/v1/supply-chain/providers", nil},
		{http.MethodPost, "/api/v1/supply-chain/providers", map[string]any{"name": "Provider"}},
		{http.MethodGet, "/api/v1/supply-chain/providers/1", nil},
		{http.MethodPut, "/api/v1/supply-chain/providers/1", map[string]any{"name": "Provider"}},
		{http.MethodDelete, "/api/v1/supply-chain/providers/1", nil},
		{http.MethodGet, "/api/v1/supply-chain/supplier-catalogs", nil},
		{http.MethodGet, "/api/v1/supply-chain/supplier-catalogs/1", nil},
		// Purchase orders.
		{http.MethodGet, "/api/v1/supply-chain/purchase-orders", nil},
		{http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{}},
		{http.MethodGet, "/api/v1/supply-chain/purchase-orders/1", nil},
		{http.MethodPut, "/api/v1/supply-chain/purchase-orders/1", map[string]any{}},
		{http.MethodDelete, "/api/v1/supply-chain/purchase-orders/1", nil},
		{http.MethodGet, "/api/v1/supply-chain/purchase-orders/1/items", nil},
		{http.MethodPost, "/api/v1/supply-chain/purchase-orders/1/items", map[string]any{}},
		{http.MethodPut, "/api/v1/supply-chain/purchase-orders/1/items/2", map[string]any{}},
		{http.MethodDelete, "/api/v1/supply-chain/purchase-orders/1/items/2", nil},
		// Sales.
		{http.MethodGet, "/api/v1/sales", nil},
		{http.MethodPost, "/api/v1/sales", map[string]any{}},
		{http.MethodGet, "/api/v1/sales/stats", nil},
		{http.MethodGet, "/api/v1/sales/1", nil},
		{http.MethodPut, "/api/v1/sales/1", map[string]any{}},
		{http.MethodDelete, "/api/v1/sales/1", nil},
		{http.MethodGet, "/api/v1/sales/1/items", nil},
		{http.MethodPost, "/api/v1/sales/1/items", map[string]any{}},
		{http.MethodPut, "/api/v1/sales/1/items/2", map[string]any{}},
		{http.MethodDelete, "/api/v1/sales/1/items/2", nil},
		// Dashboard.
		{http.MethodGet, "/api/v1/dashboard/summary", nil},
		{http.MethodGet, "/api/v1/dashboard/sales-trend", nil},
		{http.MethodGet, "/api/v1/dashboard/top-products", nil},
		{http.MethodGet, "/api/v1/dashboard/order-status-distribution", nil},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			for _, token := range []string{"", "bogus"} {
				status, raw := userRequest(t, route.method, server.URL+route.path, token, route.body)
				if status != http.StatusUnauthorized {
					t.Fatalf("token %q: status = %d body = %s, want 401", token, status, raw)
				}
			}
		})
	}

	superuserRoutes := []protectedRoute{
		{http.MethodGet, "/api/v1/users", nil},
		{http.MethodPost, "/api/v1/users", map[string]any{"email": "new@example.com", "password": "password123"}},
		{http.MethodGet, "/api/v1/users/7", nil},
		{http.MethodPut, "/api/v1/users/7", map[string]any{"full_name": "Updated"}},
		{http.MethodDelete, "/api/v1/users/7", nil},
	}
	for _, route := range superuserRoutes {
		t.Run(route.method+" "+route.path+" requires superuser", func(t *testing.T) {
			status, raw := userRequest(t, route.method, server.URL+route.path, "regular", route.body)
			if status != http.StatusForbidden {
				t.Fatalf("status = %d body = %s, want 403", status, raw)
			}
			if body := decodeUserError(t, raw); body.Detail != "The user doesn't have enough privileges" {
				t.Errorf("detail = %q", body.Detail)
			}
		})
	}
}

package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
)

type stubDashboardService struct {
	summary      *domain.DashboardSummary
	summaryErr   error
	trend        []domain.SalesTrendPoint
	trendErr     error
	products     []domain.TopProduct
	productsErr  error
	topLimit     int
	distribution []domain.OrderStatusCount
	distErr      error
}

func (s *stubDashboardService) Summary(context.Context) (*domain.DashboardSummary, error) {
	if s.summaryErr != nil {
		return nil, s.summaryErr
	}
	if s.summary == nil {
		return &domain.DashboardSummary{}, nil
	}
	return s.summary, nil
}

func (s *stubDashboardService) SalesTrend(context.Context) ([]domain.SalesTrendPoint, error) {
	return s.trend, s.trendErr
}

func (s *stubDashboardService) TopProducts(_ context.Context, limit int) ([]domain.TopProduct, error) {
	s.topLimit = limit
	return s.products, s.productsErr
}

func (s *stubDashboardService) OrderStatusDistribution(context.Context) ([]domain.OrderStatusCount, error) {
	return s.distribution, s.distErr
}

func newDashboardTestServer(t *testing.T, service *stubDashboardService) *httptest.Server {
	t.Helper()
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:      auth,
		Users:     &stubUserService{},
		Dashboard: service,
		Logger:    logger,
		Backend:   "sqlite",
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDashboardRequiresAuthOnAllRoutes(t *testing.T) {
	server := newDashboardTestServer(t, &stubDashboardService{})
	paths := []string{
		"/api/v1/dashboard/summary",
		"/api/v1/dashboard/sales-trend",
		"/api/v1/dashboard/top-products",
		"/api/v1/dashboard/order-status-distribution",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+path, "", nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("no token status = %d body = %s, want 401", status, raw)
			}
			status, raw = userRequest(t, http.MethodGet, server.URL+path, "bogus", nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("invalid token status = %d body = %s, want 401", status, raw)
			}
		})
	}
}

func TestDashboardSummaryShape(t *testing.T) {
	service := &stubDashboardService{summary: &domain.DashboardSummary{
		InventoryValue:      decimal.RequireFromString("40.00"),
		TotalSalesToday:     decimal.RequireFromString("28.50"),
		PendingPaymentSales: decimal.RequireFromString("20.00"),
		PendingOrders:       3,
		DeliveredOrders:     1,
	}}
	server := newDashboardTestServer(t, service)
	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/summary", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	var body struct {
		InventoryValue      json.Number `json:"inventory_value"`
		TotalSalesToday     json.Number `json:"total_sales_today"`
		PendingOrders       int64       `json:"pending_orders"`
		DeliveredOrders     int64       `json:"delivered_orders"`
		PendingPaymentSales json.Number `json:"pending_payment_sales"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.InventoryValue.String() != "40" || body.TotalSalesToday.String() != "28.5" ||
		body.PendingPaymentSales.String() != "20" {
		t.Fatalf("money = %s/%s/%s", body.InventoryValue, body.TotalSalesToday, body.PendingPaymentSales)
	}
	if body.PendingOrders != 3 || body.DeliveredOrders != 1 {
		t.Fatalf("counts = %d/%d", body.PendingOrders, body.DeliveredOrders)
	}
}

func TestDashboardTrendAndDistributionShapes(t *testing.T) {
	service := &stubDashboardService{
		trend: []domain.SalesTrendPoint{
			{Date: "2026-10-23", SalesAmount: decimal.RequireFromString("300.00"), OrdersCost: decimal.RequireFromString("90.00")},
			{Date: "2026-10-24", SalesAmount: decimal.RequireFromString("28.50"), OrdersCost: decimal.RequireFromString("60.00")},
		},
		distribution: []domain.OrderStatusCount{
			{Status: "pending", Count: 3, Percentage: 50},
			{Status: "sent", Count: 2, Percentage: 33.33},
			{Status: "received", Count: 1, Percentage: 16.67},
		},
	}
	server := newDashboardTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/sales-trend", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("trend status = %d body = %s", status, raw)
	}
	var trend struct {
		Data []struct {
			Date        string      `json:"date"`
			SalesAmount json.Number `json:"sales_amount"`
			OrdersCost  json.Number `json:"orders_cost"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &trend); err != nil {
		t.Fatal(err)
	}
	if len(trend.Data) != 2 || trend.Data[1].Date != "2026-10-24" ||
		trend.Data[1].SalesAmount.String() != "28.5" || trend.Data[1].OrdersCost.String() != "60" {
		t.Fatalf("trend body = %s", raw)
	}

	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/order-status-distribution", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("distribution status = %d body = %s", status, raw)
	}
	var distribution struct {
		Data []struct {
			Status     string  `json:"status"`
			Count      int64   `json:"count"`
			Percentage float64 `json:"percentage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &distribution); err != nil {
		t.Fatal(err)
	}
	if len(distribution.Data) != 3 || distribution.Data[0].Status != "pending" ||
		distribution.Data[0].Count != 3 || distribution.Data[1].Percentage != 33.33 {
		t.Fatalf("distribution body = %s", raw)
	}
}

func TestDashboardTopProductsShapeAndLimit(t *testing.T) {
	service := &stubDashboardService{products: []domain.TopProduct{{
		ProductID:         7,
		ProductName:       "Martillo",
		ProductSKU:        "SKU-7",
		BrandName:         nil,
		CategoryName:      strPtr("Herramientas"),
		MaterialName:      nil,
		TotalQuantitySold: 2,
		TotalSalesAmount:  decimal.RequireFromString("21.00"),
	}}}
	server := newDashboardTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/top-products", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.topLimit != 10 {
		t.Fatalf("default limit = %d, want 10", service.topLimit)
	}
	var body struct {
		Products []struct {
			ProductID         int64       `json:"product_id"`
			ProductName       string      `json:"product_name"`
			ProductSKU        string      `json:"product_sku"`
			BrandName         *string     `json:"brand_name"`
			CategoryName      *string     `json:"category_name"`
			MaterialName      *string     `json:"material_name"`
			TotalQuantitySold int64       `json:"total_quantity_sold"`
			TotalSalesAmount  json.Number `json:"total_sales_amount"`
		} `json:"products"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Products) != 1 || body.Products[0].ProductID != 7 ||
		body.Products[0].BrandName != nil || body.Products[0].MaterialName != nil ||
		body.Products[0].CategoryName == nil || *body.Products[0].CategoryName != "Herramientas" ||
		body.Products[0].TotalQuantitySold != 2 || body.Products[0].TotalSalesAmount.String() != "21" {
		t.Fatalf("top products body = %s", raw)
	}

	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/top-products?limit=5", "user", nil)
	if status != http.StatusOK || service.topLimit != 5 {
		t.Fatalf("limit 5 status = %d limit = %d body = %s", status, service.topLimit, raw)
	}
	for _, limit := range []string{"", "0", "101", "-1", "abc"} {
		status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/top-products?limit="+limit, "user", nil)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("limit %q status = %d body = %s, want 422", limit, status, raw)
		}
	}
}

func TestDashboardEmptyShapesAreArrays(t *testing.T) {
	server := newDashboardTestServer(t, &stubDashboardService{})
	for _, path := range []string{
		"/api/v1/dashboard/sales-trend",
		"/api/v1/dashboard/top-products",
		"/api/v1/dashboard/order-status-distribution",
	} {
		status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
		if status != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", path, status, raw)
		}
		if string(raw) == "" || len(raw) < 2 {
			t.Fatalf("%s empty body = %s", path, raw)
		}
		if containsNull(raw) {
			t.Fatalf("%s returned null instead of an empty array: %s", path, raw)
		}
	}
}

func containsNull(raw []byte) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, v := range typed {
			if v == nil {
				return true
			}
		}
	}
	return false
}

func TestDashboardServiceErrorReturns500(t *testing.T) {
	server := newDashboardTestServer(t, &stubDashboardService{summaryErr: errors.New("boom")})
	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/summary", "user", nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500", status, raw)
	}
	if string(raw) == "" || string(raw) == "null\n" {
		t.Fatalf("body = %q, want a normalized error envelope", raw)
	}
}

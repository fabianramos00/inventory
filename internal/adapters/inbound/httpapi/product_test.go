package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type stubProductService struct {
	listResult *inbound.ProductList
	listErr    error
	listFilter inbound.ProductFilter

	getResult *domain.ProductDetail
	getErr    error

	createResult *domain.Product
	createErr    error
	createInput  inbound.CreateProductInput

	updateResult *domain.Product
	updateErr    error
	updateID     int64
	updateInput  inbound.UpdateProductInput

	deleteResult *domain.Product
	deleteErr    error
	deleteID     int64

	stats    *domain.ProductStats
	statsErr error
}

func (s *stubProductService) List(_ context.Context, filter inbound.ProductFilter) (*inbound.ProductList, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.ProductList{Skip: filter.Skip, Limit: filter.Limit}, nil
	}
	return s.listResult, nil
}

func (s *stubProductService) Get(_ context.Context, id int64) (*domain.ProductDetail, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getResult == nil {
		return &domain.ProductDetail{Product: domain.Product{ID: id}}, nil
	}
	return s.getResult, nil
}

func (s *stubProductService) Create(_ context.Context, input inbound.CreateProductInput) (*domain.Product, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult == nil {
		return &domain.Product{ID: 1}, nil
	}
	return s.createResult, nil
}

func (s *stubProductService) Update(_ context.Context, id int64, input inbound.UpdateProductInput) (*domain.Product, error) {
	s.updateID, s.updateInput = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateResult == nil {
		return &domain.Product{ID: id}, nil
	}
	return s.updateResult, nil
}

func (s *stubProductService) Delete(_ context.Context, id int64) (*domain.Product, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if s.deleteResult == nil {
		return &domain.Product{ID: id}, nil
	}
	return s.deleteResult, nil
}

func (s *stubProductService) Stats(context.Context) (*domain.ProductStats, error) {
	return s.stats, s.statsErr
}

func newProductTestServer(t *testing.T, service *stubProductService) *httptest.Server {
	t.Helper()
	if service == nil {
		service = &stubProductService{}
	}
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:     auth,
		Users:    &stubUserService{},
		Products: service,
		Logger:   logger,
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProductsRequireAuth(t *testing.T) {
	server := newProductTestServer(t, nil)
	tests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/inventory/products", nil},
		{http.MethodPost, "/api/v1/inventory/products", map[string]any{"name": "Hammer"}},
		{http.MethodGet, "/api/v1/inventory/products/1", nil},
		{http.MethodPut, "/api/v1/inventory/products/1", map[string]any{"name": "Hammer"}},
		{http.MethodDelete, "/api/v1/inventory/products/1", nil},
		{http.MethodGet, "/api/v1/inventory/products/stats/overview", nil},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "", tt.body)
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d body = %s, want 401", status, raw)
			}
		})
	}
}

func TestProductListShapeFiltersAndPagination(t *testing.T) {
	price := decimal.RequireFromString("12.5")
	service := &stubProductService{listResult: &inbound.ProductList{
		Items: []*domain.ProductDetail{{
			Product: domain.Product{
				ID: 3, SKU: "SKU-ABCDEF12", Name: "Hammer", Price: price, Cost: decimal.RequireFromString("5"),
				StockQuantity: 2, LowStockThreshold: 5, BrandID: 1, CategoryID: 2, MaterialID: 3,
			},
			Category:        &domain.Category{ID: 2, Name: "Tools"},
			Material:        &domain.Material{ID: 3, Name: "Steel"},
			Brand:           &domain.Brand{ID: 1, Name: "Acme"},
			MeasurementUnit: &domain.MeasurementUnit{ID: 4, Name: "Centimeter", Abbreviation: "cm"},
		}},
		HasNext: true,
		Skip:    2,
		Limit:   1,
	}}
	server := newProductTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet,
		server.URL+"/api/v1/inventory/products?skip=2&limit=1&search=%20hammer%20&category_id=2&brand_id=1&material_id=3&stock_status=low_stock",
		"user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if service.listFilter.Search != "hammer" || service.listFilter.Skip != 2 || service.listFilter.Limit != 1 ||
		service.listFilter.CategoryID == nil || *service.listFilter.CategoryID != 2 ||
		service.listFilter.BrandID == nil || *service.listFilter.BrandID != 1 ||
		service.listFilter.MaterialID == nil || *service.listFilter.MaterialID != 3 ||
		service.listFilter.StockStatus != inbound.StockStatusLowStock {
		t.Fatalf("filter = %+v", service.listFilter)
	}

	var body struct {
		Items   []map[string]json.RawMessage `json:"items"`
		HasNext bool                         `json:"has_next"`
		Skip    int                          `json:"skip"`
		Limit   int                          `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || !body.HasNext || body.Skip != 2 || body.Limit != 1 {
		t.Fatalf("body = %+v", body)
	}
	item := body.Items[0]
	if rawPrice, ok := item["price"]; !ok || bytes.HasPrefix(rawPrice, []byte(`"`)) {
		t.Fatalf("price = %s, want a JSON number", rawPrice)
	}
	for _, key := range []string{"category", "material", "brand", "measurement_unit", "stock_quantity", "low_stock_threshold"} {
		if _, ok := item[key]; !ok {
			t.Errorf("item is missing %q", key)
		}
	}
	if string(item["category"]) != `{"id":2,"name":"Tools","code":null,"description":null}` {
		t.Errorf("category = %s", item["category"])
	}
	if string(item["price"]) != "12.5" {
		t.Errorf("price = %s, want 12.5", item["price"])
	}
}

func TestProductListDefaultsAndValidation(t *testing.T) {
	service := &stubProductService{}
	server := newProductTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/products", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if service.listFilter.Skip != 0 || service.listFilter.Limit != 100 || service.listFilter.StockStatus != "" {
		t.Fatalf("defaults = %+v", service.listFilter)
	}

	paths := []string{
		"/api/v1/inventory/products?skip=-1",
		"/api/v1/inventory/products?limit=0",
		"/api/v1/inventory/products?limit=201",
		"/api/v1/inventory/products?search=" + strings.Repeat("a", 257),
		"/api/v1/inventory/products?category_id=0",
		"/api/v1/inventory/products?brand_id=abc",
		"/api/v1/inventory/products?material_id=-1",
		"/api/v1/inventory/products?stock_status=bogus",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
			if body := decodeUserError(t, raw); body.Code != "VALIDATION_ERROR" {
				t.Errorf("code = %q, want VALIDATION_ERROR", body.Code)
			}
		})
	}
}

func TestProductCreateResponseAndInput(t *testing.T) {
	service := &stubProductService{createResult: &domain.Product{
		ID: 5, SKU: "SKU-12345678", Name: "Hammer", Price: decimal.RequireFromString("10"),
		Cost: decimal.RequireFromString("5"), StockQuantity: 0, LowStockThreshold: 10,
		BrandID: 1, CategoryID: 2, MaterialID: 3,
	}}
	server := newProductTestServer(t, service)

	status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/inventory/products", "user", map[string]any{
		"name": "Hammer", "price": 10, "cost": 5, "brand_id": 1, "category_id": 2, "material_id": 3,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if service.createInput.StockQuantity.Set || service.createInput.LowStockThreshold.Set {
		t.Fatalf("omitted stock input = %+v/%+v, want unset", service.createInput.StockQuantity, service.createInput.LowStockThreshold)
	}
	if service.createInput.Name != "Hammer" || !service.createInput.Price.Equal(decimal.RequireFromString("10")) {
		t.Errorf("input = %+v", service.createInput)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"category", "material", "brand", "measurement_unit"} {
		if _, ok := fields[key]; ok {
			t.Errorf("plain product response must not include %q", key)
		}
	}
	if string(fields["price"]) != "10" {
		t.Errorf("price = %s, want 10", fields["price"])
	}

	status, raw = userRequest(t, http.MethodPost, server.URL+"/api/v1/inventory/products", "user", map[string]any{
		"name": "Hammer", "price": 10, "cost": 5, "brand_id": 1, "category_id": 2, "material_id": 3,
		"stock_quantity": nil, "low_stock_threshold": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if !service.createInput.StockQuantity.Set || service.createInput.StockQuantity.Value != nil {
		t.Errorf("null stock input = %+v, want set with nil value", service.createInput.StockQuantity)
	}
	if !service.createInput.LowStockThreshold.Set || service.createInput.LowStockThreshold.Value != nil {
		t.Errorf("null threshold input = %+v, want set with nil value", service.createInput.LowStockThreshold)
	}
}

func TestProductUpdateNullVersusOmitted(t *testing.T) {
	service := &stubProductService{updateResult: &domain.Product{ID: 7, Name: "Old"}}
	server := newProductTestServer(t, service)

	status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/inventory/products/7", "user", map[string]any{
		"size_value": nil, "measurement_unit_id": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if !service.updateInput.SizeValue.Set || service.updateInput.SizeValue.Value != nil {
		t.Errorf("size input = %+v, want set with nil", service.updateInput.SizeValue)
	}
	if !service.updateInput.MeasurementUnitID.Set || service.updateInput.MeasurementUnitID.Value != nil {
		t.Errorf("unit input = %+v, want set with nil", service.updateInput.MeasurementUnitID)
	}
	if service.updateInput.Name.Set || service.updateInput.Price.Set {
		t.Errorf("omitted fields were marked present: %+v", service.updateInput)
	}

	status, raw = userRequest(t, http.MethodPut, server.URL+"/api/v1/inventory/products/7", "user", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if service.updateInput.SizeValue.Set || service.updateInput.MeasurementUnitID.Set || service.updateInput.Name.Set {
		t.Errorf("empty update marked fields present: %+v", service.updateInput)
	}

	status, raw = userRequest(t, http.MethodPut, server.URL+"/api/v1/inventory/products/7", "user", map[string]any{
		"name": nil, "price": nil, "stock_quantity": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	if !service.updateInput.Name.Set || service.updateInput.Name.Value != nil ||
		!service.updateInput.Price.Set || service.updateInput.Price.Value != nil ||
		!service.updateInput.StockQuantity.Set || service.updateInput.StockQuantity.Value != nil {
		t.Errorf("null required fields = %+v", service.updateInput)
	}
}

func TestProductErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		service    *stubProductService
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{
			name:       "missing product",
			method:     http.MethodGet,
			path:       "/api/v1/inventory/products/99",
			service:    &stubProductService{getErr: domain.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantDetail: "Product not found",
		},
		{
			name:       "duplicate sku",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/products",
			body:       map[string]any{"name": "Hammer", "price": 1, "cost": 1, "brand_id": 1, "category_id": 1, "material_id": 1},
			service:    &stubProductService{createErr: domain.ErrConflict},
			wantStatus: http.StatusBadRequest,
			wantCode:   "UNIQUE_VIOLATION",
		},
		{
			name:       "foreign key",
			method:     http.MethodDelete,
			path:       "/api/v1/inventory/products/1",
			service:    &stubProductService{deleteErr: domain.ErrForeignKeyViolation},
			wantStatus: http.StatusBadRequest,
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "validation",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/products",
			body:       map[string]any{"name": "", "price": 0, "cost": 0},
			service:    &stubProductService{createErr: domain.ErrValidation},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "invalid id",
			method:     http.MethodGet,
			path:       "/api/v1/inventory/products/not-a-number",
			service:    &stubProductService{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "unknown field",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/products",
			body:       map[string]any{"name": "Hammer", "min_stock_quantity": 2},
			service:    &stubProductService{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newProductTestServer(t, tt.service)
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "user", tt.body)
			if status != tt.wantStatus {
				t.Fatalf("status = %d body = %s, want %d", status, raw, tt.wantStatus)
			}
			body := decodeUserError(t, raw)
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if tt.wantDetail != "" && body.Detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", body.Detail, tt.wantDetail)
			}
		})
	}
}

func TestProductStatsShape(t *testing.T) {
	service := &stubProductService{stats: &domain.ProductStats{
		TotalProducts:       3,
		TotalStock:          9,
		LowStockCount:       2,
		TotalInventoryValue: decimal.RequireFromString("1234.56"),
	}}
	server := newProductTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/products/stats/overview", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if string(body["total_products"]) != "3" || string(body["total_stock"]) != "9" || string(body["low_stock_count"]) != "2" {
		t.Errorf("counts = %s/%s/%s", body["total_products"], body["total_stock"], body["low_stock_count"])
	}
	if string(body["total_inventory_value"]) != "1234.56" {
		t.Errorf("value = %s, want 1234.56", body["total_inventory_value"])
	}
	if bytes.HasPrefix(body["total_inventory_value"], []byte(`"`)) {
		t.Error("total_inventory_value must be a JSON number")
	}
}

func TestProductGetReturnsDetail(t *testing.T) {
	service := &stubProductService{getResult: &domain.ProductDetail{
		Product:         domain.Product{ID: 4, SKU: "SKU-00000001", Name: "Hammer", Price: decimal.RequireFromString("10"), Cost: decimal.RequireFromString("5")},
		Category:        &domain.Category{ID: 1, Name: "Tools"},
		MeasurementUnit: &domain.MeasurementUnit{ID: 2, Name: "Unit", Abbreviation: "u"},
	}}
	server := newProductTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/products/4", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if string(body["category"]) != `{"id":1,"name":"Tools","code":null,"description":null}` {
		t.Errorf("category = %s", body["category"])
	}
	if string(body["measurement_unit"]) != `{"id":2,"name":"Unit","abbreviation":"u"}` {
		t.Errorf("measurement_unit = %s", body["measurement_unit"])
	}
	if _, ok := body["brand"]; !ok {
		t.Error("brand key is missing")
	}
}

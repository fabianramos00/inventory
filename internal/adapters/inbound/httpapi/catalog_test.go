package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type stubCatalogService[T any, C any, U any] struct {
	listResult *inbound.CatalogList[T]
	listErr    error

	getResult *T
	getErr    error

	createResult *T
	createErr    error

	updateResult *T
	updateErr    error

	deleteResult *T
	deleteErr    error

	listSkip   int
	listLimit  int
	listSearch string
	getID      int64
	createIn   C
	updateID   int64
	updateIn   U
	deleteID   int64
}

func (s *stubCatalogService[T, C, U]) List(_ context.Context, skip, limit int, search string) (*inbound.CatalogList[T], error) {
	s.listSkip, s.listLimit, s.listSearch = skip, limit, search
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.CatalogList[T]{Skip: skip, Limit: limit}, nil
	}
	return s.listResult, nil
}

func (s *stubCatalogService[T, C, U]) Get(_ context.Context, id int64) (*T, error) {
	s.getID = id
	return s.getResult, s.getErr
}

func (s *stubCatalogService[T, C, U]) Create(_ context.Context, input C) (*T, error) {
	s.createIn = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult == nil {
		return new(T), nil
	}
	return s.createResult, nil
}

func (s *stubCatalogService[T, C, U]) Update(_ context.Context, id int64, input U) (*T, error) {
	s.updateID, s.updateIn = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateResult == nil {
		return new(T), nil
	}
	return s.updateResult, nil
}

func (s *stubCatalogService[T, C, U]) Delete(_ context.Context, id int64) (*T, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if s.deleteResult == nil {
		return new(T), nil
	}
	return s.deleteResult, nil
}

type catalogDeps struct {
	categories *stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]
	brands     *stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]
	materials  *stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]
	units      *stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]
}

func newCatalogTestServer(t *testing.T, deps catalogDeps) (*httptest.Server, catalogDeps) {
	t.Helper()
	if deps.categories == nil {
		deps.categories = &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{}
	}
	if deps.brands == nil {
		deps.brands = &stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{}
	}
	if deps.materials == nil {
		deps.materials = &stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{}
	}
	if deps.units == nil {
		deps.units = &stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{}
	}
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:             auth,
		Users:            &stubUserService{},
		Categories:       deps.categories,
		Brands:           deps.brands,
		Materials:        deps.materials,
		MeasurementUnits: deps.units,
		Logger:           logger,
	}))
	t.Cleanup(server.Close)
	return server, deps
}

func TestCatalogRequiresAuth(t *testing.T) {
	server, _ := newCatalogTestServer(t, catalogDeps{})
	tests := []struct {
		method string
		path   string
		body   any
	}{
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

func TestCatalogRejectsInvalidToken(t *testing.T) {
	server, _ := newCatalogTestServer(t, catalogDeps{})
	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/categories", "expired", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s, want 401", status, raw)
	}
	body := decodeUserError(t, raw)
	if body.Detail != "Could not validate credentials" || body.Code != "INVALID_TOKEN" {
		t.Fatalf("error = %+v, want invalid-token response", body)
	}
}

func TestCatalogNotFoundDetails(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		deps       catalogDeps
		wantDetail string
	}{
		{
			name:       "category",
			method:     http.MethodGet,
			path:       "/api/v1/inventory/categories/99",
			deps:       catalogDeps{categories: &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{getErr: domain.ErrNotFound}},
			wantDetail: "Category not found",
		},
		{
			name:       "brand",
			method:     http.MethodGet,
			path:       "/api/v1/inventory/brands/99",
			deps:       catalogDeps{brands: &stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{getErr: domain.ErrNotFound}},
			wantDetail: "Brand not found",
		},
		{
			name:       "material",
			method:     http.MethodPut,
			path:       "/api/v1/inventory/materials/99",
			body:       map[string]any{"name": "Steel"},
			deps:       catalogDeps{materials: &stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{updateErr: domain.ErrNotFound}},
			wantDetail: "Material not found",
		},
		{
			name:       "measurement unit",
			method:     http.MethodDelete,
			path:       "/api/v1/inventory/measurement-units/99",
			deps:       catalogDeps{units: &stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{deleteErr: domain.ErrNotFound}},
			wantDetail: "Unit of Measure not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newCatalogTestServer(t, tt.deps)
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "user", tt.body)
			if status != http.StatusNotFound {
				t.Fatalf("status = %d body = %s, want 404", status, raw)
			}
			body := decodeUserError(t, raw)
			if body.Detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", body.Detail, tt.wantDetail)
			}
		})
	}
}

func TestCatalogConflictAndForeignKeyCodes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		deps       catalogDeps
		wantStatus int
		wantCode   string
	}{
		{
			name:       "duplicate category",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/categories",
			body:       map[string]any{"name": "Tools"},
			deps:       catalogDeps{categories: &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{createErr: domain.ErrConflict}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "UNIQUE_VIOLATION",
		},
		{
			name:       "duplicate brand",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/brands",
			body:       map[string]any{"name": "Acme"},
			deps:       catalogDeps{brands: &stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{createErr: domain.ErrConflict}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "UNIQUE_VIOLATION",
		},
		{
			name:       "duplicate material",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/materials",
			body:       map[string]any{"name": "Steel"},
			deps:       catalogDeps{materials: &stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{createErr: domain.ErrConflict}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "UNIQUE_VIOLATION",
		},
		{
			name:       "duplicate measurement unit",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/measurement-units",
			body:       map[string]any{"name": "Centimeter", "abbreviation": "cm"},
			deps:       catalogDeps{units: &stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{createErr: domain.ErrConflict}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "UNIQUE_VIOLATION",
		},
		{
			name:       "referenced category delete",
			method:     http.MethodDelete,
			path:       "/api/v1/inventory/categories/1",
			deps:       catalogDeps{categories: &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{deleteErr: domain.ErrForeignKeyViolation}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "referenced brand delete",
			method:     http.MethodDelete,
			path:       "/api/v1/inventory/brands/1",
			deps:       catalogDeps{brands: &stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{deleteErr: domain.ErrForeignKeyViolation}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "referenced material delete",
			method:     http.MethodDelete,
			path:       "/api/v1/inventory/materials/1",
			deps:       catalogDeps{materials: &stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{deleteErr: domain.ErrForeignKeyViolation}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "referenced measurement unit delete",
			method:     http.MethodDelete,
			path:       "/api/v1/inventory/measurement-units/1",
			deps:       catalogDeps{units: &stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{deleteErr: domain.ErrForeignKeyViolation}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "validation",
			method:     http.MethodPost,
			path:       "/api/v1/inventory/categories",
			body:       map[string]any{"name": ""},
			deps:       catalogDeps{categories: &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{createErr: fmt.Errorf("%w: name is required", domain.ErrValidation)}},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "invalid id",
			method:     http.MethodGet,
			path:       "/api/v1/inventory/categories/not-a-number",
			deps:       catalogDeps{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newCatalogTestServer(t, tt.deps)
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "user", tt.body)
			if status != tt.wantStatus {
				t.Fatalf("status = %d body = %s, want %d", status, raw, tt.wantStatus)
			}
			body := decodeUserError(t, raw)
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Detail == "" {
				t.Error("detail is empty")
			}
		})
	}
}

type categoryJSON struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
}

func TestCatalogListShape(t *testing.T) {
	code := "TOL"
	description := "Hand tools"
	categories := &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{
		listResult: &inbound.CatalogList[domain.Category]{
			Items:   []*domain.Category{{ID: 3, Name: "Tools", Code: &code, Description: &description}},
			HasNext: true,
			Skip:    2,
			Limit:   1,
		},
	}
	server, deps := newCatalogTestServer(t, catalogDeps{categories: categories})

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/categories?skip=2&limit=1&search=%20tools%20", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	var body struct {
		Items   []categoryJSON `json:"items"`
		HasNext bool           `json:"has_next"`
		Skip    int            `json:"skip"`
		Limit   int            `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != 3 || body.Items[0].Name != "Tools" ||
		body.Items[0].Code == nil || *body.Items[0].Code != code ||
		body.Items[0].Description == nil || *body.Items[0].Description != description {
		t.Fatalf("items = %+v", body.Items)
	}
	if !body.HasNext || body.Skip != 2 || body.Limit != 1 {
		t.Errorf("pagination = has_next:%v skip:%d limit:%d, want true/2/1", body.HasNext, body.Skip, body.Limit)
	}
	if deps.categories.listSearch != "tools" {
		t.Errorf("search = %q, want trimmed tools", deps.categories.listSearch)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"items", "has_next", "skip", "limit"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("response is missing %q", key)
		}
	}
}

func TestCatalogListDefaults(t *testing.T) {
	service := &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{}
	server, _ := newCatalogTestServer(t, catalogDeps{categories: service})

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/categories", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	var body struct {
		Skip  int `json:"skip"`
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Skip != 0 || body.Limit != 100 {
		t.Errorf("defaults = skip %d limit %d, want 0/100", body.Skip, body.Limit)
	}

	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/inventory/categories?limit=200", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200 for limit 200", status, raw)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Limit != 200 {
		t.Errorf("limit = %d, want 200", body.Limit)
	}
}

func TestCatalogNoGetByID(t *testing.T) {
	server, deps := newCatalogTestServer(t, catalogDeps{})
	for _, path := range []string{"/api/v1/inventory/materials/1", "/api/v1/inventory/measurement-units/1"} {
		t.Run(path, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
			if status != http.StatusNotFound {
				t.Fatalf("status = %d body = %s, want 404", status, raw)
			}
		})
	}
	if deps.materials.getID != 0 || deps.units.getID != 0 {
		t.Errorf("GET by id reached the service: materials=%d units=%d", deps.materials.getID, deps.units.getID)
	}
}

func TestCatalogCRUD(t *testing.T) {
	t.Run("category create", func(t *testing.T) {
		code := "TOL"
		description := "Hand tools"
		service := &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{
			createResult: &domain.Category{ID: 10, Name: "Tools", Code: &code, Description: &description},
		}
		server, _ := newCatalogTestServer(t, catalogDeps{categories: service})

		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/inventory/categories", "user", map[string]any{
			"name": "Tools", "code": "TOL", "description": "Hand tools",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body categoryJSON
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 10 || body.Name != "Tools" || body.Code == nil || *body.Code != code || body.Description == nil || *body.Description != description {
			t.Fatalf("response = %+v", body)
		}
		if service.createIn.Name != "Tools" || service.createIn.Code == nil || *service.createIn.Code != "TOL" {
			t.Errorf("service input = %+v", service.createIn)
		}
	})

	t.Run("category partial update", func(t *testing.T) {
		service := &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{
			updateResult: &domain.Category{ID: 7, Name: "Old", Description: func() *string { v := "new"; return &v }()},
		}
		server, _ := newCatalogTestServer(t, catalogDeps{categories: service})

		status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/inventory/categories/7", "user", map[string]any{"description": "new"})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		if service.updateID != 7 || service.updateIn.Name != nil || service.updateIn.Code != nil ||
			service.updateIn.Description == nil || *service.updateIn.Description != "new" {
			t.Errorf("service update = id %d input %+v", service.updateID, service.updateIn)
		}
	})

	t.Run("category delete", func(t *testing.T) {
		service := &stubCatalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{
			deleteResult: &domain.Category{ID: 7, Name: "Tools"},
		}
		server, _ := newCatalogTestServer(t, catalogDeps{categories: service})

		status, raw := userRequest(t, http.MethodDelete, server.URL+"/api/v1/inventory/categories/7", "user", nil)
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body categoryJSON
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 7 || service.deleteID != 7 {
			t.Errorf("delete response id = %d service id = %d, want 7", body.ID, service.deleteID)
		}
	})

	t.Run("brand create", func(t *testing.T) {
		logo := "https://example.com/logo.png"
		service := &stubCatalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{
			createResult: &domain.Brand{ID: 4, Name: "Acme", LogoURL: &logo},
		}
		server, _ := newCatalogTestServer(t, catalogDeps{brands: service})

		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/inventory/brands", "user", map[string]any{
			"name": "Acme", "logo_url": logo,
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body struct {
			ID      int64   `json:"id"`
			Name    string  `json:"name"`
			LogoURL *string `json:"logo_url"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 4 || body.Name != "Acme" || body.LogoURL == nil || *body.LogoURL != logo {
			t.Fatalf("response = %+v", body)
		}
	})

	t.Run("material create", func(t *testing.T) {
		service := &stubCatalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{
			createResult: &domain.Material{ID: 5, Name: "Steel"},
		}
		server, _ := newCatalogTestServer(t, catalogDeps{materials: service})

		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/inventory/materials", "user", map[string]any{"name": "Steel"})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		if service.createIn.Name != "Steel" {
			t.Errorf("service input = %+v", service.createIn)
		}
	})

	t.Run("measurement unit create", func(t *testing.T) {
		service := &stubCatalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{
			createResult: &domain.MeasurementUnit{ID: 6, Name: "Centimeter", Abbreviation: "cm"},
		}
		server, _ := newCatalogTestServer(t, catalogDeps{units: service})

		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/inventory/measurement-units", "user", map[string]any{
			"name": "Centimeter", "abbreviation": "cm",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body struct {
			ID           int64  `json:"id"`
			Name         string `json:"name"`
			Abbreviation string `json:"abbreviation"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 6 || body.Name != "Centimeter" || body.Abbreviation != "cm" {
			t.Fatalf("response = %+v", body)
		}
	})
}

func TestCatalogQueryAndBodyValidation(t *testing.T) {
	server, _ := newCatalogTestServer(t, catalogDeps{})

	queryTests := []string{
		"/api/v1/inventory/categories?skip=-1",
		"/api/v1/inventory/categories?limit=0",
		"/api/v1/inventory/categories?limit=201",
		"/api/v1/inventory/categories?search=" + strings.Repeat("a", 257),
	}
	for _, path := range queryTests {
		t.Run(path, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
			body := decodeUserError(t, raw)
			if body.Code != "VALIDATION_ERROR" {
				t.Errorf("code = %q, want VALIDATION_ERROR", body.Code)
			}
		})
	}

	bodyTests := []struct {
		name string
		raw  string
	}{
		{name: "malformed json", raw: `{"name":`},
		{name: "unknown field", raw: `{"name":"Tools","bogus":true}`},
	}
	for _, tt := range bodyTests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/inventory/categories", strings.NewReader(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer user")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			raw, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", response.StatusCode, raw)
			}
			body := decodeUserError(t, raw)
			if body.Code != "VALIDATION_ERROR" {
				t.Errorf("code = %q, want VALIDATION_ERROR", body.Code)
			}
		})
	}
}

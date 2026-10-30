package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type stubProviderService struct {
	listResult *inbound.ProviderList
	listErr    error
	listFilter inbound.ProviderFilter

	getResult *domain.Provider
	getErr    error
	getID     int64

	createResult *domain.Provider
	createErr    error
	createInput  inbound.CreateProviderInput

	updateResult *domain.Provider
	updateErr    error
	updateID     int64
	updateInput  inbound.UpdateProviderInput

	deleteResult *domain.Provider
	deleteErr    error
	deleteID     int64
}

func (s *stubProviderService) List(_ context.Context, filter inbound.ProviderFilter) (*inbound.ProviderList, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.ProviderList{Skip: filter.Skip, Limit: filter.Limit}, nil
	}
	return s.listResult, nil
}

func (s *stubProviderService) Get(_ context.Context, id int64) (*domain.Provider, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getResult == nil {
		return &domain.Provider{ID: id, Name: "Provider"}, nil
	}
	return s.getResult, nil
}

func (s *stubProviderService) Create(_ context.Context, input inbound.CreateProviderInput) (*domain.Provider, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult == nil {
		return &domain.Provider{ID: 1, Name: input.Name, ContactInfo: input.ContactInfo, Email: input.Email, Phone: input.Phone}, nil
	}
	return s.createResult, nil
}

func (s *stubProviderService) Update(_ context.Context, id int64, input inbound.UpdateProviderInput) (*domain.Provider, error) {
	s.updateID, s.updateInput = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateResult == nil {
		return &domain.Provider{ID: id, Name: "Provider"}, nil
	}
	return s.updateResult, nil
}

func (s *stubProviderService) Delete(_ context.Context, id int64) (*domain.Provider, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if s.deleteResult == nil {
		return &domain.Provider{ID: id, Name: "Provider"}, nil
	}
	return s.deleteResult, nil
}

type stubSupplierCatalogService struct {
	listResult *inbound.SupplierCatalogList
	listErr    error
	listFilter inbound.SupplierCatalogFilter

	getResult *domain.SupplierCatalog
	getErr    error
	getID     int64
}

func (s *stubSupplierCatalogService) List(_ context.Context, filter inbound.SupplierCatalogFilter) (*inbound.SupplierCatalogList, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.SupplierCatalogList{Skip: filter.Skip, Limit: filter.Limit}, nil
	}
	return s.listResult, nil
}

func (s *stubSupplierCatalogService) Get(_ context.Context, id int64) (*domain.SupplierCatalog, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getResult == nil {
		return &domain.SupplierCatalog{ID: id}, nil
	}
	return s.getResult, nil
}

func newSupplyChainTestServer(t *testing.T, providers inbound.ProviderUseCase, catalogs inbound.SupplierCatalogUseCase) *httptest.Server {
	t.Helper()
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:             auth,
		Users:            &stubUserService{},
		Providers:        providers,
		SupplierCatalogs: catalogs,
		Logger:           logger,
	}))
	t.Cleanup(server.Close)
	return server
}

type providerJSON struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	ContactInfo *string `json:"contact_info"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
}

func providerPtr(value string) *string { return &value }

func TestSupplyChainAllowsAnyActiveUser(t *testing.T) {
	server := newSupplyChainTestServer(t, &stubProviderService{}, &stubSupplierCatalogService{})
	tests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/supply-chain/providers", nil},
		{http.MethodPost, "/api/v1/supply-chain/providers", map[string]any{"name": "Provider"}},
		{http.MethodGet, "/api/v1/supply-chain/providers/1", nil},
		{http.MethodPut, "/api/v1/supply-chain/providers/1", map[string]any{"name": "Provider"}},
		{http.MethodDelete, "/api/v1/supply-chain/providers/1", nil},
		{http.MethodGet, "/api/v1/supply-chain/supplier-catalogs", nil},
		{http.MethodGet, "/api/v1/supply-chain/supplier-catalogs/1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "user", tt.body)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s, want 200 for a regular active user", status, raw)
			}
		})
	}
}

func TestSupplierCatalogsHaveNoWriteRoutes(t *testing.T) {
	server := newSupplyChainTestServer(t, &stubProviderService{}, &stubSupplierCatalogService{})
	tests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/v1/supply-chain/supplier-catalogs", map[string]any{"provider_id": 1}},
		{http.MethodPut, "/api/v1/supply-chain/supplier-catalogs/1", map[string]any{"lead_time": 2}},
		{http.MethodDelete, "/api/v1/supply-chain/supplier-catalogs/1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "user", tt.body)
			if status != http.StatusNotFound {
				t.Fatalf("status = %d body = %s, want 404 (no write route)", status, raw)
			}
		})
	}
}

func TestProviderListShapeDefaultsSearchAndPagination(t *testing.T) {
	service := &stubProviderService{listResult: &inbound.ProviderList{
		Items: []*domain.Provider{{
			ID:          9,
			Name:        "Distribuidora Metálica",
			ContactInfo: providerPtr("Juan"),
			Email:       providerPtr("ventas@example.com"),
			Phone:       providerPtr("300"),
		}},
		HasNext: true,
		Skip:    2,
		Limit:   1,
	}}
	server := newSupplyChainTestServer(t, service, &stubSupplierCatalogService{})

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/providers?skip=2&limit=1&search=%20met%C3%A1lica%20", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 2 || service.listFilter.Limit != 1 || service.listFilter.Search != "metálica" {
		t.Fatalf("filter = %+v, want 2/1/metálica", service.listFilter)
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
		t.Fatalf("body = %s", raw)
	}
	for _, key := range []string{"id", "name", "contact_info", "email", "phone"} {
		if _, ok := body.Items[0][key]; !ok {
			t.Errorf("item is missing %q", key)
		}
	}
	for _, key := range []string{"created_at", "updated_at", "created_by_id"} {
		if _, ok := body.Items[0][key]; ok {
			t.Errorf("item must not expose %q", key)
		}
	}

	service.listResult = nil
	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/providers", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("defaults status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 0 || service.listFilter.Limit != 100 || service.listFilter.Search != "" {
		t.Fatalf("default filter = %+v, want 0/100/empty", service.listFilter)
	}

	for _, query := range []string{"skip=-1", "limit=0", "limit=201", "limit=abc", "search=" + strings.Repeat("a", 257)} {
		t.Run("invalid "+query, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/providers?"+query, "user", nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
			if body := decodeUserError(t, raw); body.Code != "VALIDATION_ERROR" {
				t.Fatalf("code = %q", body.Code)
			}
		})
	}

	for _, search := range []string{"%", "_", "árbol", "ÁRBOL"} {
		t.Run("search "+search, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/providers?search="+url.QueryEscape(search), "user", nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			if service.listFilter.Search != search {
				t.Fatalf("search = %q, want %q", service.listFilter.Search, search)
			}
		})
	}
}

func TestProviderCRUDHandlers(t *testing.T) {
	t.Run("create trims nothing at handler and sets creator", func(t *testing.T) {
		service := &stubProviderService{}
		server := newSupplyChainTestServer(t, service, &stubSupplierCatalogService{})
		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/supply-chain/providers", "user", map[string]any{
			"name":         "  Distribuidora  ",
			"contact_info": "  Juan  ",
			"email":        nil,
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
		if service.createInput.Name != "  Distribuidora  " || service.createInput.ContactInfo == nil || *service.createInput.ContactInfo != "  Juan  " {
			t.Fatalf("create input = %+v, handler must pass raw values", service.createInput)
		}
		if service.createInput.Email != nil {
			t.Fatalf("email = %v, want nil", service.createInput.Email)
		}
		if service.createInput.CreatedByID == nil || *service.createInput.CreatedByID != 2 {
			t.Fatalf("created_by_id = %v, want token user 2", service.createInput.CreatedByID)
		}
		var provider providerJSON
		if err := json.Unmarshal(raw, &provider); err != nil {
			t.Fatal(err)
		}
		if provider.ID != 1 || provider.Name != "  Distribuidora  " {
			t.Fatalf("response = %+v", provider)
		}
	})

	t.Run("duplicate names are allowed", func(t *testing.T) {
		service := &stubProviderService{createResult: &domain.Provider{ID: 10, Name: "Same"}}
		server := newSupplyChainTestServer(t, service, &stubSupplierCatalogService{})
		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/supply-chain/providers", "user", map[string]any{"name": "Same"})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
	})

	t.Run("update maps null and omitted fields", func(t *testing.T) {
		service := &stubProviderService{}
		server := newSupplyChainTestServer(t, service, &stubSupplierCatalogService{})
		status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/supply-chain/providers/7", "user", map[string]any{
			"name":  "Updated",
			"email": nil,
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
		if service.updateID != 7 {
			t.Fatalf("id = %d, want 7", service.updateID)
		}
		if !service.updateInput.Name.Set || service.updateInput.Name.Value == nil || *service.updateInput.Name.Value != "Updated" {
			t.Fatalf("name input = %+v", service.updateInput.Name)
		}
		if !service.updateInput.Email.Set || service.updateInput.Email.Value != nil {
			t.Fatalf("email input = %+v, want explicit null", service.updateInput.Email)
		}
		if service.updateInput.Phone.Set || service.updateInput.ContactInfo.Set {
			t.Fatalf("omitted fields were set: %+v", service.updateInput)
		}

		status, raw = userRequest(t, http.MethodPut, server.URL+"/api/v1/supply-chain/providers/7", "user", map[string]any{
			"phone": "",
		})
		if status != http.StatusOK {
			t.Fatalf("empty string status = %d body = %s", status, raw)
		}
		if !service.updateInput.Phone.Set || service.updateInput.Phone.Value == nil || *service.updateInput.Phone.Value != "" {
			t.Fatalf("phone input = %+v, want explicit empty string", service.updateInput.Phone)
		}
	})

	t.Run("get and delete", func(t *testing.T) {
		service := &stubProviderService{
			getResult:    &domain.Provider{ID: 4, Name: "Loaded"},
			deleteResult: &domain.Provider{ID: 4, Name: "Deleted"},
		}
		server := newSupplyChainTestServer(t, service, &stubSupplierCatalogService{})

		status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/providers/4", "user", nil)
		if status != http.StatusOK {
			t.Fatalf("get status = %d body = %s", status, raw)
		}
		if service.getID != 4 {
			t.Fatalf("get id = %d", service.getID)
		}
		var provider providerJSON
		if err := json.Unmarshal(raw, &provider); err != nil {
			t.Fatal(err)
		}
		if provider.Name != "Loaded" {
			t.Fatalf("get response = %+v", provider)
		}

		status, raw = userRequest(t, http.MethodDelete, server.URL+"/api/v1/supply-chain/providers/4", "user", nil)
		if status != http.StatusOK || service.deleteID != 4 {
			t.Fatalf("delete status = %d id = %d body = %s", status, service.deleteID, raw)
		}
		if err := json.Unmarshal(raw, &provider); err != nil {
			t.Fatal(err)
		}
		if provider.Name != "Deleted" {
			t.Fatalf("delete response = %+v", provider)
		}
	})

	t.Run("errors", func(t *testing.T) {
		service := &stubProviderService{}
		server := newSupplyChainTestServer(t, service, &stubSupplierCatalogService{})

		service.getErr = domain.ErrNotFound
		status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/providers/9", "user", nil)
		if status != http.StatusNotFound {
			t.Fatalf("status = %d body = %s, want 404", status, raw)
		}
		if body := decodeUserError(t, raw); body.Detail != "Provider not found" || body.Code != "" {
			t.Fatalf("error = %+v", body)
		}

		service.deleteErr = domain.ErrForeignKeyViolation
		status, raw = userRequest(t, http.MethodDelete, server.URL+"/api/v1/supply-chain/providers/9", "user", nil)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d body = %s, want 400", status, raw)
		}
		if body := decodeUserError(t, raw); body.Code != "FOREIGN_KEY_VIOLATION" || body.Detail == "" {
			t.Fatalf("error = %+v", body)
		}

		service.createErr = domain.ErrValidation
		status, raw = userRequest(t, http.MethodPost, server.URL+"/api/v1/supply-chain/providers", "user", map[string]any{"name": ""})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d body = %s, want 422", status, raw)
		}
		if body := decodeUserError(t, raw); body.Code != "VALIDATION_ERROR" {
			t.Fatalf("error = %+v", body)
		}

		for _, tt := range []struct {
			method string
			path   string
			body   any
		}{
			{http.MethodGet, "/api/v1/supply-chain/providers/0", nil},
			{http.MethodPut, "/api/v1/supply-chain/providers/abc", map[string]any{}},
			{http.MethodPost, "/api/v1/supply-chain/providers", map[string]any{"unknown": 1}},
			{http.MethodPut, "/api/v1/supply-chain/providers/1", map[string]any{"unknown": 1}},
		} {
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "user", tt.body)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("%s %s status = %d body = %s, want 422", tt.method, tt.path, status, raw)
			}
			if body := decodeUserError(t, raw); body.Code != "VALIDATION_ERROR" {
				t.Fatalf("%s %s error = %+v", tt.method, tt.path, body)
			}
		}
	})
}

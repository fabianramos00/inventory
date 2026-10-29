package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type stubClientService struct {
	listResult *inbound.ClientList
	listErr    error
	listFilter inbound.ClientFilter

	getResult *domain.Client
	getErr    error
	getID     int64

	createResult *domain.Client
	createErr    error
	createInput  inbound.CreateClientInput

	updateResult *domain.Client
	updateErr    error
	updateID     int64
	updateInput  inbound.UpdateClientInput

	deleteResult *domain.Client
	deleteErr    error
	deleteID     int64
}

func (s *stubClientService) List(_ context.Context, filter inbound.ClientFilter) (*inbound.ClientList, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.ClientList{Skip: filter.Skip, Limit: filter.Limit}, nil
	}
	return s.listResult, nil
}

func (s *stubClientService) Get(_ context.Context, id int64) (*domain.Client, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getResult == nil {
		return &domain.Client{ID: id, Name: "Client", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	}
	return s.getResult, nil
}

func (s *stubClientService) Create(_ context.Context, input inbound.CreateClientInput) (*domain.Client, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult == nil {
		return &domain.Client{ID: 1, Name: input.Name, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	}
	return s.createResult, nil
}

func (s *stubClientService) Update(_ context.Context, id int64, input inbound.UpdateClientInput) (*domain.Client, error) {
	s.updateID, s.updateInput = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateResult == nil {
		return &domain.Client{ID: id, Name: "Client", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	}
	return s.updateResult, nil
}

func (s *stubClientService) Delete(_ context.Context, id int64) (*domain.Client, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if s.deleteResult == nil {
		return &domain.Client{ID: id, Name: "Client", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	}
	return s.deleteResult, nil
}

func newClientTestServer(t *testing.T, service *stubClientService) *httptest.Server {
	t.Helper()
	if service == nil {
		service = &stubClientService{}
	}
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:    auth,
		Users:   &stubUserService{},
		Clients: service,
		Logger:  logger,
	}))
	t.Cleanup(server.Close)
	return server
}

type clientJSON struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	IdentityCard *string `json:"identity_card"`
	Email        *string `json:"email"`
	Phone        *string `json:"phone"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    *string `json:"updated_at"`
}

func clientPtr(value string) *string {
	return &value
}

func TestClientsRequireAuth(t *testing.T) {
	server := newClientTestServer(t, nil)
	tests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/sales/clients", nil},
		{http.MethodPost, "/api/v1/sales/clients", map[string]any{"name": "Ana"}},
		{http.MethodGet, "/api/v1/sales/clients/1", nil},
		{http.MethodPut, "/api/v1/sales/clients/1", map[string]any{"name": "Ana"}},
		{http.MethodDelete, "/api/v1/sales/clients/1", nil},
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

func TestClientListShapeDefaultsAndPagination(t *testing.T) {
	service := &stubClientService{listResult: &inbound.ClientList{
		Items: []*domain.Client{{
			ID:           9,
			Name:         "Ana",
			IdentityCard: clientPtr("123"),
			Email:        clientPtr("ana@example.com"),
			Phone:        clientPtr("555"),
			CreatedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}},
		HasNext: true,
		Skip:    2,
		Limit:   1,
	}}
	server := newClientTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/clients?skip=2&limit=1&search=%20ana%20", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 2 || service.listFilter.Limit != 1 || service.listFilter.Search != "ana" {
		t.Fatalf("filter = %+v, want 2/1/ana", service.listFilter)
	}
	var body struct {
		Items   []clientJSON `json:"items"`
		HasNext bool         `json:"has_next"`
		Skip    int          `json:"skip"`
		Limit   int          `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != 9 || body.Items[0].Name != "Ana" ||
		body.Items[0].IdentityCard == nil || *body.Items[0].IdentityCard != "123" ||
		body.Items[0].Email == nil || *body.Items[0].Email != "ana@example.com" ||
		body.Items[0].Phone == nil || *body.Items[0].Phone != "555" {
		t.Fatalf("items = %+v", body.Items)
	}
	if body.Items[0].CreatedAt != "2026-01-02T03:04:05Z" || body.Items[0].UpdatedAt != nil {
		t.Fatalf("timestamps = %q/%v", body.Items[0].CreatedAt, body.Items[0].UpdatedAt)
	}
	if !body.HasNext || body.Skip != 2 || body.Limit != 1 {
		t.Fatalf("pagination = %v/%d/%d", body.HasNext, body.Skip, body.Limit)
	}

	// Every item carries the documented keys, including nullable ones.
	var itemFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &itemFields); err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(itemFields["items"], &items); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "name", "identity_card", "email", "phone", "created_at", "updated_at"} {
		if _, ok := items[0][key]; !ok {
			t.Errorf("item is missing %q", key)
		}
	}

	service.listResult = nil
	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/clients", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("defaults status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 0 || service.listFilter.Limit != 100 || service.listFilter.Search != "" {
		t.Fatalf("default filter = %+v, want 0/100/empty", service.listFilter)
	}

	for _, query := range []string{"skip=-1", "limit=0", "limit=201", "limit=abc", "search=" + strings.Repeat("a", 257)} {
		t.Run("invalid "+query, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/clients?"+query, "user", nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
			if body := decodeUserError(t, raw); body.Code != "VALIDATION_ERROR" {
				t.Fatalf("code = %q", body.Code)
			}
		})
	}
}

func TestClientCRUDHandlers(t *testing.T) {
	t.Run("create returns full shape", func(t *testing.T) {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		service := &stubClientService{createResult: &domain.Client{
			ID:           11,
			Name:         "Ana",
			IdentityCard: clientPtr("123"),
			Email:        clientPtr("ana@example.com"),
			CreatedAt:    created,
		}}
		server := newClientTestServer(t, service)

		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/sales/clients", "user", map[string]any{
			"name":          "  Ana  ",
			"identity_card": " 123 ",
			"email":         " ana@example.com ",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
		if service.createInput.Name != "  Ana  " || service.createInput.IdentityCard == nil || *service.createInput.IdentityCard != " 123 " {
			t.Fatalf("create input = %+v, handler must pass raw values to the service", service.createInput)
		}
		var client clientJSON
		if err := json.Unmarshal(raw, &client); err != nil {
			t.Fatal(err)
		}
		if client.ID != 11 || client.Name != "Ana" || client.IdentityCard == nil || *client.IdentityCard != "123" ||
			client.Email == nil || *client.Email != "ana@example.com" ||
			client.CreatedAt != "2026-01-02T03:04:05Z" || client.UpdatedAt != nil {
			t.Fatalf("response = %+v", client)
		}
	})

	t.Run("get", func(t *testing.T) {
		service := &stubClientService{getResult: &domain.Client{
			ID:        5,
			Name:      "Ana",
			CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}}
		server := newClientTestServer(t, service)
		status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/clients/5", "user", nil)
		if status != http.StatusOK || service.getID != 5 {
			t.Fatalf("status = %d id = %d body = %s", status, service.getID, raw)
		}
		var client clientJSON
		if err := json.Unmarshal(raw, &client); err != nil {
			t.Fatal(err)
		}
		if client.ID != 5 || client.Name != "Ana" {
			t.Fatalf("response = %+v", client)
		}
	})

	t.Run("update null versus omitted", func(t *testing.T) {
		updated := time.Date(2026, 1, 2, 4, 4, 5, 0, time.UTC)
		service := &stubClientService{updateResult: &domain.Client{
			ID:        5,
			Name:      "Ana",
			Phone:     clientPtr("777"),
			CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			UpdatedAt: &updated,
		}}
		server := newClientTestServer(t, service)

		status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/sales/clients/5", "user", map[string]any{
			"email": nil,
			"phone": " 777 ",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
		input := service.updateInput
		if input.Name.Set || input.IdentityCard.Set {
			t.Fatalf("input flags = name:%v identity:%v, want false/false", input.Name.Set, input.IdentityCard.Set)
		}
		if !input.Email.Set || input.Email.Value != nil {
			t.Fatalf("email = %+v, want explicit null", input.Email)
		}
		if !input.Phone.Set || input.Phone.Value == nil || *input.Phone.Value != " 777 " {
			t.Fatalf("phone = %+v, want raw 777", input.Phone)
		}

		var client clientJSON
		if err := json.Unmarshal(raw, &client); err != nil {
			t.Fatal(err)
		}
		if client.UpdatedAt == nil || *client.UpdatedAt != "2026-01-02T04:04:05Z" {
			t.Fatalf("updated_at = %v", client.UpdatedAt)
		}
	})

	t.Run("delete returns client", func(t *testing.T) {
		service := &stubClientService{deleteResult: &domain.Client{
			ID:        5,
			Name:      "Ana",
			CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}}
		server := newClientTestServer(t, service)
		status, raw := userRequest(t, http.MethodDelete, server.URL+"/api/v1/sales/clients/5", "user", nil)
		if status != http.StatusOK || service.deleteID != 5 {
			t.Fatalf("status = %d id = %d body = %s", status, service.deleteID, raw)
		}
		var client clientJSON
		if err := json.Unmarshal(raw, &client); err != nil {
			t.Fatal(err)
		}
		if client.ID != 5 {
			t.Fatalf("response = %+v", client)
		}
	})

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run("invalid id "+method, func(t *testing.T) {
			server := newClientTestServer(t, nil)
			status, raw := userRequest(t, method, server.URL+"/api/v1/sales/clients/not-a-number", "user", map[string]any{"name": "Ana"})
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
		})
	}
}

func TestClientErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		service    *stubClientService
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{
			name:       "duplicate identity card",
			method:     http.MethodPost,
			path:       "/api/v1/sales/clients",
			body:       map[string]any{"name": "Ana"},
			service:    &stubClientService{createErr: domain.ErrConflict},
			wantStatus: http.StatusBadRequest,
			wantCode:   "UNIQUE_VIOLATION",
		},
		{
			name:       "missing client",
			method:     http.MethodGet,
			path:       "/api/v1/sales/clients/99",
			service:    &stubClientService{getErr: domain.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantDetail: "Client not found",
		},
		{
			name:       "referenced client",
			method:     http.MethodDelete,
			path:       "/api/v1/sales/clients/99",
			service:    &stubClientService{deleteErr: domain.ErrForeignKeyViolation},
			wantStatus: http.StatusBadRequest,
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "validation",
			method:     http.MethodPut,
			path:       "/api/v1/sales/clients/99",
			body:       map[string]any{"name": ""},
			service:    &stubClientService{updateErr: domain.ErrValidation},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newClientTestServer(t, tt.service)
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

func TestClientRejectsUnknownFieldsAndMalformedBody(t *testing.T) {
	server := newClientTestServer(t, nil)
	for _, raw := range []string{`{"name":"Ana","bogus":true}`, `{"name":`} {
		t.Run(raw, func(t *testing.T) {
			status, body := rawClientRequest(t, http.MethodPost, server.URL+"/api/v1/sales/clients", "user", raw)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, body)
			}
			if decoded := decodeUserError(t, body); decoded.Code != "VALIDATION_ERROR" {
				t.Fatalf("code = %q", decoded.Code)
			}
		})
	}
}

func rawClientRequest(t *testing.T, method, endpoint, token, raw string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}

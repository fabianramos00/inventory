package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type userJSON struct {
	ID          int64  `json:"id"`
	FullName    string `json:"full_name"`
	Email       string `json:"email"`
	IsActive    bool   `json:"is_active"`
	IsSuperuser bool   `json:"is_superuser"`
}

type userListJSON struct {
	Items   []userJSON `json:"items"`
	HasNext bool       `json:"has_next"`
	Skip    int        `json:"skip"`
	Limit   int        `json:"limit"`
}

type usersErrorJSON struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

// stubUserService records the inbound call and returns configured values.
type stubUserService struct {
	listResult  *inbound.UserList
	listErr     error
	listSkip    int
	listLimit   int
	listSearch  string
	getID       int64
	getResult   *domain.User
	getErr      error
	createInput inbound.CreateUserInput
	createErr   error
	updateID    int64
	updateInput inbound.UpdateUserInput
	updateErr   error
	deleteID    int64
	deleteErr   error
}

func (s *stubUserService) List(_ context.Context, skip, limit int, search string) (*inbound.UserList, error) {
	s.listSkip, s.listLimit, s.listSearch = skip, limit, search
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.UserList{Skip: skip, Limit: limit}, nil
	}
	return s.listResult, nil
}

func (s *stubUserService) Get(_ context.Context, id int64) (*domain.User, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getResult, nil
}

func (s *stubUserService) Create(_ context.Context, input inbound.CreateUserInput) (*domain.User, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &domain.User{
		ID:          10,
		Email:       input.Email,
		FullName:    input.FullName,
		IsActive:    input.IsActive == nil || *input.IsActive,
		IsSuperuser: input.IsSuperuser != nil && *input.IsSuperuser,
	}, nil
}

func (s *stubUserService) Update(_ context.Context, id int64, input inbound.UpdateUserInput) (*domain.User, error) {
	s.updateID, s.updateInput = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &domain.User{ID: id, Email: "updated@example.com", FullName: "Updated", IsActive: true}, nil
}

func (s *stubUserService) Delete(_ context.Context, id int64) (*domain.User, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	return &domain.User{ID: id, Email: "deleted@example.com", FullName: "Deleted", IsActive: true}, nil
}

type stubAuthService struct {
	tokens        map[string]*domain.User
	refreshResult *inbound.TokenPair
	refreshErr    error
}

func (s *stubAuthService) Login(context.Context, string, string) (*inbound.LoginResult, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAuthService) Refresh(context.Context, string) (*inbound.TokenPair, error) {
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	if s.refreshResult == nil {
		return nil, errors.New("not implemented")
	}
	return s.refreshResult, nil
}

func (s *stubAuthService) Logout(context.Context, string) error {
	return errors.New("not implemented")
}

func (s *stubAuthService) AuthenticateAccessToken(_ context.Context, rawToken string) (*domain.User, error) {
	user, ok := s.tokens[rawToken]
	if !ok {
		return nil, domain.ErrInvalidAccess
	}
	return user, nil
}

func newUsersTestServer(t *testing.T, service inbound.UserUseCase) *httptest.Server {
	t.Helper()
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"super":   {ID: 1, FullName: "Admin User", Email: "admin@example.com", IsActive: true, IsSuperuser: true},
		"regular": {ID: 2, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{Auth: auth, Users: service, Logger: logger}))
	t.Cleanup(server.Close)
	return server
}

func userRequest(t *testing.T, method, endpoint, token string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func decodeUserError(t *testing.T, raw []byte) usersErrorJSON {
	t.Helper()
	var body usersErrorJSON
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestUsersRequireSuperuser(t *testing.T) {
	service := &stubUserService{}
	server := newUsersTestServer(t, service)

	tests := []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodGet, path: "/api/v1/users"},
		{method: http.MethodPost, path: "/api/v1/users", body: map[string]any{"email": "new@example.com", "password": "password123"}},
		{method: http.MethodGet, path: "/api/v1/users/7"},
		{method: http.MethodPut, path: "/api/v1/users/7", body: map[string]any{"full_name": "Updated"}},
		{method: http.MethodDelete, path: "/api/v1/users/7"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "regular", tt.body)
			if status != http.StatusForbidden {
				t.Fatalf("status = %d body = %s, want 403", status, raw)
			}
			body := decodeUserError(t, raw)
			if body.Detail != "The user doesn't have enough privileges" {
				t.Errorf("detail = %q", body.Detail)
			}
		})
	}
}

func TestUsersSuperuserList(t *testing.T) {
	service := &stubUserService{listResult: &inbound.UserList{
		Items: []*domain.User{
			{ID: 3, FullName: "Ana Torres", Email: "ana@example.com", IsActive: true},
			{ID: 4, FullName: "Ana Pérez", Email: "ana.perez@example.com", IsActive: false},
		},
		HasNext: true,
		Skip:    10,
		Limit:   1,
	}}
	server := newUsersTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/users?skip=10&limit=1&search=%20ana%20", "super", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	var body userListJSON
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(body.Items))
	}
	if !body.HasNext || body.Skip != 10 || body.Limit != 1 {
		t.Errorf("pagination = has_next:%v skip:%d limit:%d, want true/10/1", body.HasNext, body.Skip, body.Limit)
	}
	if body.Items[0].ID != 3 || body.Items[0].FullName != "Ana Torres" || !body.Items[0].IsActive {
		t.Errorf("first item = %+v", body.Items[0])
	}
	if body.Items[1].IsActive {
		t.Errorf("second item is_active = true, want false")
	}
	if service.listSkip != 10 || service.listLimit != 1 || service.listSearch != "ana" {
		t.Errorf("service received skip=%d limit=%d search=%q, want 10/1/ana", service.listSkip, service.listLimit, service.listSearch)
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

func TestUsersSuperuserCRUD(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		service := &stubUserService{}
		server := newUsersTestServer(t, service)

		status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/users", "super", map[string]any{
			"email":        "new@example.com",
			"password":     "password123",
			"full_name":    "New User",
			"is_active":    false,
			"is_superuser": true,
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body userJSON
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 10 || body.Email != "new@example.com" || body.FullName != "New User" {
			t.Errorf("response = %+v", body)
		}
		if body.IsActive {
			t.Error("response is_active = true, want false")
		}
		if !body.IsSuperuser {
			t.Error("response is_superuser = false, want true")
		}
		if service.createInput.Email != "new@example.com" || service.createInput.Password != "password123" ||
			service.createInput.FullName != "New User" {
			t.Errorf("service input = %+v", service.createInput)
		}
		if service.createInput.IsActive == nil || *service.createInput.IsActive {
			t.Errorf("service is_active = %v, want explicit false", service.createInput.IsActive)
		}
		if service.createInput.IsSuperuser == nil || !*service.createInput.IsSuperuser {
			t.Errorf("service is_superuser = %v, want explicit true", service.createInput.IsSuperuser)
		}
	})

	t.Run("get", func(t *testing.T) {
		service := &stubUserService{getResult: &domain.User{ID: 7, FullName: "Ana", Email: "ana@example.com", IsActive: true}}
		server := newUsersTestServer(t, service)

		status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/users/7", "super", nil)
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body userJSON
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 7 || body.FullName != "Ana" || body.Email != "ana@example.com" {
			t.Errorf("response = %+v", body)
		}
		if service.getID != 7 {
			t.Errorf("service id = %d, want 7", service.getID)
		}
	})

	t.Run("update", func(t *testing.T) {
		service := &stubUserService{}
		server := newUsersTestServer(t, service)

		status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/users/7", "super", map[string]any{
			"full_name": "Updated",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body userJSON
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 7 || body.FullName != "Updated" {
			t.Errorf("response = %+v", body)
		}
		if service.updateID != 7 {
			t.Errorf("service id = %d, want 7", service.updateID)
		}
		if service.updateInput.FullName == nil || *service.updateInput.FullName != "Updated" {
			t.Errorf("service full_name = %v, want Updated", service.updateInput.FullName)
		}
		if service.updateInput.Email != nil || service.updateInput.Password != nil ||
			service.updateInput.IsActive != nil || service.updateInput.IsSuperuser != nil {
			t.Errorf("partial update leaked fields: %+v", service.updateInput)
		}
	})

	t.Run("delete", func(t *testing.T) {
		service := &stubUserService{}
		server := newUsersTestServer(t, service)

		status, raw := userRequest(t, http.MethodDelete, server.URL+"/api/v1/users/7", "super", nil)
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s, want 200", status, raw)
		}
		var body userJSON
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 7 {
			t.Errorf("response id = %d, want 7", body.ID)
		}
		if service.deleteID != 7 {
			t.Errorf("service id = %d, want 7", service.deleteID)
		}
	})
}

func TestUsersErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		service    *stubUserService
		wantStatus int
		wantDetail string
		wantCode   string
	}{
		{
			name:       "duplicate email",
			method:     http.MethodPost,
			path:       "/api/v1/users",
			body:       map[string]any{"email": "taken@example.com", "password": "password123"},
			service:    &stubUserService{createErr: domain.ErrConflict},
			wantStatus: http.StatusBadRequest,
			wantDetail: "The user with this username already exists in the system.",
		},
		{
			name:       "missing user",
			method:     http.MethodGet,
			path:       "/api/v1/users/99",
			service:    &stubUserService{getErr: domain.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantDetail: "User not found",
		},
		{
			name:       "update missing user",
			method:     http.MethodPut,
			path:       "/api/v1/users/99",
			body:       map[string]any{"full_name": "Ghost"},
			service:    &stubUserService{updateErr: domain.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantDetail: "User not found",
		},
		{
			name:       "delete missing user",
			method:     http.MethodDelete,
			path:       "/api/v1/users/99",
			service:    &stubUserService{deleteErr: domain.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantDetail: "User not found",
		},
		{
			name:       "delete foreign key violation",
			method:     http.MethodDelete,
			path:       "/api/v1/users/7",
			service:    &stubUserService{deleteErr: domain.ErrForeignKeyViolation},
			wantStatus: http.StatusBadRequest,
			wantDetail: "The user cannot be deleted because it is referenced by other records.",
			wantCode:   "FOREIGN_KEY_VIOLATION",
		},
		{
			name:       "invalid password",
			method:     http.MethodPost,
			path:       "/api/v1/users",
			body:       map[string]any{"email": "new@example.com", "password": "short"},
			service:    &stubUserService{createErr: fmt.Errorf("%w: password must be 8-128 characters", domain.ErrValidation)},
			wantStatus: http.StatusUnprocessableEntity,
			wantDetail: "password must be 8-128 characters",
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "invalid id",
			method:     http.MethodGet,
			path:       "/api/v1/users/not-a-number",
			service:    &stubUserService{},
			wantStatus: http.StatusUnprocessableEntity,
			wantDetail: "Invalid user id",
			wantCode:   "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newUsersTestServer(t, tt.service)
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "super", tt.body)
			if status != tt.wantStatus {
				t.Fatalf("status = %d body = %s, want %d", status, raw, tt.wantStatus)
			}
			body := decodeUserError(t, raw)
			if body.Detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", body.Detail, tt.wantDetail)
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
		})
	}
}

func TestRefreshResponseOmitsUser(t *testing.T) {
	auth := &stubAuthService{
		tokens: map[string]*domain.User{},
		refreshResult: &inbound.TokenPair{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{Auth: auth, Users: &stubUserService{}, Logger: logger}))
	t.Cleanup(server.Close)

	status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": "old-refresh",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"access_token", "refresh_token", "token_type"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("response is missing %q", key)
		}
	}
	if _, ok := fields["user"]; ok {
		t.Errorf("refresh response must not include %q: %s", "user", raw)
	}
	if len(fields) != 3 {
		t.Errorf("response has %d fields, want exactly 3: %s", len(fields), raw)
	}
}

func TestUsersRequestBodyErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"malformed json", `{"email":`, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"oversized body", strings.Repeat(" ", (10<<20)+1), http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE"},
	}

	server := newUsersTestServer(t, &stubUserService{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/users", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer super")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			raw, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d body = %s, want %d", response.StatusCode, raw, tt.wantStatus)
			}
			body := decodeUserError(t, raw)
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
		})
	}
}

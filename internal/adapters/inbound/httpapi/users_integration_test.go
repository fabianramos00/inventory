package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
)

func TestUsersOptionsIntegration(t *testing.T) {
	for _, mode := range []string{"sqlite", "postgres"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			gormDB, _ := openTestDB(t, mode)

			users := gormstore.NewUserStore(gormDB)
			passwords := security.NewPasswordHasher(4)
			hash, err := passwords.Hash("password123")
			if err != nil {
				t.Fatal(err)
			}

			user := func(fullName, email string, active bool) *domain.User {
				created := &domain.User{FullName: fullName, Email: email, HashedPassword: hash, IsActive: active}
				if err := users.Create(ctx, created); err != nil {
					t.Fatal(err)
				}
				return created
			}
			regular := user("Regular User", "regular@example.com", true)
			_ = user("Picked Alpha", "picked-alpha@example.com", true)
			_ = user("Picked Beta", "picked-beta@example.com", true)
			_ = user("Picked Gamma", "picked-gamma@example.com", true)
			_ = user("Picked Inactive", "picked-inactive@example.com", false)
			_ = user("Hidden Delta", "hidden@example.com", true)

			tokens := gormstore.NewRefreshTokenStore(gormDB)
			issuer := security.NewAccessTokenIssuer("test-secret", 30*time.Minute, "inventory-desktop/"+mode)
			authService := application.NewAuthService(
				users,
				tokens,
				passwords,
				issuer,
				security.NewRefreshTokenService(),
				7*24*time.Hour,
			)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
				Auth:   authService,
				Users:  application.NewUserService(users, tokens, passwords),
				Logger: logger,
			}))
			t.Cleanup(server.Close)

			accessToken, err := issuer.Issue(regular.ID)
			if err != nil {
				t.Fatal(err)
			}

			status, raw := doRequest(t, server, http.MethodGet, "/api/v1/users/options?search=picked&limit=2", nil, &accessToken)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s, want 200", status, raw)
			}
			var page struct {
				Items   []map[string]json.RawMessage `json:"items"`
				HasNext bool                         `json:"has_next"`
				Skip    int                          `json:"skip"`
				Limit   int                          `json:"limit"`
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				t.Fatal(err)
			}
			if !page.HasNext || page.Limit != 2 || len(page.Items) != 2 {
				t.Fatalf("first page = %d items has_next=%v limit=%d, want 2/true/2", len(page.Items), page.HasNext, page.Limit)
			}
			for _, item := range page.Items {
				if len(item) != 2 {
					t.Errorf("option keys = %v, want only id and full_name", item)
				}
				if string(item["full_name"]) == `"Picked Inactive"` {
					t.Error("options include an inactive user")
				}
			}

			status, raw = doRequest(t, server, http.MethodGet, "/api/v1/users/options?search=picked&skip=2&limit=2", nil, &accessToken)
			if status != http.StatusOK {
				t.Fatalf("second page status = %d body = %s, want 200", status, raw)
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				t.Fatal(err)
			}
			if page.HasNext || len(page.Items) != 1 {
				t.Fatalf("second page = %d items has_next=%v, want 1 and false", len(page.Items), page.HasNext)
			}

			status, raw = doRequest(t, server, http.MethodGet, "/api/v1/users/options?search=hidden@example.com", nil, &accessToken)
			if status != http.StatusOK {
				t.Fatalf("email search status = %d body = %s, want 200", status, raw)
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				t.Fatal(err)
			}
			if page.HasNext || len(page.Items) != 0 {
				t.Fatalf("email search = %d items has_next=%v, want 0 and false", len(page.Items), page.HasNext)
			}

			status, raw = doRequest(t, server, http.MethodGet, "/api/v1/users/options", nil, nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("no token status = %d body = %s, want 401", status, raw)
			}

			status, raw = doRequest(t, server, http.MethodGet, "/api/v1/users", nil, &accessToken)
			if status != http.StatusForbidden {
				t.Fatalf("admin list status = %d body = %s, want 403", status, raw)
			}
		})
	}
}

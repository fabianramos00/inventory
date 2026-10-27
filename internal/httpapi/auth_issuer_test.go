package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	sqlitestore "inventory-desktop/internal/adapters/persistence/sqlite"
	"inventory-desktop/internal/auth"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/httpapi"
	"inventory-desktop/internal/service"
)

// TestAccessTokensAreBoundToBackendIssuer proves that an access token minted
// by one backend's service is rejected by a service configured for the other
// backend. Both services share one temporary SQLite store; only the issuer
// differs, so a rejection cannot be explained by a missing user row.
func TestAccessTokensAreBoundToBackendIssuer(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "issuer.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}

	users := sqlitestore.NewUserRepository(conn)
	tokens := sqlitestore.NewRefreshTokenRepository(conn)
	password := "password123"
	hash, err := auth.HashPassword(password, 4)
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{FullName: "Issuer User", Email: "issuer@example.com", HashedPassword: hash, IsActive: true}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	const secret = "shared-test-secret"
	sqliteService := service.NewAuthService(users, tokens, secret, "sqlite", 30*time.Minute, 24*time.Hour)
	postgresService := service.NewAuthService(users, tokens, secret, "postgres", 30*time.Minute, 24*time.Hour)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sqliteServer := httptest.NewServer(httpapi.NewRouterWithBackend(sqliteService, logger, "sqlite"))
	defer sqliteServer.Close()
	postgresServer := httptest.NewServer(httpapi.NewRouterWithBackend(postgresService, logger, "postgres"))
	defer postgresServer.Close()

	sqliteLogin, err := sqliteService.Login(ctx, user.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	postgresLogin, err := postgresService.Login(ctx, user.Email, password)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("sqlite token rejected by postgres service", func(t *testing.T) {
		assertProtectedStatus(t, postgresServer.URL, sqliteLogin.AccessToken, http.StatusUnauthorized)
	})
	t.Run("postgres token rejected by sqlite service", func(t *testing.T) {
		assertProtectedStatus(t, sqliteServer.URL, postgresLogin.AccessToken, http.StatusUnauthorized)
	})
	t.Run("own tokens still authenticate", func(t *testing.T) {
		// /api/v1/users is not implemented yet; reaching 404 proves the auth
		// middleware accepted the token instead of rejecting it with 401.
		assertProtectedStatus(t, sqliteServer.URL, sqliteLogin.AccessToken, http.StatusNotFound)
		assertProtectedStatus(t, postgresServer.URL, postgresLogin.AccessToken, http.StatusNotFound)
	})
}

func assertProtectedStatus(t *testing.T, baseURL, token string, want int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/users", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d body = %s, want %d", response.StatusCode, body, want)
	}
}

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

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
)

// TestAccessTokensAreBoundToBackendIssuer proves that an access token minted
// by one backend's service is rejected by a service configured for the other
// backend. Both services share one temporary SQLite store; only the issuer
// differs, so a rejection cannot be explained by a missing user row.
func TestAccessTokensAreBoundToBackendIssuer(t *testing.T) {
	ctx := context.Background()
	gormDB, conn, err := db.OpenSQLiteGORM(filepath.Join(t.TempDir(), "issuer.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}

	users := gormstore.NewUserStore(gormDB)
	tokens := gormstore.NewRefreshTokenStore(gormDB)
	password := "password123"
	passwords := security.NewPasswordHasher(4)
	hash, err := passwords.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{FullName: "Issuer User", Email: "issuer@example.com", HashedPassword: hash, IsActive: true}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	const secret = "shared-test-secret"
	refreshTokens := security.NewRefreshTokenService()
	sqliteService := application.NewAuthService(
		users, tokens, passwords,
		security.NewAccessTokenIssuer(secret, 30*time.Minute, "inventory-desktop/sqlite"),
		refreshTokens, 24*time.Hour,
	)
	postgresService := application.NewAuthService(
		users, tokens, passwords,
		security.NewAccessTokenIssuer(secret, 30*time.Minute, "inventory-desktop/postgres"),
		refreshTokens, 24*time.Hour,
	)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	userService := application.NewUserService(users, passwords)
	sqliteServer := httptest.NewServer(httpapi.NewRouterWithBackend(sqliteService, userService, logger, "sqlite"))
	defer sqliteServer.Close()
	postgresServer := httptest.NewServer(httpapi.NewRouterWithBackend(postgresService, userService, logger, "postgres"))
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
		// The test user is not a superuser, so reaching 403 proves the auth
		// middleware accepted the token instead of rejecting it with 401.
		assertProtectedStatus(t, sqliteServer.URL, sqliteLogin.AccessToken, http.StatusForbidden)
		assertProtectedStatus(t, postgresServer.URL, postgresLogin.AccessToken, http.StatusForbidden)
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

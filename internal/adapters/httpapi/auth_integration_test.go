package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"inventory-desktop/internal/adapters/httpapi"
	"inventory-desktop/internal/adapters/persistence/postgres"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/auth"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/tests/testdb"
)

type authResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	User         *domain.User `json:"user"`
}

type errorResponse struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func TestAuthFlow(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the auth integration test")
	}

	ctx := context.Background()
	// The contract suite truncates the same test database; serialize the two
	// test binaries so neither deletes the other's fixture rows mid-test.
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := db.MigratePostgres(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := db.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "TRUNCATE refresh_tokens, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}

	users := postgres.NewUserRepository(pool)
	hash, err := auth.HashPassword("admin12345", 4)
	if err != nil {
		t.Fatal(err)
	}
	admin := &domain.User{FullName: "Admin User", Email: "admin@example.com", HashedPassword: hash, IsActive: true, IsSuperuser: true}
	if err := users.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	inactiveHash, err := auth.HashPassword("inactive123", 4)
	if err != nil {
		t.Fatal(err)
	}
	inactive := &domain.User{FullName: "Inactive User", Email: "inactive@example.com", HashedPassword: inactiveHash, IsActive: false}
	if err := users.Create(ctx, inactive); err != nil {
		t.Fatal(err)
	}

	authService := application.NewAuthService(users, postgres.NewRefreshTokenRepository(pool), "test-secret", "postgres", 30*time.Minute, 7*24*time.Hour)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(authService, logger, true))
	defer server.Close()

	t.Run("login", func(t *testing.T) {
		response, header := login(t, server.URL, "admin@example.com", "admin12345")
		assertNoStore(t, header)
		if response.AccessToken == "" || response.RefreshToken == "" {
			t.Fatal("expected access and refresh tokens")
		}
		if response.TokenType != "bearer" {
			t.Fatalf("token_type = %q, want bearer", response.TokenType)
		}
		if response.User == nil || response.User.Email != "admin@example.com" || !response.User.IsSuperuser {
			t.Fatalf("unexpected user: %+v", response.User)
		}
		if response.User.HashedPassword != "" {
			t.Fatal("hashed password must never be serialized")
		}
	})

	t.Run("invalid credentials and inactive user", func(t *testing.T) {
		status, body := postForm(t, server.URL+"/api/v1/auth/login", "admin@example.com", "wrong-password")
		if status != http.StatusUnauthorized || body.Detail != "Incorrect email or password" {
			t.Fatalf("got status %d detail %q", status, body.Detail)
		}
		status, body = postForm(t, server.URL+"/api/v1/auth/login", "inactive@example.com", "inactive123")
		if status != http.StatusUnauthorized || body.Detail != "Inactive user" {
			t.Fatalf("got status %d detail %q", status, body.Detail)
		}
	})

	var loginResponse authResponse
	t.Run("login session", func(t *testing.T) {
		loginResponse, _ = login(t, server.URL, "admin@example.com", "admin12345")
	})

	t.Run("refresh rotates and rejects old token", func(t *testing.T) {
		first, header := refresh(t, server.URL, loginResponse.RefreshToken)
		assertNoStore(t, header)
		if first.RefreshToken == loginResponse.RefreshToken {
			t.Fatal("refresh token was not rotated")
		}
		status, body, _ := postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": loginResponse.RefreshToken})
		if status != http.StatusUnauthorized || body.Detail != "Invalid or expired refresh token" {
			t.Fatalf("got status %d detail %q", status, body.Detail)
		}
		second, _ := refresh(t, server.URL, first.RefreshToken)
		if second.RefreshToken == first.RefreshToken {
			t.Fatal("second refresh token was not rotated")
		}
		loginResponse.RefreshToken = second.RefreshToken
	})

	t.Run("logout revokes token", func(t *testing.T) {
		status, body, header := postJSON(t, server.URL+"/api/v1/auth/logout", map[string]string{"refresh_token": loginResponse.RefreshToken})
		assertNoStore(t, header)
		if status != http.StatusOK || body.Detail != "Logged out" {
			t.Fatalf("got status %d detail %q", status, body.Detail)
		}
		status, body, _ = postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": loginResponse.RefreshToken})
		if status != http.StatusUnauthorized || body.Detail != "Invalid or expired refresh token" {
			t.Fatalf("got status %d detail %q", status, body.Detail)
		}
	})

	t.Run("protected route requires bearer token", func(t *testing.T) {
		response, err := http.Get(server.URL + "/api/v1/users")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var body errorResponse
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusUnauthorized || body.Detail != "Not authenticated" {
			t.Fatalf("got status %d detail %q", response.StatusCode, body.Detail)
		}

		request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/users", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+loginResponse.AccessToken)
		response, err = http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("got status %d, want 404 for an unimplemented protected route", response.StatusCode)
		}
	})

	t.Run("cors allows only configured origins", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodOptions, server.URL+"/api/v1/auth/login", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "http://localhost:5173")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", response.StatusCode)
		}
		if got := response.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Fatalf("allow origin = %q", got)
		}

		request, err = http.NewRequest(http.MethodOptions, server.URL+"/api/v1/auth/login", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "http://evil.example")
		response, err = http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if got := response.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("unexpected allow origin %q", got)
		}
	})

	t.Run("dev origins are rejected when dev mode is off", func(t *testing.T) {
		strict := httptest.NewServer(httpapi.NewRouter(authService, logger))
		defer strict.Close()

		originHeader := func(origin string) http.Header {
			request, err := http.NewRequest(http.MethodOptions, strict.URL+"/api/v1/auth/login", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", origin)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", response.StatusCode)
			}
			return response.Header
		}

		if got := originHeader("http://localhost:5173").Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("dev origin allowed with dev mode off: %q", got)
		}
		if got := originHeader("wails://wails").Get("Access-Control-Allow-Origin"); got != "wails://wails" {
			t.Fatalf("wails origin not allowed with dev mode off: %q", got)
		}
	})
}

func assertNoStore(t *testing.T, header http.Header) {
	t.Helper()
	if got := header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := header.Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
}

func login(t *testing.T, baseURL, email, password string) (authResponse, http.Header) {
	t.Helper()
	status, raw, header := postFormRaw(t, baseURL+"/api/v1/auth/login", email, password)
	if status != http.StatusOK {
		t.Fatalf("login status = %d body = %s", status, raw)
	}
	var response authResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	return response, header
}

func refresh(t *testing.T, baseURL, token string) (authResponse, http.Header) {
	t.Helper()
	status, raw, header := postJSONRaw(t, baseURL+"/api/v1/auth/refresh", map[string]string{"refresh_token": token})
	if status != http.StatusOK {
		t.Fatalf("refresh status = %d body = %s", status, raw)
	}
	var response authResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	return response, header
}

func postForm(t *testing.T, endpoint, email, password string) (int, errorResponse) {
	t.Helper()
	status, raw, _ := postFormRaw(t, endpoint, email, password)
	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return status, body
}

func postFormRaw(t *testing.T, endpoint, email, password string) (int, []byte, http.Header) {
	t.Helper()
	form := url.Values{"username": {email}, "password": {password}}
	response, err := http.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw, response.Header
}

func postJSON(t *testing.T, endpoint string, payload any) (int, errorResponse, http.Header) {
	t.Helper()
	status, raw, header := postJSONRaw(t, endpoint, payload)
	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return status, body, header
}

func postJSONRaw(t *testing.T, endpoint string, payload any) (int, []byte, http.Header) {
	t.Helper()
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(rawPayload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw, response.Header
}

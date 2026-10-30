package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

// HTTP-level client verification: real router, service, and GORM repository
// against both SQLite and PostgreSQL.

type ciBackend struct {
	name         string
	gorm         *gorm.DB
	conn         *sql.DB
	server       *httptest.Server
	clients      outbound.ClientRepository
	validToken   string
	expiredToken string
	userID       int64
}

type ciClient struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	IdentityCard *string `json:"identity_card"`
	Email        *string `json:"email"`
	Phone        *string `json:"phone"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    *string `json:"updated_at"`
}

type ciList struct {
	Items   []ciClient `json:"items"`
	HasNext bool       `json:"has_next"`
	Skip    int        `json:"skip"`
	Limit   int        `json:"limit"`
}

type ciError struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func TestClientEndpointsAgainstBothBackends(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		runClientBackendSuite(t, newCiSQLiteBackend(t))
	})
	t.Run("postgres", func(t *testing.T) {
		runClientBackendSuite(t, newCiPostgresBackend(t))
	})
}

func runClientBackendSuite(t *testing.T, b *ciBackend) {
	t.Helper()
	t.Run("auth on all five routes", func(t *testing.T) { testCiAuth(t, b) })
	t.Run("create get and timestamps", func(t *testing.T) { testCiCreateGet(t, b) })
	t.Run("id bounds unknown fields and formats", func(t *testing.T) { testCiEdgeCases(t, b) })
	t.Run("update null and omitted semantics", func(t *testing.T) { testCiUpdate(t, b) })
	t.Run("search all fields accents and literal wildcards", func(t *testing.T) { testCiSearch(t, b) })
	t.Run("pagination", func(t *testing.T) { testCiPagination(t, b) })
	t.Run("duplicate identity card", func(t *testing.T) { testCiDuplicateIdentity(t, b) })
	t.Run("foreign key delete", func(t *testing.T) { testCiForeignKeyDelete(t, b) })
	t.Run("delete and not found", func(t *testing.T) { testCiDelete(t, b) })
}

func newCiSQLiteBackend(t *testing.T) *ciBackend {
	t.Helper()
	gormDB, conn := openTestDB(t, "sqlite")
	return newCiBackend(t, "sqlite", gormDB, conn)
}

func newCiPostgresBackend(t *testing.T) *ciBackend {
	t.Helper()
	gormDB, conn := openTestDB(t, "postgres")
	return newCiBackend(t, "postgres", gormDB, conn)
}

func newCiBackend(t *testing.T, name string, gormDB *gorm.DB, conn *sql.DB) *ciBackend {
	t.Helper()
	ctx := context.Background()
	users := gormstore.NewUserStore(gormDB)
	tokens := gormstore.NewRefreshTokenStore(gormDB)
	clients := gormstore.NewClientStore(gormDB)

	hasher := security.NewPasswordHasher(4)
	hash, err := hasher.Hash("client-test-password")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{
		FullName:       "Client Tester",
		Email:          name + "-clients@example.com",
		HashedPassword: hash,
		IsActive:       true,
		IsSuperuser:    false,
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	const secret = "client-integration-secret"
	issuerName := "inventory-desktop/" + name
	issuer := security.NewAccessTokenIssuer(secret, time.Hour, issuerName)
	token, err := issuer.Issue(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	expired := security.NewAccessTokenIssuer(secret, -time.Minute, issuerName)
	expiredToken, err := expired.Issue(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	authService := application.NewAuthService(users, tokens, hasher, issuer, security.NewRefreshTokenService(), time.Hour)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:    authService,
		Users:   application.NewUserService(users, tokens, hasher),
		Clients: application.NewClientService(clients),
		Logger:  logger,
		Backend: name,
	}))
	t.Cleanup(server.Close)

	return &ciBackend{name: name, gorm: gormDB, conn: conn, server: server, clients: clients, validToken: token, expiredToken: expiredToken, userID: user.ID}
}

func (b *ciBackend) request(t *testing.T, method, path string, body any, token *string) (int, []byte) {
	t.Helper()
	return doRequest(t, b.server, method, path, body, token)
}

func (b *ciBackend) authRequest(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	return b.request(t, method, path, body, &b.validToken)
}

func (b *ciBackend) createClient(t *testing.T, body map[string]any) ciClient {
	t.Helper()
	status, raw := b.authRequest(t, http.MethodPost, "/api/v1/sales/clients", body)
	if status != http.StatusOK {
		t.Fatalf("create status = %d body = %s", status, raw)
	}
	return ciDecodeClient(t, raw)
}

func ciDecodeClient(t *testing.T, raw []byte) ciClient {
	t.Helper()
	var client ciClient
	if err := json.Unmarshal(raw, &client); err != nil {
		t.Fatalf("decode client: %v body = %s", err, raw)
	}
	return client
}

func ciDecodeList(t *testing.T, raw []byte) ciList {
	t.Helper()
	var list ciList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode list: %v body = %s", err, raw)
	}
	return list
}

func ciAssertError(t *testing.T, status int, raw []byte, wantStatus int, wantCode string) ciError {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status = %d body = %s, want %d", status, raw, wantStatus)
	}
	var body ciError
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode error: %v body = %s", err, raw)
	}
	if body.Detail == "" {
		t.Fatal("error detail is empty")
	}
	if body.Code != wantCode {
		t.Fatalf("code = %q, want %q", body.Code, wantCode)
	}
	return body
}

func ciQuery(value string) string {
	return url.QueryEscape(value)
}

func testCiAuth(t *testing.T, b *ciBackend) {
	t.Helper()
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
	invalid := "not-a-valid-token"
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, raw := b.request(t, tt.method, tt.path, tt.body, nil)
			ciAssertError(t, status, raw, http.StatusUnauthorized, "NOT_AUTHENTICATED")

			status, raw = b.request(t, tt.method, tt.path, tt.body, &invalid)
			ciAssertError(t, status, raw, http.StatusUnauthorized, "INVALID_TOKEN")

			status, raw = b.request(t, tt.method, tt.path, tt.body, &b.expiredToken)
			ciAssertError(t, status, raw, http.StatusUnauthorized, "INVALID_TOKEN")

			// Any active non-superuser may use the module.
			status, raw = b.authRequest(t, tt.method, tt.path, tt.body)
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				t.Fatalf("active non-superuser got %d on %s %s: %s", status, tt.method, tt.path, raw)
			}
		})
	}
}

func testCiCreateGet(t *testing.T, b *ciBackend) {
	t.Helper()
	created := b.createClient(t, map[string]any{
		"name":          "  Ana Pérez  ",
		"identity_card": "  CI-123  ",
		"email":         "  ana@example.com  ",
		"phone":         "   ",
	})
	if created.Name != "Ana Pérez" || created.IdentityCard == nil || *created.IdentityCard != "CI-123" ||
		created.Email == nil || *created.Email != "ana@example.com" || created.Phone != nil {
		t.Fatalf("created = %+v", created)
	}
	if created.UpdatedAt != nil {
		t.Fatalf("updated_at = %v, want null", created.UpdatedAt)
	}
	createdAt, err := time.Parse(time.RFC3339, created.CreatedAt)
	if err != nil {
		t.Fatalf("created_at %q is not RFC3339: %v", created.CreatedAt, err)
	}
	if createdAt.Location() != time.UTC {
		t.Fatalf("created_at location = %v, want UTC", createdAt.Location())
	}

	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/"+ciID(created.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("get status = %d body = %s", status, raw)
	}
	got := ciDecodeClient(t, raw)
	if got.ID != created.ID || got.Name != created.Name || got.CreatedAt != created.CreatedAt ||
		!ciSameText(got.IdentityCard, created.IdentityCard) || !ciSameText(got.Email, created.Email) || got.Phone != nil {
		t.Fatalf("get = %+v, want %+v", got, created)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/999999", nil)
	body := ciAssertError(t, status, raw, http.StatusNotFound, "")
	if body.Detail != "Client not found" {
		t.Fatalf("detail = %q, want Client not found", body.Detail)
	}
}

func testCiEdgeCases(t *testing.T, b *ciBackend) {
	t.Helper()

	// FastAPI treats 0/negative ids as missing rows; the Go contract documents
	// 422 for non-positive ids on both backends.
	for _, id := range []string{"0", "-1", "99999999999999999999"} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			t.Run("invalid id "+method+" "+id, func(t *testing.T) {
				status, raw := b.authRequest(t, method, "/api/v1/sales/clients/"+id, map[string]any{"name": "Ana"})
				ciAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
			})
		}
	}

	// Unknown JSON fields are rejected on update as well as create.
	client := b.createClient(t, map[string]any{"name": "CI Unknown Fields"})
	for _, body := range []map[string]any{
		{"name": "CI Renamed", "bogus": true},
		{"identity_card": "CI-X", "unexpected": nil},
	} {
		status, raw := b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), body)
		ciAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
	}
	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("get after rejected updates status = %d body = %s", status, raw)
	}
	if got := ciDecodeClient(t, raw); got.Name != "CI Unknown Fields" {
		t.Fatalf("name after rejected updates = %q, want unchanged", got.Name)
	}

	// Email and phone have no format validation, matching legacy.
	freeform := b.createClient(t, map[string]any{
		"name":  "CI Freeform Contact",
		"email": "not-an-email",
		"phone": "call me maybe",
	})
	if freeform.Email == nil || *freeform.Email != "not-an-email" ||
		freeform.Phone == nil || *freeform.Phone != "call me maybe" {
		t.Fatalf("freeform contact = %+v, want values preserved", freeform)
	}

	// Rows with null optional columns appear in lists without panicking.
	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?search="+ciQuery("CI Unknown Fields"), nil)
	if status != http.StatusOK {
		t.Fatalf("list null columns status = %d body = %s", status, raw)
	}
	list := ciDecodeList(t, raw)
	if len(list.Items) != 1 || list.Items[0].IdentityCard != nil || list.Items[0].Email != nil || list.Items[0].Phone != nil {
		t.Fatalf("list null columns = %+v", list.Items)
	}
}

func testCiUpdate(t *testing.T, b *ciBackend) {
	t.Helper()
	client := b.createClient(t, map[string]any{
		"name":          "Update Client",
		"identity_card": "UP-1",
		"email":         "update@example.com",
		"phone":         "300",
	})

	status, raw := b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("empty update status = %d body = %s", status, raw)
	}
	if got := ciDecodeClient(t, raw); got.UpdatedAt != nil || got.Name != client.Name {
		t.Fatalf("empty update = %+v, want unchanged and updated_at null", got)
	}

	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), map[string]any{"name": "  Update Client  "})
	if status != http.StatusOK {
		t.Fatalf("trim-only update status = %d body = %s", status, raw)
	}
	if got := ciDecodeClient(t, raw); got.UpdatedAt != nil {
		t.Fatalf("trim-only update changed updated_at: %v", got.UpdatedAt)
	}

	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), map[string]any{"name": "Update Client Renamed"})
	if status != http.StatusOK {
		t.Fatalf("rename status = %d body = %s", status, raw)
	}
	renamed := ciDecodeClient(t, raw)
	if renamed.Name != "Update Client Renamed" || renamed.UpdatedAt == nil {
		t.Fatalf("renamed = %+v", renamed)
	}
	if renamed.CreatedAt != client.CreatedAt {
		t.Fatalf("created_at changed from %q to %q", client.CreatedAt, renamed.CreatedAt)
	}

	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), map[string]any{"name": "Update Client Renamed"})
	if status != http.StatusOK {
		t.Fatalf("no-op update status = %d body = %s", status, raw)
	}
	if got := ciDecodeClient(t, raw); !ciSameText(got.UpdatedAt, renamed.UpdatedAt) {
		t.Fatalf("updated_at = %v, want unchanged %v", got.UpdatedAt, renamed.UpdatedAt)
	}

	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), map[string]any{
		"identity_card": nil,
		"email":         "   ",
		"phone":         nil,
	})
	if status != http.StatusOK {
		t.Fatalf("clear status = %d body = %s", status, raw)
	}
	cleared := ciDecodeClient(t, raw)
	if cleared.IdentityCard != nil || cleared.Email != nil || cleared.Phone != nil {
		t.Fatalf("cleared = %+v, want null optional fields", cleared)
	}
	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("get cleared status = %d body = %s", status, raw)
	}
	if got := ciDecodeClient(t, raw); got.IdentityCard != nil || got.Email != nil || got.Phone != nil {
		t.Fatalf("persisted cleared = %+v", got)
	}

	for _, name := range []any{nil, "", "   "} {
		t.Run("invalid name", func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(client.ID), map[string]any{"name": name})
			ciAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}

	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/999999", map[string]any{"name": "Ghost"})
	ciAssertError(t, status, raw, http.StatusNotFound, "")
}

func testCiSearch(t *testing.T, b *ciBackend) {
	t.Helper()
	accented := b.createClient(t, map[string]any{
		"name":          "CI María Árbol",
		"identity_card": "CI-CC-1000",
		"email":         "Ci.Arbol@Example.com",
		"phone":         "+57 3001",
	})

	for _, tt := range []struct {
		name  string
		query string
		want  int64
	}{
		{"name lowercase accent", "maría", accented.ID},
		{"name uppercase accent", "ÁRBOL", accented.ID},
		{"identity card", "ci-cc-1000", accented.ID},
		{"email", "arbol@example", accented.ID},
		{"phone", "3001", accented.ID},
	} {
		t.Run("search "+tt.name, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?limit=200&search="+ciQuery(tt.query), nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			list := ciDecodeList(t, raw)
			found := false
			for _, item := range list.Items {
				if item.ID == tt.want {
					found = true
				}
			}
			if list.HasNext || !found {
				t.Fatalf("search %q = %+v has_next=%v, want client %d", tt.query, list.Items, list.HasNext, tt.want)
			}
		})
	}

	percent := b.createClient(t, map[string]any{"name": "CI 100% Steel"})
	b.createClient(t, map[string]any{"name": "CI 100 Steel"})
	underscore := b.createClient(t, map[string]any{"name": "CI a_b Steel"})
	b.createClient(t, map[string]any{"name": "CI axb Steel"})

	for _, tt := range []struct {
		query string
		want  int64
	}{{"%", percent.ID}, {"_", underscore.ID}} {
		t.Run("literal "+tt.query, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?limit=200&search="+ciQuery(tt.query), nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			list := ciDecodeList(t, raw)
			if list.HasNext || len(list.Items) != 1 || list.Items[0].ID != tt.want {
				t.Fatalf("search %q = %+v has_next=%v, want only id %d", tt.query, list.Items, list.HasNext, tt.want)
			}
		})
	}
}

func testCiPagination(t *testing.T, b *ciBackend) {
	t.Helper()
	ids := make([]int64, 0, 4)
	for _, name := range []string{"CI Page Alpha", "CI Page Beta", "CI Page Gamma", "CI Page Delta"} {
		ids = append(ids, b.createClient(t, map[string]any{"name": name}).ID)
	}

	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?search=CI+Page&limit=2", nil)
	if status != http.StatusOK {
		t.Fatalf("first page status = %d body = %s", status, raw)
	}
	first := ciDecodeList(t, raw)
	if !first.HasNext || len(first.Items) != 2 || first.Items[0].ID != ids[3] || first.Items[1].ID != ids[2] {
		t.Fatalf("first page = %+v has_next=%v, want %d/%d and true", first.Items, first.HasNext, ids[3], ids[2])
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?search=CI+Page&limit=2&skip=2", nil)
	if status != http.StatusOK {
		t.Fatalf("second page status = %d body = %s", status, raw)
	}
	last := ciDecodeList(t, raw)
	if last.HasNext || len(last.Items) != 2 || last.Items[0].ID != ids[1] || last.Items[1].ID != ids[0] {
		t.Fatalf("second page = %+v has_next=%v, want %d/%d and false", last.Items, last.HasNext, ids[1], ids[0])
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?search=CI+Page&limit=2&skip=4", nil)
	if status != http.StatusOK {
		t.Fatalf("past-end status = %d body = %s", status, raw)
	}
	if past := ciDecodeList(t, raw); past.HasNext || len(past.Items) != 0 {
		t.Fatalf("past-end page = %+v, want empty", past)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients", nil)
	if status != http.StatusOK {
		t.Fatalf("default status = %d body = %s", status, raw)
	}
	if defaults := ciDecodeList(t, raw); defaults.Skip != 0 || defaults.Limit != 100 {
		t.Fatalf("defaults = skip %d limit %d, want 0/100", defaults.Skip, defaults.Limit)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?limit=200", nil)
	if status != http.StatusOK {
		t.Fatalf("cap status = %d body = %s, want 200", status, raw)
	}
	if capped := ciDecodeList(t, raw); capped.Limit != 200 {
		t.Fatalf("cap limit = %d, want 200", capped.Limit)
	}

	for _, query := range []string{"skip=-1", "limit=0", "limit=201", "limit=abc", "search=" + strings.Repeat("a", 257)} {
		t.Run("invalid "+query, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/sales/clients?"+query, nil)
			ciAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}
}

func testCiDuplicateIdentity(t *testing.T, b *ciBackend) {
	t.Helper()
	b.createClient(t, map[string]any{"name": "CI Duplicate One", "identity_card": "CI-DUP-1"})

	status, raw := b.authRequest(t, http.MethodPost, "/api/v1/sales/clients", map[string]any{
		"name":          "CI Duplicate Two",
		"identity_card": "CI-DUP-1",
	})
	ciAssertError(t, status, raw, http.StatusBadRequest, "UNIQUE_VIOLATION")

	third := b.createClient(t, map[string]any{"name": "CI Duplicate Three", "identity_card": "CI-DUP-3"})
	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/sales/clients/"+ciID(third.ID), map[string]any{"identity_card": "CI-DUP-1"})
	ciAssertError(t, status, raw, http.StatusBadRequest, "UNIQUE_VIOLATION")

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/"+ciID(third.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("get after conflict status = %d body = %s", status, raw)
	}
	if got := ciDecodeClient(t, raw); got.IdentityCard == nil || *got.IdentityCard != "CI-DUP-3" {
		t.Fatalf("identity after failed update = %v, want CI-DUP-3", got.IdentityCard)
	}

	b.createClient(t, map[string]any{"name": "CI No Card A", "identity_card": nil})
	b.createClient(t, map[string]any{"name": "CI No Card B", "identity_card": ""})
}

func testCiForeignKeyDelete(t *testing.T, b *ciBackend) {
	t.Helper()
	client := b.createClient(t, map[string]any{"name": "CI Referenced", "identity_card": "CI-REF-1"})

	money := any("0.00")
	if b.name == "sqlite" {
		money = int64(0)
	}
	var saleID int64
	if err := b.gorm.Raw(
		"INSERT INTO sales (created_at, total_amount, amount_paid, client_id) VALUES (?, ?, ?, ?) RETURNING id",
		"2026-01-01T00:00:00.000000000Z", money, money, client.ID,
	).Scan(&saleID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = b.gorm.Exec("DELETE FROM sales WHERE id = ?", saleID).Error
	})

	status, raw := b.authRequest(t, http.MethodDelete, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	ciAssertError(t, status, raw, http.StatusBadRequest, "FOREIGN_KEY_VIOLATION")

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("referenced client must survive: status = %d body = %s", status, raw)
	}
}

func testCiDelete(t *testing.T, b *ciBackend) {
	t.Helper()
	client := b.createClient(t, map[string]any{"name": "CI Delete Me"})
	status, raw := b.authRequest(t, http.MethodDelete, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", status, raw)
	}
	if deleted := ciDecodeClient(t, raw); deleted.ID != client.ID {
		t.Fatalf("deleted = %+v, want id %d", deleted, client.ID)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	body := ciAssertError(t, status, raw, http.StatusNotFound, "")
	if body.Detail != "Client not found" {
		t.Fatalf("detail = %q, want Client not found", body.Detail)
	}

	status, raw = b.authRequest(t, http.MethodDelete, "/api/v1/sales/clients/"+ciID(client.ID), nil)
	ciAssertError(t, status, raw, http.StatusNotFound, "")
}

func ciID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func ciSameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

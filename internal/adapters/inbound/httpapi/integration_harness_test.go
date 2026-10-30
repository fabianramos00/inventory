package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"inventory-desktop/internal/db"
	"inventory-desktop/tests/testdb"
)

// openTestDB opens a freshly migrated database for the HTTP integration suites.
// PostgreSQL is skipped when INVENTORY_TEST_DATABASE_URL is unset and is
// serialized through the shared test lock.
func openTestDB(t *testing.T, mode string) (*gorm.DB, *sql.DB) {
	t.Helper()
	ctx := context.Background()

	if mode == "sqlite" {
		gormDB, conn, err := db.OpenSQLiteGORM(filepath.Join(t.TempDir(), "inventory.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		if err := db.MigrateSQLite(ctx, conn); err != nil {
			t.Fatal(err)
		}
		return gormDB, conn
	}

	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the postgres integration suites")
	}
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	gormDB, conn, err := db.OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `TRUNCATE
		refresh_tokens, users, categories, materials, brands, measurement_units,
		products, price_history, clients, providers, supplier_catalogs,
		purchase_orders, order_items, sales, sale_items
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return gormDB, conn
}

// doRequest performs one JSON request against the test server. A nil token
// sends no Authorization header.
func doRequest(t *testing.T, server *httptest.Server, method, path string, body any, token *string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != nil {
		request.Header.Set("Authorization", "Bearer "+*token)
	}
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

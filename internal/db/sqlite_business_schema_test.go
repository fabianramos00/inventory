package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"inventory-desktop/tests/testdb"
)

const schemaTestTimestamp = "2026-01-02T03:04:05.123456789Z"

var businessTableNames = []string{
	"brands",
	"categories",
	"clients",
	"materials",
	"measurement_units",
	"order_items",
	"price_history",
	"products",
	"providers",
	"purchase_orders",
	"sale_items",
	"sales",
	"supplier_catalogs",
}

var publicTableNames = []string{
	"brands",
	"categories",
	"clients",
	"materials",
	"measurement_units",
	"order_items",
	"price_history",
	"products",
	"providers",
	"purchase_orders",
	"refresh_tokens",
	"sale_items",
	"sales",
	"supplier_catalogs",
	"users",
}

func TestSQLiteBusinessSchemaApplies(t *testing.T) {
	conn := openMigratedSQLite(t)

	got := sqliteTableNames(t, conn)
	if !reflect.DeepEqual(got, publicTableNames) {
		t.Fatalf("public tables = %v, want %v", got, publicTableNames)
	}
}

func TestSQLiteBusinessSchemaIsStrict(t *testing.T) {
	conn := openMigratedSQLite(t)

	for _, table := range businessTableNames {
		var ddl string
		if err := conn.QueryRow(
			`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&ddl); err != nil {
			t.Fatalf("read DDL for %s: %v", table, err)
		}
		if !strings.Contains(strings.ToUpper(ddl), "STRICT") {
			t.Fatalf("table %s is not STRICT: %s", table, ddl)
		}
	}
}

func TestSQLiteBusinessSchemaColumnTypes(t *testing.T) {
	conn := openMigratedSQLite(t)

	money := []struct{ table, column string }{
		{"products", "price"},
		{"products", "cost"},
		{"price_history", "old_price"},
		{"price_history", "new_price"},
		{"sales", "total_amount"},
		{"sales", "amount_paid"},
		{"sale_items", "unit_price"},
		{"sale_items", "subtotal"},
		{"order_items", "unit_cost"},
		{"order_items", "subtotal"},
		{"purchase_orders", "total_amount"},
		{"supplier_catalogs", "last_quoted_cost"},
	}
	for _, c := range money {
		if got := sqliteColumnType(t, conn, c.table, c.column); got != "INTEGER" {
			t.Fatalf("%s.%s type = %q, want INTEGER", c.table, c.column, got)
		}
	}

	timestamps := []struct{ table, column string }{
		{"clients", "created_at"},
		{"clients", "updated_at"},
		{"sales", "created_at"},
		{"sales", "updated_at"},
		{"price_history", "changed_at"},
		{"purchase_orders", "order_date"},
	}
	for _, c := range timestamps {
		if got := sqliteColumnType(t, conn, c.table, c.column); got != "TEXT" {
			t.Fatalf("%s.%s type = %q, want TEXT", c.table, c.column, got)
		}
	}
}

func TestSQLiteBusinessSchemaIntegrity(t *testing.T) {
	conn := openMigratedSQLite(t)
	seedRepresentativeRows(t, conn)

	rows, err := conn.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	violation := rows.Next()
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if violation {
		t.Fatal("PRAGMA foreign_key_check reported violations")
	}

	var result string
	if err := conn.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatalf("PRAGMA integrity_check = %q, want ok", result)
	}
}

func TestSQLiteBusinessSchemaRejectsInvalidData(t *testing.T) {
	conn := openMigratedSQLite(t)
	ids := seedRepresentativeRows(t, conn)

	tests := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name:  "negative stock quantity",
			query: `INSERT INTO products (sku, name, price, cost, stock_quantity, low_stock_threshold) VALUES (?, ?, ?, ?, ?, ?)`,
			args:  []any{"SKU-NEGATIVE", "Negativo", 100, 100, -1, 0},
		},
		{
			name:  "malformed timestamp",
			query: `INSERT INTO price_history (product_id, old_price, new_price, changed_at) VALUES (?, ?, ?, ?)`,
			args:  []any{ids.productID, 100, 200, "2026-01-02 03:04:05"},
		},
		{
			name:  "lowercase payment status enum",
			query: `INSERT INTO sales (created_at, total_amount, amount_paid, payment_status) VALUES (?, ?, ?, ?)`,
			args:  []any{schemaTestTimestamp, 0, 0, "pending"},
		},
		{
			name:  "delivered quantity over quantity",
			query: `INSERT INTO sale_items (sale_id, product_id, quantity, unit_price, subtotal, delivered_quantity) VALUES (?, ?, ?, ?, ?, ?)`,
			args:  []any{ids.saleID, ids.productID, 2, 100, 200, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := conn.Exec(tt.query, tt.args...); err == nil {
				t.Fatal("insert succeeded, want a constraint failure")
			} else if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
				t.Fatalf("insert error = %v, want a constraint failure", err)
			}
		})
	}
}

func TestSQLiteBusinessSchemaForeignKeys(t *testing.T) {
	t.Run("product delete cascades price history", func(t *testing.T) {
		conn := openMigratedSQLite(t)
		productID := insertProduct(t, conn, "SKU-CASCADE", nil)
		if _, err := conn.Exec(
			`INSERT INTO price_history (product_id, old_price, new_price, changed_at) VALUES (?, ?, ?, ?)`,
			productID, 100, 200, schemaTestTimestamp,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`DELETE FROM products WHERE id = ?`, productID); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := conn.QueryRow(`SELECT count(*) FROM price_history WHERE product_id = ?`, productID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("price_history rows after product delete = %d, want 0", count)
		}
	})

	t.Run("product referenced by sale item", func(t *testing.T) {
		conn := openMigratedSQLite(t)
		productID := insertProduct(t, conn, "SKU-SALE-ITEM", nil)
		saleID := insertSale(t, conn)
		if _, err := conn.Exec(
			`INSERT INTO sale_items (sale_id, product_id, quantity, unit_price, subtotal, delivered_quantity) VALUES (?, ?, ?, ?, ?, ?)`,
			saleID, productID, 1, 100, 100, 0,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`DELETE FROM products WHERE id = ?`, productID); err == nil {
			t.Fatal("delete succeeded, want a foreign key violation")
		}
		if got := countRows(t, conn, `SELECT count(*) FROM products WHERE id = ?`, productID); got != 1 {
			t.Fatalf("product rows after failed delete = %d, want 1", got)
		}
	})

	t.Run("category referenced by product", func(t *testing.T) {
		conn := openMigratedSQLite(t)
		res, err := conn.Exec(`INSERT INTO categories (name) VALUES (?)`, "Categoria FK")
		if err != nil {
			t.Fatal(err)
		}
		categoryID, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		insertProduct(t, conn, "SKU-CATEGORY", &categoryID)
		if _, err := conn.Exec(`DELETE FROM categories WHERE id = ?`, categoryID); err == nil {
			t.Fatal("delete succeeded, want a foreign key violation")
		}
		if got := countRows(t, conn, `SELECT count(*) FROM categories WHERE id = ?`, categoryID); got != 1 {
			t.Fatalf("category rows after failed delete = %d, want 1", got)
		}
	})
}

func TestSQLiteBusinessSchemaIndexes(t *testing.T) {
	conn := openMigratedSQLite(t)

	for _, name := range []string{
		"ux_sale_items_sale_product",
		"ux_order_items_order_product",
		"ux_supplier_catalogs_product_provider",
	} {
		if got := countRows(t, conn, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name); got != 0 {
			t.Fatalf("index %s exists, want it omitted", name)
		}
	}

	for _, table := range businessTableNames {
		for _, index := range sqliteIndexes(t, conn, table) {
			if index.origin != "c" {
				continue
			}
			if cols := sqliteIndexColumns(t, conn, index.name); len(cols) == 1 && cols[0] == "id" {
				t.Fatalf("%s has redundant index %s on the primary key", table, index.name)
			}
		}
	}
}

func TestSQLitePostgresSchemaParity(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to compare SQLite and PostgreSQL schemas")
	}

	ctx := context.Background()
	sqliteConn := openMigratedSQLite(t)
	sqliteTables := sqliteTableNames(t, sqliteConn)
	if !reflect.DeepEqual(sqliteTables, publicTableNames) {
		t.Fatalf("sqlite tables = %v, want %v", sqliteTables, publicTableNames)
	}

	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	_, pgConn, err := OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pgConn.Close() })
	if err := pgConn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := MigratePostgres(ctx, pgConn); err != nil {
		t.Fatal(err)
	}

	pgTables := postgresTableNames(t, ctx, pgConn)
	if !reflect.DeepEqual(pgTables, sqliteTables) {
		t.Fatalf("table names differ\nsqlite:   %v\npostgres: %v", sqliteTables, pgTables)
	}

	for _, table := range sqliteTables {
		sqliteCols := sorted(sqliteTableColumns(t, sqliteConn, table))
		pgCols := sorted(postgresTableColumns(t, ctx, pgConn, table))
		if !reflect.DeepEqual(sqliteCols, pgCols) {
			t.Fatalf("%s columns differ\nsqlite:   %v\npostgres: %v", table, sqliteCols, pgCols)
		}
	}
}

type seedIDs struct {
	userID    int64
	productID int64
	saleID    int64
}

func seedRepresentativeRows(t *testing.T, conn *sql.DB) seedIDs {
	t.Helper()

	insert := func(query string, args ...any) int64 {
		t.Helper()
		res, err := conn.Exec(query, args...)
		if err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	var ids seedIDs
	ids.userID = insert(
		`INSERT INTO users (full_name, email, hashed_password) VALUES (?, ?, ?)`,
		"Seed User", "seed@example.test", "hash",
	)
	categoryID := insert(
		`INSERT INTO categories (name, description, code) VALUES (?, ?, ?)`,
		"Herramientas", "Herramientas manuales", "HER",
	)
	materialID := insert(`INSERT INTO materials (name) VALUES (?)`, "Acero")
	brandID := insert(
		`INSERT INTO brands (name, logo_url) VALUES (?, ?)`,
		"Truper", "https://example.test/logo.png",
	)
	unitID := insert(
		`INSERT INTO measurement_units (name, abbreviation) VALUES (?, ?)`,
		"Unidad", "UN",
	)
	ids.productID = insert(
		`INSERT INTO products (sku, name, price, cost, stock_quantity, low_stock_threshold, measurement_unit_id, brand_id, category_id, material_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"SKU-SEED", "Martillo", 123456, 100000, 5, 2, unitID, brandID, categoryID, materialID,
	)
	insert(
		`INSERT INTO price_history (product_id, old_price, new_price, changed_at) VALUES (?, ?, ?, ?)`,
		ids.productID, 100000, 123456, schemaTestTimestamp,
	)
	clientID := insert(
		`INSERT INTO clients (created_at, updated_at, name, identity_card, email, phone) VALUES (?, ?, ?, ?, ?, ?)`,
		schemaTestTimestamp, schemaTestTimestamp, "Cliente Seed", "CC-1", "client@example.test", "3000000000",
	)
	providerID := insert(
		`INSERT INTO providers (name, contact_info, email, phone, created_by_id) VALUES (?, ?, ?, ?, ?)`,
		"Proveedor Seed", "Call center", "provider@example.test", "3100000000", ids.userID,
	)
	insert(
		`INSERT INTO supplier_catalogs (product_id, provider_id, supplier_sku, last_quoted_cost, lead_time) VALUES (?, ?, ?, ?, ?)`,
		ids.productID, providerID, "PROV-SKU-1", 90000, 7,
	)
	orderID := insert(
		`INSERT INTO purchase_orders (provider_id, order_date, total_amount, created_by_id) VALUES (?, ?, ?, ?)`,
		providerID, schemaTestTimestamp, 90000, ids.userID,
	)
	insert(
		`INSERT INTO order_items (order_id, product_id, quantity, unit_cost, subtotal, supplier_sku) VALUES (?, ?, ?, ?, ?, ?)`,
		orderID, ids.productID, 2, 90000, 180000, "PROV-SKU-1",
	)
	ids.saleID = insert(
		`INSERT INTO sales (created_at, updated_at, total_amount, amount_paid, payment_method, created_by_id, client_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		schemaTestTimestamp, schemaTestTimestamp, 123456, 123456, "CASH", ids.userID, clientID,
	)
	insert(
		`INSERT INTO sale_items (sale_id, product_id, quantity, unit_price, subtotal, delivered_quantity) VALUES (?, ?, ?, ?, ?, ?)`,
		ids.saleID, ids.productID, 2, 123456, 246912, 1,
	)
	return ids
}

func openMigratedSQLite(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business-schema.sqlite")
	_, conn, err := OpenSQLiteGORM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := MigrateSQLite(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

func sqliteTableNames(t *testing.T, conn *sql.DB) []string {
	t.Helper()
	rows, err := conn.Query(
		`SELECT name FROM sqlite_master
		 WHERE type = 'table'
		   AND name NOT LIKE 'sqlite_%'
		   AND name <> 'goose_db_version'
		 ORDER BY name`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func sqliteTableColumns(t *testing.T, conn *sql.DB, table string) []string {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func sqliteColumnType(t *testing.T, conn *sql.DB, table, column string) string {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return typ
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("column %s.%s not found", table, column)
	return ""
}

type sqliteIndex struct {
	name   string
	origin string
}

func sqliteIndexes(t *testing.T, conn *sql.DB, table string) []sqliteIndex {
	t.Helper()
	rows, err := conn.Query(`PRAGMA index_list(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var indexes []sqliteIndex
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, sqliteIndex{name: name, origin: origin})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return indexes
}

func sqliteIndexColumns(t *testing.T, conn *sql.DB, index string) []string {
	t.Helper()
	rows, err := conn.Query(`PRAGMA index_info(` + index + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var (
			seqNo int
			cid   int
			name  sql.NullString
		)
		if err := rows.Scan(&seqNo, &cid, &name); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func insertProduct(t *testing.T, conn *sql.DB, sku string, categoryID *int64) int64 {
	t.Helper()
	var category any
	if categoryID != nil {
		category = *categoryID
	}
	res, err := conn.Exec(
		`INSERT INTO products (sku, name, price, cost, stock_quantity, low_stock_threshold, category_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sku, "Product "+sku, 100, 100, 1, 0, category,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertSale(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	res, err := conn.Exec(
		`INSERT INTO sales (created_at, total_amount, amount_paid) VALUES (?, ?, ?)`,
		schemaTestTimestamp, 100, 100,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func countRows(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := conn.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func postgresTableNames(t *testing.T, ctx context.Context, conn *sql.DB) []string {
	t.Helper()
	rows, err := conn.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public'
		   AND table_type = 'BASE TABLE'
		   AND table_name <> 'goose_db_version'
		 ORDER BY table_name`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func postgresTableColumns(t *testing.T, ctx context.Context, conn *sql.DB, table string) []string {
	t.Helper()
	rows, err := conn.QueryContext(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1
		 ORDER BY ordinal_position`,
		table,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func sorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

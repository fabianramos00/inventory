package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	sqlitemigrations "inventory-desktop/migrations/sqlite"
)

func TestSQLiteSalePaymentsMigrationBackfillsOpeningPayments(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sale-payments-migration.sqlite")
	_, conn, err := OpenSQLiteGORM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, sqlitemigrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Exec(
		`INSERT INTO users (full_name, email, hashed_password) VALUES (?, ?, ?)`,
		"Migration User", "migration@example.test", "hash",
	); err != nil {
		t.Fatal(err)
	}
	var userID int64
	if err := conn.QueryRow(`SELECT id FROM users WHERE email = ?`, "migration@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}

	insertSale := func(amountPaid int64, method any) int64 {
		t.Helper()
		res, err := conn.Exec(
			`INSERT INTO sales (created_at, total_amount, amount_paid, payment_method, created_by_id)
			 VALUES (?, ?, ?, ?, ?)`,
			schemaTestTimestamp, amountPaid, amountPaid, method, userID,
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
	paidSaleID := insertSale(500, "CASH")
	zeroSaleID := insertSale(0, nil)
	nullMethodSaleID := insertSale(250, nil)

	var salesBefore int
	if err := conn.QueryRow(`SELECT count(*) FROM sales`).Scan(&salesBefore); err != nil {
		t.Fatal(err)
	}

	if err := MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}

	var paymentMethodColumn int
	if err := conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('sales') WHERE name = 'payment_method'`,
	).Scan(&paymentMethodColumn); err != nil {
		t.Fatal(err)
	}
	if paymentMethodColumn != 0 {
		t.Fatal("sales.payment_method still exists after migration")
	}

	var salesAfter int
	if err := conn.QueryRow(`SELECT count(*) FROM sales`).Scan(&salesAfter); err != nil {
		t.Fatal(err)
	}
	if salesAfter != salesBefore {
		t.Fatalf("sales rows = %d, want %d preserved", salesAfter, salesBefore)
	}
	for _, id := range []int64{paidSaleID, zeroSaleID, nullMethodSaleID} {
		var found int
		if err := conn.QueryRow(`SELECT count(*) FROM sales WHERE id = ?`, id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Fatalf("sale %d was not preserved by the rebuild", id)
		}
	}

	assertSalesIndexes(t, conn)
	assertSalesConstraints(t, conn)

	var indexCount int
	if err := conn.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'ix_sale_payments_sale_id_created_at_id'`,
	).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("sale_payments index rows = %d, want 1", indexCount)
	}

	// New rows must respect the payment constraints after the rebuild.
	for _, amount := range []int64{0, -1} {
		if _, err := conn.Exec(
			`INSERT INTO sale_payments (sale_id, amount, payment_method, created_at) VALUES (?, ?, 'CASH', ?)`,
			zeroSaleID, amount, schemaTestTimestamp,
		); err == nil {
			t.Fatalf("sale_payments accepted amount %d", amount)
		}
	}
	if _, err := conn.Exec(
		`INSERT INTO sale_payments (sale_id, amount, payment_method, created_at) VALUES (?, 1, 'BITCOIN', ?)`,
		zeroSaleID, schemaTestTimestamp,
	); err == nil {
		t.Fatal("sale_payments accepted an invalid method")
	}
	if _, err := conn.Exec(
		`INSERT INTO sales (created_at, total_amount, amount_paid, payment_status) VALUES (?, 1, 0, 'BOGUS')`,
		schemaTestTimestamp,
	); err == nil {
		t.Fatal("rebuilt sales accepted an invalid payment_status")
	}

	assertPayments := func(saleID int64, wantRows int, wantAmount int64, wantMethod string) {
		t.Helper()
		var rows, amount, creator int64
		var method, createdAt string
		if err := conn.QueryRow(
			`SELECT count(*), COALESCE(SUM(amount), 0), COALESCE(MAX(payment_method), ''),
			        COALESCE(MAX(created_by_id), -1), COALESCE(MAX(created_at), '')
			 FROM sale_payments WHERE sale_id = ?`, saleID,
		).Scan(&rows, &amount, &method, &creator, &createdAt); err != nil {
			t.Fatal(err)
		}
		if rows != int64(wantRows) || amount != wantAmount {
			t.Fatalf("sale %d payments = %d rows / %d amount, want %d / %d", saleID, rows, amount, wantRows, wantAmount)
		}
		if wantRows == 0 {
			return
		}
		if method != wantMethod {
			t.Fatalf("sale %d payment method = %q, want %q", saleID, method, wantMethod)
		}
		if creator != userID {
			t.Fatalf("sale %d payment creator = %d, want %d", saleID, creator, userID)
		}
		if createdAt != schemaTestTimestamp {
			t.Fatalf("sale %d payment created_at = %q, want %q", saleID, createdAt, schemaTestTimestamp)
		}
	}
	assertPayments(paidSaleID, 1, 500, "CASH")
	assertPayments(zeroSaleID, 0, 0, "")
	assertPayments(nullMethodSaleID, 1, 250, "")

	if _, err := conn.Exec(`DELETE FROM sales WHERE id = ?`, paidSaleID); err != nil {
		t.Fatal(err)
	}
	assertPayments(paidSaleID, 0, 0, "")
	assertPayments(nullMethodSaleID, 1, 250, "")

	assertForeignKey(t, conn, "sale_payments", "sales", "sale_id", "CASCADE")
	assertForeignKey(t, conn, "sale_payments", "users", "created_by_id", "NO ACTION")
	assertForeignKey(t, conn, "sale_items", "sales", "sale_id", "NO ACTION")

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
		t.Fatal("PRAGMA foreign_key_check reported violations after migration")
	}
}

func assertSalesIndexes(t *testing.T, conn *sql.DB) {
	t.Helper()
	want := map[string]bool{
		"ix_sales_created_at":      true,
		"ix_sales_payment_status":  true,
		"ix_sales_delivery_status": true,
		"ix_sales_client_id":       true,
		"ix_sales_created_by_id":   true,
	}
	rows, err := conn.Query(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'sales' AND name NOT LIKE 'sqlite_autoindex%'`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("rebuilt sales is missing index %s", name)
		}
	}
	if got["ix_sales_payment_method"] {
		t.Fatal("rebuilt sales still has ix_sales_payment_method")
	}
}

func assertSalesConstraints(t *testing.T, conn *sql.DB) {
	t.Helper()
	var ddl string
	if err := conn.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'sales'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ck_sales_created_at_format",
		"ck_sales_updated_at_format",
		"ck_sales_total_amount_nonnegative",
		"ck_sales_total_amount_range",
		"ck_sales_amount_paid_range",
		"ck_sales_payment_status",
		"ck_sales_delivery_status",
		"fk_sales_client_id",
		"sales_created_by_id_fkey",
	} {
		if !strings.Contains(ddl, name) {
			t.Fatalf("rebuilt sales is missing constraint %s", name)
		}
	}
	if strings.Contains(ddl, "payment_method") {
		t.Fatal("rebuilt sales DDL still mentions payment_method")
	}
	if !strings.Contains(strings.ToUpper(ddl), "STRICT") {
		t.Fatal("rebuilt sales is not STRICT")
	}
}

func assertForeignKey(t *testing.T, conn *sql.DB, table, refTable, from, onDelete string) {
	t.Helper()
	rows, err := conn.Query(`PRAGMA foreign_key_list('` + table + `')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id, seq                       int
			ref, fromCol, toCol, onUpdate string
			onDeleteValue, match          string
		)
		if err := rows.Scan(&id, &seq, &ref, &fromCol, &toCol, &onUpdate, &onDeleteValue, &match); err != nil {
			t.Fatal(err)
		}
		if ref == refTable && fromCol == from {
			if onDeleteValue != onDelete {
				t.Fatalf("%s.%s ON DELETE = %s, want %s", table, from, onDeleteValue, onDelete)
			}
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("%s has no foreign key %s -> %s", table, from, refTable)
}

package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	sqlitemigrations "inventory-desktop/migrations/sqlite"
)

// SQLite 00008 adds the integer-cents debt cache and backfills it from sales.
func TestSQLiteClientDebtMigrationBackfillsIntegerCents(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "client-debt-migration.sqlite")
	_, conn, err := OpenSQLiteGORM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, sqlitemigrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 7); err != nil {
		t.Fatal(err)
	}

	insertClient := func(name string, updatedAt any) int64 {
		t.Helper()
		result, err := conn.Exec(
			`INSERT INTO clients (created_at, updated_at, name) VALUES (?, ?, ?)`,
			schemaTestTimestamp, updatedAt, name,
		)
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	insertSale := func(total, paid int64, clientID any) {
		t.Helper()
		if _, err := conn.Exec(
			`INSERT INTO sales (created_at, total_amount, amount_paid, client_id)
			 VALUES (?, ?, ?, ?)`,
			schemaTestTimestamp, total, paid, clientID,
		); err != nil {
			t.Fatal(err)
		}
	}

	indebted := insertClient("Indebted client", schemaTestTimestamp)
	settled := insertClient("Settled client", nil)
	insertSale(10000, 4000, indebted) // 60.00 owed
	insertSale(1000, 2000, indebted)  // legacy overpayment clamps to 0
	insertSale(500, 500, indebted)    // paid exactly
	insertSale(5000, 0, nil)          // anonymous sale is excluded
	insertSale(250, 250, settled)     // paid exactly

	if err := MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}

	var columnType string
	if err := conn.QueryRow(
		`SELECT type FROM pragma_table_info('clients') WHERE name = 'debt_amount'`,
	).Scan(&columnType); err != nil {
		t.Fatal(err)
	}
	if columnType != "INTEGER" {
		t.Fatalf("debt_amount type = %q, want INTEGER", columnType)
	}

	for _, tt := range []struct {
		id   int64
		want int64
	}{
		{indebted, 6000},
		{settled, 0},
	} {
		var got int64
		if err := conn.QueryRow(`SELECT debt_amount FROM clients WHERE id = ?`, tt.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Fatalf("client %d debt_amount = %d cents, want %d", tt.id, got, tt.want)
		}
	}

	var updatedAt string
	if err := conn.QueryRow(`SELECT updated_at FROM clients WHERE id = ?`, indebted).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if updatedAt != schemaTestTimestamp {
		t.Fatalf("backfill changed updated_at to %q, want %q", updatedAt, schemaTestTimestamp)
	}

	newClient, err := conn.Exec(
		`INSERT INTO clients (created_at, name) VALUES (?, ?)`, schemaTestTimestamp, "Default client",
	)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := newClient.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	var defaultDebt int64
	if err := conn.QueryRow(`SELECT debt_amount FROM clients WHERE id = ?`, newID).Scan(&defaultDebt); err != nil {
		t.Fatal(err)
	}
	if defaultDebt != 0 {
		t.Fatalf("new client debt_amount = %d, want default 0", defaultDebt)
	}

	if _, err := conn.Exec(`UPDATE clients SET debt_amount = -1 WHERE id = ?`, indebted); err == nil {
		t.Fatal("clients.debt_amount accepted a negative value")
	}
}

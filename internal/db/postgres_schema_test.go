package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"inventory-desktop/tests/testdb"
)

func TestPostgresSchemaUsesIdentityAndDropsRedundantIndexes(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to verify the PostgreSQL schema")
	}

	ctx := context.Background()
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	_, conn, err := OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}

	for _, table := range publicTableNames {
		var identity string
		if err := conn.QueryRowContext(ctx,
			`SELECT is_identity FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name = $1 AND column_name = 'id'`,
			table,
		).Scan(&identity); err != nil {
			t.Fatalf("%s.id identity: %v", table, err)
		}
		if identity != "YES" {
			t.Fatalf("%s.id is_identity = %q, want YES", table, identity)
		}

		var redundant string
		err := conn.QueryRowContext(ctx,
			`SELECT i.relname
			 FROM pg_index idx
			 JOIN pg_class i ON i.oid = idx.indexrelid
			 JOIN pg_class tbl ON tbl.oid = idx.indrelid
			 JOIN pg_namespace n ON n.oid = tbl.relnamespace
			 JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = ANY (idx.indkey)
			 WHERE n.nspname = 'public'
			   AND tbl.relname = $1
			   AND NOT idx.indisprimary
			 GROUP BY i.relname
			 HAVING count(*) = 1 AND bool_and(a.attname = 'id')`,
			table,
		).Scan(&redundant)
		if err == nil {
			t.Fatalf("%s has redundant id index %s", table, redundant)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s redundant id index: %v", table, err)
		}
	}
}

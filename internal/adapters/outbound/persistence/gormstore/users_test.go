package gormstore

import (
	"context"
	"path/filepath"
	"testing"

	"inventory-desktop/internal/db"
)

func TestUserRepositoryReadsNullableFullName(t *testing.T) {
	ctx := context.Background()
	gormDB, conn, err := db.OpenSQLiteGORM(filepath.Join(t.TempDir(), "inventory.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO users (full_name, email, hashed_password, is_active, is_superuser)
		VALUES (NULL, ?, ?, 1, 0)`,
		"nullable@example.com", "hash",
	); err != nil {
		t.Fatal(err)
	}

	user, err := NewUserStore(gormDB).GetByEmail(ctx, "nullable@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.FullName != "" {
		t.Fatalf("full_name = %q, want empty string", user.FullName)
	}
}

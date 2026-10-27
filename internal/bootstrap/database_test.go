package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"inventory-desktop/internal/config"
	"inventory-desktop/internal/domain"
)

const unreachableDSN = "postgresql://inventory:inventory@127.0.0.1:1/inventory_desktop_test?connect_timeout=1"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestOpenSQLiteMode(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		DBMode:      "sqlite",
		SQLitePath:  filepath.Join(t.TempDir(), "data", "inventory.sqlite"),
		AutoMigrate: true,
	}

	stores, err := Open(ctx, cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stores.Closer() })

	if stores.Backend != "sqlite" {
		t.Fatalf("backend = %q, want sqlite", stores.Backend)
	}
	if stores.Users == nil || stores.RefreshTokens == nil {
		t.Fatal("sqlite stores are not wired")
	}

	user := &domain.User{FullName: "Offline User", Email: "offline@example.com", HashedPassword: "hash", IsActive: true}
	if err := stores.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user on migrated sqlite store: %v", err)
	}
	if _, err := stores.Users.GetByEmail(ctx, user.Email); err != nil {
		t.Fatalf("read user from migrated sqlite store: %v", err)
	}
}

func TestOpenPostgresModeFailsWhenUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := &config.Config{
		DBMode:      "postgres",
		DatabaseURL: unreachableDSN,
		SQLitePath:  filepath.Join(t.TempDir(), "fallback.sqlite"),
		AutoMigrate: true,
	}

	stores, err := Open(ctx, cfg, discardLogger())
	if err == nil {
		stores.Closer()
		t.Fatal("postgres mode silently succeeded with an unreachable server")
	}
	if stores != nil {
		t.Fatalf("stores = %+v, want nil on failure", stores)
	}
}

func TestOpenAutoFallsBackToSQLite(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		DBMode:      "auto",
		DatabaseURL: unreachableDSN,
		SQLitePath:  filepath.Join(t.TempDir(), "data", "inventory.sqlite"),
		AutoMigrate: true,
	}

	stores, err := Open(ctx, cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stores.Closer() })

	if stores.Backend != "sqlite" {
		t.Fatalf("backend = %q, want sqlite fallback", stores.Backend)
	}
	if _, err := os.Stat(cfg.SQLitePath); err != nil {
		t.Fatalf("sqlite fallback file was not created: %v", err)
	}
}

func TestSQLitePermissions(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		DBMode:      "sqlite",
		SQLitePath:  filepath.Join(t.TempDir(), "nested", "inventory.sqlite"),
		AutoMigrate: true,
	}

	stores, err := Open(ctx, cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stores.Closer() })

	dirInfo, err := os.Stat(filepath.Dir(cfg.SQLitePath))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("sqlite directory mode = %04o, want 0700", got)
	}

	fileInfo, err := os.Stat(cfg.SQLitePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("sqlite file mode = %04o, want 0600", got)
	}
}

package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestTargetAttrsOmitsCredentials(t *testing.T) {
	attrs := targetAttrs("postgresql://alice:s3cr3t@db.example.com:5432/store")
	rendered := fmt.Sprint(attrs...)

	if !strings.Contains(rendered, "db.example.com:5432") {
		t.Fatalf("target = %q, want the host and port", rendered)
	}
	if !strings.Contains(rendered, "store") {
		t.Fatalf("target = %q, want the database name", rendered)
	}
	if strings.Contains(rendered, "alice") || strings.Contains(rendered, "s3cr3t") {
		t.Fatalf("target = %q leaked the credentials", rendered)
	}
}

func TestTargetAttrsFallsBackToGenericMarker(t *testing.T) {
	attrs := targetAttrs("host=127.0.0.1 port=5432 dbname=store")
	rendered := fmt.Sprint(attrs...)
	if !strings.Contains(rendered, "unknown") {
		t.Fatalf("target = %q, want the generic marker", rendered)
	}
	if strings.Contains(rendered, "5432") {
		t.Fatalf("target = %q leaked a non-URI DSN", rendered)
	}
}

func TestAutoFallbackLogsTargetAndFailHardHint(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg := &config.Config{
		DBMode:      "auto",
		DatabaseURL: "postgresql://alice:s3cr3t@127.0.0.1:1/store",
		SQLitePath:  filepath.Join(t.TempDir(), "data", "inventory.sqlite"),
		AutoMigrate: true,
	}

	stores, err := Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stores.Closer() })

	rendered := logs.String()
	if !strings.Contains(rendered, "set DB_MODE=postgres to fail hard") {
		t.Fatalf("logs = %q, want the fail-hard hint", rendered)
	}
	if !strings.Contains(rendered, "host=127.0.0.1:1") {
		t.Fatalf("logs = %q, want the postgres host and port", rendered)
	}
	if !strings.Contains(rendered, "database=store") {
		t.Fatalf("logs = %q, want the postgres database name", rendered)
	}
	if strings.Contains(rendered, "s3cr3t") {
		t.Fatalf("logs = %q leaked the password", rendered)
	}
}

func TestOpenPostgresModePingTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range held {
			conn.Close()
		}
	})

	original := postgresPingTimeout
	postgresPingTimeout = 300 * time.Millisecond
	t.Cleanup(func() { postgresPingTimeout = original })

	dsn := fmt.Sprintf("postgresql://inventory:inventory@%s/inventory_desktop_test?sslmode=disable", listener.Addr().String())
	cfg := &config.Config{DBMode: "postgres", DatabaseURL: dsn}

	start := time.Now()
	stores, err := Open(context.Background(), cfg, discardLogger())
	elapsed := time.Since(start)
	if err == nil {
		stores.Closer()
		t.Fatal("postgres mode succeeded against a server that never answers")
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("ping failed after %v, expected it to wait for the %v timeout", elapsed, postgresPingTimeout)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("ping ignored the %v timeout and took %v", postgresPingTimeout, elapsed)
	}
}

func TestDescribeTarget(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL: "postgresql://alice:s3cr3t@127.0.0.1:5432/inventory_desktop",
		SQLitePath:  "/tmp/inventory.sqlite",
	}

	if got := DescribeTarget(cfg, "postgres"); got != "host=127.0.0.1:5432 database=inventory_desktop" {
		t.Fatalf("DescribeTarget(postgres) = %q", got)
	}
	if got := DescribeTarget(cfg, "sqlite"); got != "path=/tmp/inventory.sqlite" {
		t.Fatalf("DescribeTarget(sqlite) = %q", got)
	}
}

func TestAnnounceSeedTargetWarnsOnSQLite(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL: "postgresql://alice:s3cr3t@127.0.0.1:5432/inventory_desktop",
		SQLitePath:  "/tmp/inventory.sqlite",
	}

	var out bytes.Buffer
	AnnounceSeedTarget(&out, cfg, "sqlite")
	rendered := out.String()
	if !strings.Contains(rendered, "backend=sqlite") {
		t.Fatalf("output = %q, want the resolved backend", rendered)
	}
	if !strings.Contains(rendered, "path=/tmp/inventory.sqlite") {
		t.Fatalf("output = %q, want the sqlite path", rendered)
	}
	if !strings.Contains(rendered, "warning") {
		t.Fatalf("output = %q, want a warning that this seeds the offline database", rendered)
	}
	if strings.Contains(rendered, "s3cr3t") {
		t.Fatalf("output = %q leaked the password", rendered)
	}

	out.Reset()
	AnnounceSeedTarget(&out, cfg, "postgres")
	rendered = out.String()
	if !strings.Contains(rendered, "backend=postgres") || !strings.Contains(rendered, "database=inventory_desktop") {
		t.Fatalf("output = %q, want the resolved postgres target", rendered)
	}
	if strings.Contains(rendered, "warning") {
		t.Fatalf("output = %q, want no sqlite warning for postgres", rendered)
	}
}

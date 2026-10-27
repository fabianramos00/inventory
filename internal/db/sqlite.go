package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	sqlitemigrations "inventory-desktop/migrations/sqlite"
)

func OpenSQLiteGORM(path string) (*gorm.DB, *sql.DB, error) {
	if err := rejectSymlink(path); err != nil {
		return nil, nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("secure sqlite directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite database: %w", err)
	}
	conn, err := gormDB.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("access sqlite connection: %w", err)
	}
	// ponytail: one connection avoids SQLITE_BUSY; raise when desktop concurrency needs it.
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	secureSQLiteFiles(path)
	return gormDB, conn, nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect sqlite database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sqlite database path %q must not be a symlink", path)
	}
	return nil
}

func secureSQLiteFiles(path string) {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Stat(candidate); err == nil {
			_ = os.Chmod(candidate, 0o600)
		}
	}
}

func MigrateSQLite(ctx context.Context, conn *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, sqlitemigrations.FS)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}
	return nil
}

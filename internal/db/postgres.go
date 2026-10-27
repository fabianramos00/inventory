package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	postgresmigrations "inventory-desktop/migrations/postgres"
)

func OpenPostgresGORM(dsn string) (*gorm.DB, *sql.DB, error) {
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:               logger.Default.LogMode(logger.Silent),
		DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open postgres: %w", err)
	}
	conn, err := gormDB.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("access postgres connection: %w", err)
	}
	return gormDB, conn, nil
}

func MigratePostgres(ctx context.Context, conn *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, postgresmigrations.FS)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/ports/outbound"
)

const postgresProbeTimeout = 3 * time.Second

type Stores struct {
	Users         outbound.UserRepository
	RefreshTokens outbound.RefreshTokenRepository
	Closer        func() error
	Backend       string
}

func Open(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Stores, error) {
	switch cfg.DBMode {
	case "postgres":
		return openPostgres(ctx, cfg, logger)
	case "sqlite":
		return openSQLite(ctx, cfg, logger)
	case "auto":
		probeCtx, cancel := context.WithTimeout(ctx, postgresProbeTimeout)
		gormDB, conn, err := db.OpenPostgresGORM(cfg.DatabaseURL)
		if err == nil {
			err = conn.PingContext(probeCtx)
		}
		cancel()
		if err == nil {
			return finishPostgres(ctx, cfg, logger, gormDB, conn)
		}
		if conn != nil {
			conn.Close()
		}
		logger.Warn("postgres unavailable, falling back to sqlite", "error", err, "path", cfg.SQLitePath)
		return openSQLite(ctx, cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported DB_MODE %q", cfg.DBMode)
	}
}

func openPostgres(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Stores, error) {
	gormDB, conn, err := db.OpenPostgresGORM(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return finishPostgres(ctx, cfg, logger, gormDB, conn)
}

func finishPostgres(ctx context.Context, cfg *config.Config, logger *slog.Logger, gormDB *gorm.DB, conn *sql.DB) (*Stores, error) {
	if cfg.AutoMigrate {
		if err := db.MigratePostgres(ctx, conn); err != nil {
			conn.Close()
			return nil, err
		}
	}
	logger.Info("database ready", "backend", "postgres")
	return &Stores{
		Users:         gormstore.NewUserStore(gormDB),
		RefreshTokens: gormstore.NewRefreshTokenStore(gormDB),
		Closer:        conn.Close,
		Backend:       "postgres",
	}, nil
}

func openSQLite(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Stores, error) {
	gormDB, conn, err := db.OpenSQLiteGORM(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}
	if cfg.AutoMigrate {
		if err := db.MigrateSQLite(ctx, conn); err != nil {
			conn.Close()
			return nil, err
		}
	}
	logger.Info("database ready", "backend", "sqlite", "path", cfg.SQLitePath)
	return &Stores{
		Users:         gormstore.NewUserStore(gormDB),
		RefreshTokens: gormstore.NewRefreshTokenStore(gormDB),
		Closer:        conn.Close,
		Backend:       "sqlite",
	}, nil
}

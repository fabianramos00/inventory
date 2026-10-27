package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"inventory-desktop/internal/adapters/persistence/postgres"
	"inventory-desktop/internal/adapters/persistence/sqlite"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/ports"
)

const postgresProbeTimeout = 3 * time.Second

type Stores struct {
	Users         ports.UserRepository
	RefreshTokens ports.RefreshTokenRepository
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
		pool, err := db.OpenPostgres(probeCtx, cfg.DatabaseURL)
		cancel()
		if err == nil {
			return finishPostgres(ctx, cfg, logger, pool)
		}
		logger.Warn("postgres unavailable, falling back to sqlite", "error", err, "path", cfg.SQLitePath)
		return openSQLite(ctx, cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported DB_MODE %q", cfg.DBMode)
	}
}

func openPostgres(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Stores, error) {
	pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	return finishPostgres(ctx, cfg, logger, pool)
}

func finishPostgres(ctx context.Context, cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool) (*Stores, error) {
	if cfg.AutoMigrate {
		if err := db.MigratePostgres(ctx, cfg.DatabaseURL); err != nil {
			pool.Close()
			return nil, err
		}
	}
	logger.Info("database ready", "backend", "postgres")
	return &Stores{
		Users:         postgres.NewUserRepository(pool),
		RefreshTokens: postgres.NewRefreshTokenRepository(pool),
		Closer: func() error {
			pool.Close()
			return nil
		},
		Backend: "postgres",
	}, nil
}

func openSQLite(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Stores, error) {
	conn, err := db.OpenSQLite(cfg.SQLitePath)
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
		Users:         sqlite.NewUserRepository(conn),
		RefreshTokens: sqlite.NewRefreshTokenRepository(conn),
		Closer:        conn.Close,
		Backend:       "sqlite",
	}, nil
}

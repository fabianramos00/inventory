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
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

const postgresProbeTimeout = 3 * time.Second

type Stores struct {
	Users            outbound.UserRepository
	RefreshTokens    outbound.RefreshTokenRepository
	Categories       outbound.CatalogRepository[domain.Category]
	Brands           outbound.CatalogRepository[domain.Brand]
	Materials        outbound.CatalogRepository[domain.Material]
	MeasurementUnits outbound.CatalogRepository[domain.MeasurementUnit]
	Products         outbound.ProductRepository
	Clients          outbound.ClientRepository
	Closer           func() error
	Backend          string
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
	return newStores(gormDB, conn, "postgres"), nil
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
	return newStores(gormDB, conn, "sqlite"), nil
}

func newStores(gormDB *gorm.DB, conn *sql.DB, backend string) *Stores {
	return &Stores{
		Users:            gormstore.NewUserStore(gormDB),
		RefreshTokens:    gormstore.NewRefreshTokenStore(gormDB),
		Categories:       gormstore.NewCategoryStore(gormDB),
		Brands:           gormstore.NewBrandStore(gormDB),
		Materials:        gormstore.NewMaterialStore(gormDB),
		MeasurementUnits: gormstore.NewMeasurementUnitStore(gormDB),
		Products:         gormstore.NewProductStore(gormDB),
		Clients:          gormstore.NewClientStore(gormDB),
		Closer:           conn.Close,
		Backend:          backend,
	}
}

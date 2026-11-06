package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

const postgresProbeTimeout = 3 * time.Second

// postgresPingTimeout bounds the explicit postgres connection attempt so a
// black-holed host cannot hang startup until the OS TCP timeout.
var postgresPingTimeout = 10 * time.Second

type Stores struct {
	Users            outbound.UserRepository
	RefreshTokens    outbound.RefreshTokenRepository
	Categories       outbound.CatalogRepository[domain.Category]
	Brands           outbound.CatalogRepository[domain.Brand]
	Materials        outbound.CatalogRepository[domain.Material]
	MeasurementUnits outbound.CatalogRepository[domain.MeasurementUnit]
	Products         outbound.ProductRepository
	Clients          outbound.ClientRepository
	Sales            outbound.SaleRepository
	Providers        outbound.ProviderRepository
	SupplierCatalogs outbound.SupplierCatalogRepository
	PurchaseOrders   outbound.PurchaseOrderRepository
	Dashboard        outbound.DashboardRepository
	UnitOfWork       outbound.UnitOfWork
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
		logger.Warn(
			"postgres unavailable, falling back to sqlite (set DB_MODE=postgres to fail hard)",
			append([]any{"error", err, "path", cfg.SQLitePath}, targetAttrs(cfg.DatabaseURL)...)...,
		)
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
	pingCtx, cancel := context.WithTimeout(ctx, postgresPingTimeout)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
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
	logger.Info("database ready", append([]any{"backend", "postgres"}, targetAttrs(cfg.DatabaseURL)...)...)
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

// targetAttrs describes the resolved postgres target without credentials.
func targetAttrs(dsn string) []any {
	target, ok := parsePostgresTarget(dsn)
	if !ok {
		return []any{"target", "unknown"}
	}
	attrs := []any{}
	if target.host != "" {
		attrs = append(attrs, "host", target.host)
	}
	if target.database != "" {
		attrs = append(attrs, "database", target.database)
	}
	return attrs
}

// postgresTarget is a credential-free description of a postgres DSN.
type postgresTarget struct {
	host     string
	database string
}

// parsePostgresTarget reads the host and database name from a URI DSN. It
// reports false for DSNs that are not URIs, so nothing but the generic
// marker is ever logged.
func parsePostgresTarget(dsn string) (postgresTarget, bool) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Host == "" {
		return postgresTarget{}, false
	}
	return postgresTarget{host: parsed.Host, database: strings.TrimPrefix(parsed.Path, "/")}, true
}

// DescribeTarget renders the resolved database target for CLI output without
// credentials.
func DescribeTarget(cfg *config.Config, backend string) string {
	if backend == "sqlite" {
		return "path=" + cfg.SQLitePath
	}
	target, ok := parsePostgresTarget(cfg.DatabaseURL)
	if !ok {
		return "target=unknown"
	}
	parts := []string{}
	if target.host != "" {
		parts = append(parts, "host="+target.host)
	}
	if target.database != "" {
		parts = append(parts, "database="+target.database)
	}
	return strings.Join(parts, " ")
}

// AnnounceSeedTarget prints the resolved database target before a seed run so
// writing the wrong database is self-evident.
func AnnounceSeedTarget(w io.Writer, cfg *config.Config, backend string) {
	fmt.Fprintf(w, "seeding backend=%s %s\n", backend, DescribeTarget(cfg, backend))
	if backend == "sqlite" {
		fmt.Fprintln(w, "warning: this seeds the local offline SQLite database; the app may be running on PostgreSQL")
	}
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
		Sales:            gormstore.NewSaleStore(gormDB),
		Providers:        gormstore.NewProviderStore(gormDB),
		SupplierCatalogs: gormstore.NewSupplierCatalogStore(gormDB),
		PurchaseOrders:   gormstore.NewPurchaseOrderStore(gormDB),
		Dashboard:        gormstore.NewDashboardStore(gormDB),
		UnitOfWork:       gormstore.NewUnitOfWork(gormDB),
		Closer:           conn.Close,
		Backend:          backend,
	}
}

package persistence_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/tests/testdb"
)

const truncateSaleConcurrencyTables = `TRUNCATE
	refresh_tokens, users, categories, materials, brands, measurement_units,
	products, price_history, clients, providers, supplier_catalogs,
	purchase_orders, order_items, sales, sale_items
	RESTART IDENTITY CASCADE`

func TestPostgresSalesCannotOversell(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the sale concurrency test")
	}

	ctx := context.Background()
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	gormDB, conn, err := db.OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer func() {
		if _, err := conn.ExecContext(ctx, truncateSaleConcurrencyTables); err != nil {
			t.Errorf("cleanup truncate: %v", err)
		}
	}()
	if err := conn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, truncateSaleConcurrencyTables); err != nil {
		t.Fatal(err)
	}

	categories := gormstore.NewCategoryStore(gormDB)
	brands := gormstore.NewBrandStore(gormDB)
	materials := gormstore.NewMaterialStore(gormDB)
	products := gormstore.NewProductStore(gormDB)
	users := gormstore.NewUserStore(gormDB)

	category, err := categories.Create(ctx, &domain.Category{Name: "Oversell Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := brands.Create(ctx, &domain.Brand{Name: "Oversell Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := materials.Create(ctx, &domain.Material{Name: "Oversell Material"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := products.Create(ctx, &domain.Product{
		SKU:               "OVERSELL-1",
		Name:              "Oversell Product",
		Price:             decimal.RequireFromString("10.00"),
		Cost:              decimal.RequireFromString("1.00"),
		StockQuantity:     1,
		LowStockThreshold: 0,
		BrandID:           brand.ID,
		CategoryID:        category.ID,
		MaterialID:        material.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{FullName: "Oversell User", Email: "oversell@example.com", HashedPassword: "hash", IsActive: true}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	service := application.NewSaleService(gormstore.NewSaleStore(gormDB), gormstore.NewUnitOfWork(gormDB))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.Create(ctx, inbound.CreateSaleInput{
				CreatedByID: user.ID,
				Items: []inbound.CreateSaleItemInput{{
					ProductID: product.ID,
					Quantity:  1,
				}},
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded := 0
	insufficient := 0
	for err := range results {
		var stockErr *domain.InsufficientStockError
		switch {
		case err == nil:
			succeeded++
		case errors.As(err, &stockErr):
			insufficient++
		default:
			t.Fatalf("unexpected sale error: %v", err)
		}
	}
	if succeeded != 1 || insufficient != 1 {
		t.Fatalf("sales succeeded=%d insufficient=%d, want 1/1", succeeded, insufficient)
	}

	var stock int
	if err := gormDB.Raw("SELECT stock_quantity FROM products WHERE id = ?", product.ID).Row().Scan(&stock); err != nil {
		t.Fatal(err)
	}
	if stock != 0 {
		t.Fatalf("final stock = %d, want 0", stock)
	}
	var sales int
	if err := gormDB.Raw("SELECT COUNT(*) FROM sales").Row().Scan(&sales); err != nil {
		t.Fatal(err)
	}
	if sales != 1 {
		t.Fatalf("sale rows = %d, want 1", sales)
	}
}

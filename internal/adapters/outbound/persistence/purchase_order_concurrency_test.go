package persistence_test

import (
	"context"
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

func TestPostgresReceivingOrderTwiceIncrementsStockOnce(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the purchase order concurrency test")
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
	if err := conn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `TRUNCATE
		refresh_tokens, users, categories, materials, brands, measurement_units,
		products, price_history, clients, providers, supplier_catalogs,
		purchase_orders, order_items, sales, sale_items
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	categories := gormstore.NewCategoryStore(gormDB)
	brands := gormstore.NewBrandStore(gormDB)
	materials := gormstore.NewMaterialStore(gormDB)
	products := gormstore.NewProductStore(gormDB)
	providers := gormstore.NewProviderStore(gormDB)
	users := gormstore.NewUserStore(gormDB)

	category, err := categories.Create(ctx, &domain.Category{Name: "Order Concurrency Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := brands.Create(ctx, &domain.Brand{Name: "Order Concurrency Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := materials.Create(ctx, &domain.Material{Name: "Order Concurrency Material"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := products.Create(ctx, &domain.Product{
		SKU:               "ORDER-CONCURRENCY-1",
		Name:              "Order Concurrency Product",
		Price:             decimal.RequireFromString("10.00"),
		Cost:              decimal.RequireFromString("1.00"),
		StockQuantity:     0,
		LowStockThreshold: 0,
		BrandID:           brand.ID,
		CategoryID:        category.ID,
		MaterialID:        material.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.Create(ctx, &domain.Provider{Name: "Order Concurrency Provider"})
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{FullName: "Order Concurrency User", Email: "order-concurrency@example.com", HashedPassword: "hash", IsActive: true}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	service := application.NewPurchaseOrderService(gormstore.NewPurchaseOrderStore(gormDB), gormstore.NewUnitOfWork(gormDB))
	order, err := service.Create(ctx, inbound.CreatePurchaseOrderInput{
		ProviderID:  provider.ID,
		CreatedByID: user.ID,
		Items:       []inbound.CreateOrderItemInput{{ProductID: product.ID, Quantity: 5, UnitCost: decimal.RequireFromString("2.50")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	received := domain.OrderStatusReceived
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.Update(ctx, order.ID, inbound.UpdatePurchaseOrderInput{
				Status: inbound.Optional[string]{Set: true, Value: &received},
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent receive error: %v", err)
		}
	}

	var stock int
	if err := gormDB.Raw("SELECT stock_quantity FROM products WHERE id = ?", product.ID).Row().Scan(&stock); err != nil {
		t.Fatal(err)
	}
	if stock != 5 {
		t.Fatalf("stock = %d, want exactly one increment to 5", stock)
	}
	var catalogs int
	if err := gormDB.Raw(
		"SELECT COUNT(*) FROM supplier_catalogs WHERE product_id = ? AND provider_id = ?",
		product.ID, provider.ID,
	).Row().Scan(&catalogs); err != nil {
		t.Fatal(err)
	}
	if catalogs != 1 {
		t.Fatalf("catalog rows = %d, want 1", catalogs)
	}
}

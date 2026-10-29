package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

// TestPostgresReceiveLocksOrderRow proves the concurrency test is not passing
// by accident. It holds the product stock row, starts a receive, and then probes
// whether the receive transaction holds the purchase-order row lock. Without
// GetForUpdate the probe succeeds because nothing locked the order before the
// stock wait, and the two-service race below would double-increment.
func TestPostgresReceiveLocksOrderRow(t *testing.T) {
	repos := openPostgresRepositories(t)
	ctx := context.Background()
	product, order, _, _ := seedPurchaseOrderForConcurrency(t, repos, "lock", 5)
	service := application.NewPurchaseOrderService(repos.purchaseOrders, repos.uow)

	sqlDB, err := repos.gorm.DB()
	if err != nil {
		t.Fatal(err)
	}
	stockConn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stockConn.Close()
	stockTx, err := stockConn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lockedStock int
	if err := stockTx.QueryRowContext(ctx, "SELECT stock_quantity FROM products WHERE id = $1 FOR UPDATE", product.ID).Scan(&lockedStock); err != nil {
		_ = stockTx.Rollback()
		t.Fatal(err)
	}

	received := domain.OrderStatusReceived
	done := make(chan error, 1)
	go func() {
		_, err := service.Update(ctx, order.ID, inbound.UpdatePurchaseOrderInput{
			Status: inbound.Optional[string]{Set: true, Value: &received},
		})
		done <- err
	}()

	probeConn, err := sqlDB.Conn(ctx)
	if err != nil {
		_ = stockTx.Rollback()
		t.Fatal(err)
	}
	defer probeConn.Close()
	orderLocked := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var id int64
		err := probeConn.QueryRowContext(
			ctx, "SELECT id FROM purchase_orders WHERE id = $1 FOR UPDATE NOWAIT", order.ID,
		).Scan(&id)
		if err != nil {
			orderLocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !orderLocked {
		_ = stockTx.Rollback()
		<-done
		t.Fatal("receive waited on product stock without locking the purchase order row")
	}

	if err := stockTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("receive after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("receive did not resume after the product lock was released")
	}
	if stock := qaStock(t, repos, product.ID); stock != 5 {
		t.Fatalf("stock = %d, want one increment to 5", stock)
	}
}

// Two independent service instances model two HTTP requests racing on the same
// order. Repeating the race makes an unlocked read-modify-write likely to show
// up instead of relying on a single lucky interleaving.
func TestPostgresConcurrentReceiveTwoServices(t *testing.T) {
	repos := openPostgresRepositories(t)
	ctx := context.Background()
	product, _, provider, user := seedPurchaseOrderForConcurrency(t, repos, "race", 5)

	setup := func() *application.PurchaseOrderService {
		return application.NewPurchaseOrderService(
			gormstore.NewPurchaseOrderStore(repos.gorm),
			gormstore.NewUnitOfWork(repos.gorm),
		)
	}
	services := [2]*application.PurchaseOrderService{setup(), setup()}
	received := domain.OrderStatusReceived
	for iteration := 1; iteration <= 3; iteration++ {
		order, err := services[0].Create(ctx, inbound.CreatePurchaseOrderInput{
			ProviderID:  provider.ID,
			CreatedByID: user.ID,
			Items:       []inbound.CreateOrderItemInput{{ProductID: product.ID, Quantity: 5, UnitCost: decimal.RequireFromString("2.50")}},
		})
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, service := range services {
			wg.Add(1)
			go func(service *application.PurchaseOrderService) {
				defer wg.Done()
				<-start
				_, err := service.Update(ctx, order.ID, inbound.UpdatePurchaseOrderInput{
					Status: inbound.Optional[string]{Set: true, Value: &received},
				})
				results <- err
			}(service)
		}
		close(start)
		wg.Wait()
		close(results)

		for err := range results {
			if err != nil {
				t.Fatalf("iteration %d: concurrent receive error: %v", iteration, err)
			}
		}
		if stock := qaStock(t, repos, product.ID); stock != iteration*5 {
			t.Fatalf("iteration %d: stock = %d, want %d", iteration, stock, iteration*5)
		}
	}

	var catalogs int
	if err := repos.gorm.Raw(
		"SELECT COUNT(*) FROM supplier_catalogs WHERE product_id = ?", product.ID,
	).Row().Scan(&catalogs); err != nil {
		t.Fatal(err)
	}
	if catalogs != 1 {
		t.Fatalf("catalog rows = %d, want 1", catalogs)
	}
}

func seedPurchaseOrderForConcurrency(t *testing.T, repos *repositories, suffix string, quantity int) (*domain.Product, *domain.PurchaseOrder, *domain.Provider, *domain.User) {
	t.Helper()
	ctx := context.Background()
	category, err := repos.categories.Create(ctx, &domain.Category{Name: "concurrency " + suffix + " category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "concurrency " + suffix + " brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "concurrency " + suffix + " material"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := repos.products.Create(ctx, &domain.Product{
		SKU:               "QA-CONCURRENCY-" + suffix,
		Name:              "QA concurrency " + suffix + " product",
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
	provider, err := repos.providers.Create(ctx, &domain.Provider{Name: "concurrency " + suffix + " provider"})
	if err != nil {
		t.Fatal(err)
	}
	user := createUser(t, repos, "concurrency-"+suffix+"@qa.test")

	order, err := application.NewPurchaseOrderService(repos.purchaseOrders, repos.uow).Create(ctx, inbound.CreatePurchaseOrderInput{
		ProviderID:  provider.ID,
		CreatedByID: user.ID,
		Items:       []inbound.CreateOrderItemInput{{ProductID: product.ID, Quantity: quantity, UnitCost: decimal.RequireFromString("2.50")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return product, order, provider, user
}

func qaStock(t *testing.T, repos *repositories, productID int64) int {
	t.Helper()
	var stock int
	if err := repos.gorm.Raw("SELECT stock_quantity FROM products WHERE id = ?", productID).Row().Scan(&stock); err != nil {
		t.Fatal(err)
	}
	return stock
}

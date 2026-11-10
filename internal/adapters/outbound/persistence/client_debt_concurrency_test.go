package persistence_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/tests/testdb"
)

// Client lock ordering must survive concurrent payments and a payment racing
// an item edit: no overpayment and no stale debt cache.
func TestClientDebtConcurrencyKeepsCacheConsistent(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the client debt concurrency test")
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
	clients := gormstore.NewClientStore(gormDB)
	users := gormstore.NewUserStore(gormDB)

	category, err := categories.Create(ctx, &domain.Category{Name: "Client Debt Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := brands.Create(ctx, &domain.Brand{Name: "Client Debt Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := materials.Create(ctx, &domain.Material{Name: "Client Debt Material"})
	if err != nil {
		t.Fatal(err)
	}
	newProduct := func(sku, price string) *domain.Product {
		t.Helper()
		product, err := products.Create(ctx, &domain.Product{
			SKU:               sku,
			Name:              sku,
			Price:             decimal.RequireFromString(price),
			Cost:              decimal.RequireFromString("1.00"),
			StockQuantity:     1000,
			LowStockThreshold: 0,
			BrandID:           brand.ID,
			CategoryID:        category.ID,
			MaterialID:        material.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return product
	}
	big := newProduct("CLIENT-DEBT-BIG", "100.00")
	small := newProduct("CLIENT-DEBT-SMALL", "20.00")

	user := &domain.User{FullName: "Debt Concurrency User", Email: "debt-concurrency@example.com", HashedPassword: "hash", IsActive: true}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	newClient := func(name string) *domain.Client {
		t.Helper()
		client, err := clients.Create(ctx, &domain.Client{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	service := application.NewSaleService(gormstore.NewSaleStore(gormDB), gormstore.NewUnitOfWork(gormDB))

	createSale := func(t *testing.T, clientID, productID int64, price string) *domain.Sale {
		t.Helper()
		unitPrice := decimal.RequireFromString(price)
		sale, err := service.Create(ctx, inbound.CreateSaleInput{
			ClientID:    &clientID,
			CreatedByID: user.ID,
			Items: []inbound.CreateSaleItemInput{{
				ProductID: productID,
				Quantity:  1,
				UnitPrice: &unitPrice,
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return sale
	}
	assertCacheMatchesDerived := func(t *testing.T, clientID int64, want string) {
		t.Helper()
		expected := decimal.RequireFromString(want)
		client, err := clients.Get(ctx, clientID)
		if err != nil {
			t.Fatal(err)
		}
		if !client.DebtAmount.Equal(expected) {
			t.Fatalf("cached debt = %s, want %s", client.DebtAmount, expected)
		}
		var text string
		if err := gormDB.Raw(
			`SELECT COALESCE(SUM(GREATEST(total_amount - amount_paid, 0)), 0)::text
			 FROM sales WHERE client_id = ?`, clientID,
		).Row().Scan(&text); err != nil {
			t.Fatal(err)
		}
		if derived := decimal.RequireFromString(text); !derived.Equal(client.DebtAmount) {
			t.Fatalf("derived debt = %s, cached = %s", derived, client.DebtAmount)
		}
	}

	t.Run("concurrent client payments cannot overpay", func(t *testing.T) {
		client := newClient("Concurrent Payment Client")
		createSale(t, client.ID, big.ID, "100.00")
		createSale(t, client.ID, big.ID, "100.00")

		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := service.CreateClientPayment(ctx, inbound.CreateClientPaymentInput{
					ClientID:      client.ID,
					Amount:        decimal.RequireFromString("150.00"),
					PaymentMethod: domain.PaymentMethodCash,
					CreatedByID:   user.ID,
				})
				results <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		succeeded, rejected := 0, 0
		for err := range results {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, domain.ErrValidation):
				rejected++
			default:
				t.Fatalf("unexpected payment error: %v", err)
			}
		}
		if succeeded != 1 || rejected != 1 {
			t.Fatalf("payments succeeded=%d rejected=%d, want 1/1", succeeded, rejected)
		}
		assertCacheMatchesDerived(t, client.ID, "50.00")

		var payments int
		var paidText string
		if err := gormDB.Raw(
			`SELECT COUNT(*), COALESCE(SUM(amount), 0)::text FROM sale_payments
			 JOIN sales ON sales.id = sale_payments.sale_id WHERE sales.client_id = ?`, client.ID,
		).Row().Scan(&payments, &paidText); err != nil {
			t.Fatal(err)
		}
		if payments != 2 {
			t.Fatalf("payment rows = %d, want 2 (one per allocated sale)", payments)
		}
		if paid := decimal.RequireFromString(paidText); !paid.Equal(decimal.RequireFromString("150.00")) {
			t.Fatalf("payments sum = %s, want 150.00", paid)
		}
	})

	t.Run("payment racing an item edit keeps the cache correct", func(t *testing.T) {
		client := newClient("Payment Item Edit Client")
		sale := createSale(t, client.ID, big.ID, "100.00")

		start := make(chan struct{})
		errorsCh := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.CreateClientPayment(ctx, inbound.CreateClientPaymentInput{
				ClientID:      client.ID,
				Amount:        decimal.RequireFromString("100.00"),
				PaymentMethod: domain.PaymentMethodCash,
				CreatedByID:   user.ID,
			})
			errorsCh <- err
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			unitPrice := decimal.RequireFromString("20.00")
			_, err := service.AddItem(ctx, sale.ID, inbound.AddSaleItemInput{
				ProductID: small.ID,
				Quantity:  1,
				UnitPrice: &unitPrice,
			})
			errorsCh <- err
		}()
		close(start)
		wg.Wait()
		close(errorsCh)

		for err := range errorsCh {
			if err != nil {
				t.Fatalf("concurrent operation failed: %v", err)
			}
		}
		updated, err := service.Get(ctx, sale.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !updated.TotalAmount.Equal(decimal.RequireFromString("120.00")) ||
			!updated.AmountPaid.Equal(decimal.RequireFromString("100.00")) ||
			!updated.DebtAmount.Equal(decimal.RequireFromString("20.00")) {
			t.Fatalf("sale after race = total %s paid %s debt %s, want 120/100/20",
				updated.TotalAmount, updated.AmountPaid, updated.DebtAmount)
		}
		assertCacheMatchesDerived(t, client.ID, "20.00")
	})

	t.Run("client reassignment racing a payment keeps both caches correct", func(t *testing.T) {
		fromClient := newClient("Reassign Race From")
		toClient := newClient("Reassign Race To")
		movedSale := createSale(t, fromClient.ID, big.ID, "100.00")
		otherSale := createSale(t, fromClient.ID, big.ID, "50.00")

		assertCacheEqualsDerived := func(t *testing.T, clientID int64) {
			t.Helper()
			client, err := clients.Get(ctx, clientID)
			if err != nil {
				t.Fatalf("get client %d: %v", clientID, err)
			}
			var text string
			if err := gormDB.Raw(
				`SELECT COALESCE(SUM(GREATEST(total_amount - amount_paid, 0)), 0)::text
				 FROM sales WHERE client_id = ?`, clientID,
			).Row().Scan(&text); err != nil {
				t.Fatal(err)
			}
			derived := decimal.RequireFromString(text)
			if !client.DebtAmount.Equal(derived) {
				t.Fatalf("client %d cached debt = %s, derived = %s", clientID, client.DebtAmount, derived)
			}
		}

		raceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		start := make(chan struct{})
		var wg sync.WaitGroup
		var reassignErr, paymentErr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, reassignErr = service.Update(raceCtx, movedSale.ID, inbound.UpdateSaleInput{
				ClientID: inbound.Optional[int64]{Set: true, Value: &toClient.ID},
			})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, paymentErr = service.CreateClientPayment(raceCtx, inbound.CreateClientPaymentInput{
				ClientID:      fromClient.ID,
				Amount:        decimal.RequireFromString("120.00"),
				PaymentMethod: domain.PaymentMethodCash,
				CreatedByID:   user.ID,
			})
		}()
		close(start)
		wg.Wait()

		if reassignErr != nil {
			t.Fatalf("concurrent reassignment failed: %v", reassignErr)
		}
		if paymentErr != nil && !errors.Is(paymentErr, domain.ErrValidation) {
			t.Fatalf("concurrent payment failed with an unexpected error: %v", paymentErr)
		}

		moved, err := service.Get(ctx, movedSale.ID)
		if err != nil {
			t.Fatal(err)
		}
		if moved.ClientID == nil || *moved.ClientID != toClient.ID {
			t.Fatalf("moved sale client = %v, want %d", moved.ClientID, toClient.ID)
		}

		var paidText string
		if err := gormDB.Raw(
			`SELECT COALESCE(SUM(amount), 0)::text FROM sale_payments WHERE sale_id IN (?, ?)`,
			movedSale.ID, otherSale.ID,
		).Row().Scan(&paidText); err != nil {
			t.Fatal(err)
		}
		paid := decimal.RequireFromString(paidText)
		if paymentErr == nil {
			if !paid.Equal(decimal.RequireFromString("120.00")) {
				t.Fatalf("succeeded payment sum = %s, want 120.00", paid)
			}
		} else if !paid.IsZero() {
			t.Fatalf("rejected payment wrote %s to the ledger", paid)
		}

		for _, saleID := range []int64{movedSale.ID, otherSale.ID} {
			sale, err := service.Get(ctx, saleID)
			if err != nil {
				t.Fatal(err)
			}
			if sale.AmountPaid.GreaterThan(sale.TotalAmount) {
				t.Fatalf("sale %d overpaid: paid %s total %s", sale.ID, sale.AmountPaid, sale.TotalAmount)
			}
			if sale.DebtAmount.IsNegative() {
				t.Fatalf("sale %d has negative debt %s", sale.ID, sale.DebtAmount)
			}
		}

		assertCacheEqualsDerived(t, fromClient.ID)
		assertCacheEqualsDerived(t, toClient.ID)
	})
}

package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

func saleDecimal(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}

func saleString(value string) *string { return &value }

func testSales(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()

	category, err := repos.categories.Create(ctx, &domain.Category{Name: "Sales Contract Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "Sales Contract Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "Sales Contract Material"})
	if err != nil {
		t.Fatal(err)
	}
	user := createUser(t, repos, "sales-contract@contract.test")
	client, err := repos.clients.Create(ctx, &domain.Client{
		Name:      "Sales Contract Client",
		CreatedAt: time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	newProduct := func(sku, name, price string, stock int) *domain.Product {
		t.Helper()
		product, err := repos.products.Create(ctx, &domain.Product{
			SKU:               sku,
			Name:              name,
			Price:             saleDecimal(price),
			Cost:              saleDecimal("1.00"),
			StockQuantity:     stock,
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
	hammer := newProduct("SALES-CONTRACT-1", "Contract Hammer", "10.50", 10)
	nails := newProduct("SALES-CONTRACT-2", "Contract Nails", "0.20", 10)
	var pendingSaleID int64
	// Registered on the parent *testing.T so the pending sale survives every
	// nested subtest (stats_exact depends on it) and is only removed at the end.
	t.Cleanup(func() {
		if pendingSaleID == 0 {
			return
		}
		_ = repos.sales.DeleteItems(context.Background(), pendingSaleID)
		_ = repos.sales.Delete(context.Background(), pendingSaleID)
	})

	before, err := repos.sales.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.UnpaidCount != 0 || before.UndeliveredCount != 0 ||
		!before.TotalAmountSum.IsZero() || !before.AmountPaidSum.IsZero() {
		t.Fatalf("empty stats = %+v, want all zeros", before)
	}

	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	sale := &domain.Sale{
		CreatedAt:      createdAt,
		TotalAmount:    saleDecimal("21.20"),
		AmountPaid:     saleDecimal("5.00"),
		DebtAmount:     saleDecimal("16.20"),
		PaymentStatus:  domain.PaymentStatusPartial,
		DeliveryStatus: domain.DeliveryStatusPartial,
		PaymentMethod:  saleString(domain.PaymentMethodCash),
		ClientID:       &client.ID,
		CreatedByID:    user.ID,
	}
	items := []*domain.SaleItem{
		{ProductID: hammer.ID, Quantity: 2, DeliveredQuantity: 0, UnitPrice: saleDecimal("10.50"), Subtotal: saleDecimal("21.00")},
		{ProductID: nails.ID, Quantity: 1, DeliveredQuantity: 1, UnitPrice: saleDecimal("0.20"), Subtotal: saleDecimal("0.20")},
	}
	created, err := repos.sales.Create(ctx, sale, items)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= 0 {
		t.Fatalf("Create returned id %d, want a positive id", created.ID)
	}

	t.Run("get relations statuses and money", func(t *testing.T) {
		got, err := repos.sales.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.TotalAmount.Equal(saleDecimal("21.20")) || !got.AmountPaid.Equal(saleDecimal("5.00")) || !got.DebtAmount.Equal(saleDecimal("16.20")) {
			t.Fatalf("money = %s/%s/%s", got.TotalAmount, got.AmountPaid, got.DebtAmount)
		}
		if got.PaymentStatus != domain.PaymentStatusPartial || got.DeliveryStatus != domain.DeliveryStatusPartial {
			t.Fatalf("statuses = %q/%q", got.PaymentStatus, got.DeliveryStatus)
		}
		if got.PaymentMethod == nil || *got.PaymentMethod != domain.PaymentMethodCash {
			t.Fatalf("payment method = %v", got.PaymentMethod)
		}
		if !got.CreatedAt.Equal(createdAt) || got.UpdatedAt != nil {
			t.Fatalf("timestamps = %v/%v, want %v/nil", got.CreatedAt, got.UpdatedAt, createdAt)
		}
		if got.CreatedBy == nil || got.CreatedBy.ID != user.ID || got.CreatedBy.Email != user.Email || got.CreatedBy.HashedPassword != user.HashedPassword {
			t.Fatalf("created_by = %+v", got.CreatedBy)
		}
		if got.Client == nil || got.Client.ID != client.ID || got.Client.Name != client.Name {
			t.Fatalf("client = %+v", got.Client)
		}
		if got.ClientID == nil || *got.ClientID != client.ID {
			t.Fatalf("client_id = %v", got.ClientID)
		}

		var rawStatus, rawDelivery, rawMethod string
		if err := repos.gorm.Raw(
			"SELECT payment_status, delivery_status, payment_method FROM sales WHERE id = ?", created.ID,
		).Row().Scan(&rawStatus, &rawDelivery, &rawMethod); err != nil {
			t.Fatal(err)
		}
		if rawStatus != "PARTIAL" {
			t.Fatalf("stored payment status = %q, want PARTIAL", rawStatus)
		}
		if rawDelivery != "PARTIAL" {
			t.Fatalf("stored delivery status = %q, want PARTIAL", rawDelivery)
		}
		if rawMethod != "CASH" {
			t.Fatalf("stored payment method = %q, want CASH", rawMethod)
		}

		// PostgreSQL keeps NUMERIC text; SQLite keeps integer cents (ADR-0016).
		if repos.gorm.Dialector.Name() == "sqlite" {
			var cents int64
			if err := repos.gorm.Raw("SELECT total_amount FROM sales WHERE id = ?", created.ID).Row().Scan(&cents); err != nil {
				t.Fatal(err)
			}
			if cents != 2120 {
				t.Fatalf("stored sqlite total_amount = %d cents, want 2120", cents)
			}
		} else {
			var text string
			if err := repos.gorm.Raw("SELECT total_amount::text FROM sales WHERE id = ?", created.ID).Row().Scan(&text); err != nil {
				t.Fatal(err)
			}
			if text != "21.20" {
				t.Fatalf("stored postgres total_amount = %q, want 21.20", text)
			}
		}

		if _, err := repos.sales.Get(ctx, created.ID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get missing sale error = %v, want ErrNotFound", err)
		}
	})

	t.Run("items order names and money", func(t *testing.T) {
		list, hasNext, err := repos.sales.ListItems(ctx, created.ID, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !hasNext || len(list) != 1 {
			t.Fatalf("first item page = %+v has_next=%v", list, hasNext)
		}
		all, err := repos.sales.ListAllItems(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 2 || all[0].ID >= all[1].ID {
			t.Fatalf("items = %+v, want id ASC", all)
		}
		if all[0].ProductName != hammer.Name || all[0].ProductSKU != hammer.SKU ||
			all[0].BrandName == nil || *all[0].BrandName != brand.Name ||
			all[0].MaterialName == nil || *all[0].MaterialName != material.Name ||
			all[0].CategoryName == nil || *all[0].CategoryName != category.Name {
			t.Fatalf("item names = %+v", all[0])
		}
		if !all[0].UnitPrice.Equal(saleDecimal("10.50")) || !all[0].Subtotal.Equal(saleDecimal("21.00")) ||
			!all[1].UnitPrice.Equal(saleDecimal("0.20")) || !all[1].Subtotal.Equal(saleDecimal("0.20")) {
			t.Fatalf("item money = %+v/%+v", all[0], all[1])
		}

		byProduct, err := repos.sales.GetItemByProduct(ctx, created.ID, nails.ID)
		if err != nil {
			t.Fatal(err)
		}
		if byProduct.ID != all[1].ID {
			t.Fatalf("item by product = %+v, want %+v", byProduct, all[1])
		}
		if _, err := repos.sales.GetItem(ctx, created.ID, all[1].ID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing item error = %v, want ErrNotFound", err)
		}
	})

	t.Run("update sale status and timestamps", func(t *testing.T) {
		got, err := repos.sales.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		updatedAt := time.Date(2026, time.January, 2, 4, 5, 6, 654321000, time.UTC)
		got.AmountPaid = saleDecimal("21.20")
		got.PaymentStatus = domain.PaymentStatusPaid
		got.DebtAmount = decimal.Zero
		got.UpdatedAt = &updatedAt
		if err := repos.sales.Update(ctx, created.ID, got); err != nil {
			t.Fatal(err)
		}
		again, err := repos.sales.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if again.PaymentStatus != domain.PaymentStatusPaid || !again.DebtAmount.IsZero() {
			t.Fatalf("updated status = %q debt %s", again.PaymentStatus, again.DebtAmount)
		}
		if again.UpdatedAt == nil || !again.UpdatedAt.Equal(updatedAt) {
			t.Fatalf("updated_at = %v, want %v", again.UpdatedAt, updatedAt)
		}
		if err := repos.sales.Update(ctx, created.ID+1000, got); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing sale error = %v, want ErrNotFound", err)
		}
	})

	t.Run("filters ordering and pagination", func(t *testing.T) {
		second, err := repos.sales.Create(ctx, &domain.Sale{
			CreatedAt:      time.Date(2026, time.January, 3, 3, 4, 5, 0, time.UTC),
			TotalAmount:    saleDecimal("3.00"),
			AmountPaid:     decimal.Zero,
			PaymentStatus:  domain.PaymentStatusPending,
			DeliveryStatus: domain.DeliveryStatusPending,
			CreatedByID:    user.ID,
		}, []*domain.SaleItem{
			{ProductID: nails.ID, Quantity: 15, UnitPrice: saleDecimal("0.20"), Subtotal: saleDecimal("3.00")},
		})
		if err != nil {
			t.Fatal(err)
		}
		pendingSaleID = second.ID

		list, hasNext, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(list) < 2 || list[0].ID != second.ID {
			t.Fatalf("sales list = %+v has_next=%v, want id DESC starting %d", list, hasNext, second.ID)
		}

		noClient, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, NoClient: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(noClient) != 1 || noClient[0].ID != second.ID {
			t.Fatalf("no_client = %+v, want only %d", noClient, second.ID)
		}

		byClient, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, ClientID: &client.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(byClient) != 1 || byClient[0].ID != created.ID {
			t.Fatalf("client filter = %+v, want only %d", byClient, created.ID)
		}

		byUser, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, UserID: &user.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(byUser) != 2 {
			t.Fatalf("user filter returned %d sales, want 2", len(byUser))
		}

		paid, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, PaymentStatus: domain.PaymentStatusPaid})
		if err != nil {
			t.Fatal(err)
		}
		if len(paid) != 1 || paid[0].ID != created.ID {
			t.Fatalf("paid filter = %+v", paid)
		}

		delivered, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, DeliveryStatus: domain.DeliveryStatusPartial})
		if err != nil {
			t.Fatal(err)
		}
		if len(delivered) != 1 || delivered[0].ID != created.ID {
			t.Fatalf("delivery filter = %+v", delivered)
		}

		cash, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, PaymentMethod: domain.PaymentMethodCash})
		if err != nil {
			t.Fatal(err)
		}
		if len(cash) != 1 || cash[0].ID != created.ID {
			t.Fatalf("cash filter = %+v", cash)
		}

		start := time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, time.January, 3, 23, 59, 59, 0, time.UTC)
		inRange, _, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 100, StartDate: &start, EndDate: &end})
		if err != nil {
			t.Fatal(err)
		}
		if len(inRange) != 1 || inRange[0].ID != second.ID {
			t.Fatalf("date filter = %+v", inRange)
		}

		page, hasNext, err := repos.sales.List(ctx, outbound.SaleFilter{Limit: 1, Skip: 1})
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(page) != 1 || page[0].ID != created.ID {
			t.Fatalf("page = %+v has_next=%v, want one sale and false", page, hasNext)
		}
	})

	t.Run("item mutations and delete", func(t *testing.T) {
		sale, err := repos.sales.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		all, err := repos.sales.ListAllItems(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		item := all[0]
		item.Quantity = 3
		item.DeliveredQuantity = 3
		item.Subtotal = saleDecimal("31.50")
		if err := repos.sales.UpdateItem(ctx, item); err != nil {
			t.Fatal(err)
		}
		reloaded, err := repos.sales.GetItem(ctx, created.ID, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Quantity != 3 || reloaded.DeliveredQuantity != 3 || !reloaded.Subtotal.Equal(saleDecimal("31.50")) {
			t.Fatalf("updated item = %+v", reloaded)
		}
		if err := repos.sales.DeleteItem(ctx, created.ID, all[1].ID); err != nil {
			t.Fatal(err)
		}
		remaining, err := repos.sales.ListAllItems(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(remaining) != 1 || remaining[0].ID != item.ID {
			t.Fatalf("remaining items = %+v", remaining)
		}

		sale.TotalAmount = saleDecimal("31.50")
		if err := repos.sales.Update(ctx, created.ID, sale); err != nil {
			t.Fatal(err)
		}
		if err := repos.sales.DeleteItems(ctx, created.ID); err != nil {
			t.Fatal(err)
		}
		if err := repos.sales.Delete(ctx, created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repos.sales.Get(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("deleted sale Get error = %v, want ErrNotFound", err)
		}
		if err := repos.sales.Delete(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("second delete error = %v, want ErrNotFound", err)
		}
	})

	t.Run("stats exact", func(t *testing.T) {
		after, err := repos.sales.Stats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// The first sale was deleted above; the pending sale remains.
		if after.UnpaidCount-before.UnpaidCount != 1 {
			t.Fatalf("unpaid delta = %d, want 1", after.UnpaidCount-before.UnpaidCount)
		}
		if after.UndeliveredCount-before.UndeliveredCount != 1 {
			t.Fatalf("undelivered delta = %d, want 1", after.UndeliveredCount-before.UndeliveredCount)
		}
		wantTotal := before.TotalAmountSum.Add(saleDecimal("3.00"))
		if !after.TotalAmountSum.Equal(wantTotal) {
			t.Fatalf("total sum = %s, want %s", after.TotalAmountSum, wantTotal)
		}
		if !after.AmountPaidSum.Equal(before.AmountPaidSum) {
			t.Fatalf("paid sum = %s, want %s", after.AmountPaidSum, before.AmountPaidSum)
		}
	})

	t.Run("unit of work stock and rollback", func(t *testing.T) {
		var locked map[int64]int64
		err := repos.uow.WithinTx(ctx, func(ctx context.Context, tx outbound.TxRepos) error {
			var err error
			locked, err = tx.Stock.LockForUpdate(ctx, []int64{nails.ID, hammer.ID})
			if err != nil {
				return err
			}
			if err := tx.Stock.ApplyDelta(ctx, hammer.ID, -2); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if locked[hammer.ID] != 10 || locked[nails.ID] != 10 {
			t.Fatalf("locked quantities = %+v, want 10/10", locked)
		}
		got, err := repos.products.Get(ctx, hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.StockQuantity != 8 {
			t.Fatalf("stock after commit = %d, want 8", got.StockQuantity)
		}

		forced := errors.New("forced rollback")
		err = repos.uow.WithinTx(ctx, func(ctx context.Context, tx outbound.TxRepos) error {
			if err := tx.Stock.ApplyDelta(ctx, hammer.ID, -3); err != nil {
				return err
			}
			return forced
		})
		if !errors.Is(err, forced) {
			t.Fatalf("rollback error = %v, want %v", err, forced)
		}
		got, err = repos.products.Get(ctx, hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.StockQuantity != 8 {
			t.Fatalf("stock after rollback = %d, want 8", got.StockQuantity)
		}

		if err := repos.stock.ApplyDelta(ctx, hammer.ID, -999); !errors.Is(err, domain.ErrInsufficientStock) {
			t.Fatalf("insufficient stock error = %v, want ErrInsufficientStock", err)
		}
	})
}

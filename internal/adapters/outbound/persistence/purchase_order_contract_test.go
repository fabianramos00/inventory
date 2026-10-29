package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

func testPurchaseOrders(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()
	service := application.NewPurchaseOrderService(repos.purchaseOrders, repos.uow)

	category, err := repos.categories.Create(ctx, &domain.Category{Name: "Orders Contract Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "Orders Contract Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "Orders Contract Material"})
	if err != nil {
		t.Fatal(err)
	}
	createProduct := func(sku, name, price string, stock int) *domain.Product {
		t.Helper()
		product, err := repos.products.Create(ctx, &domain.Product{
			SKU:               sku,
			Name:              name,
			Price:             decimal.RequireFromString(price),
			Cost:              decimal.RequireFromString("1.00"),
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
	hammer := createProduct("ORDERS-CONTRACT-1", "Contract Order Hammer", "10.50", 10)
	nails := createProduct("ORDERS-CONTRACT-2", "Contract Order Nails", "0.20", 10)
	screwdriver := createProduct("ORDERS-CONTRACT-3", "Contract Order Screwdriver", "1.50", 10)

	provider, err := repos.providers.Create(ctx, &domain.Provider{Name: "Orders Contract Provider"})
	if err != nil {
		t.Fatal(err)
	}
	otherProvider, err := repos.providers.Create(ctx, &domain.Provider{Name: "Orders Contract Provider Two"})
	if err != nil {
		t.Fatal(err)
	}
	user := createUser(t, repos, "orders-contract@contract.test")
	sku := "CONTRACT-SKU"

	order, err := service.Create(ctx, inbound.CreatePurchaseOrderInput{
		ProviderID:  provider.ID,
		CreatedByID: user.ID,
		Items: []inbound.CreateOrderItemInput{
			{ProductID: hammer.ID, Quantity: 2, UnitCost: decimal.RequireFromString("10.25"), SupplierSKU: &sku},
			{ProductID: nails.ID, Quantity: 1, UnitCost: decimal.RequireFromString("0.50")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("get money timestamp and provider", func(t *testing.T) {
		got, err := repos.purchaseOrders.Get(ctx, order.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.TotalAmount.Equal(decimal.RequireFromString("21.00")) {
			t.Fatalf("total = %s, want 21.00", got.TotalAmount)
		}
		if got.Status != domain.OrderStatusPending || got.PaymentStatus != domain.PurchasePaymentStatusPending {
			t.Fatalf("statuses = %q/%q", got.Status, got.PaymentStatus)
		}
		if got.CreatedByID != user.ID || got.ProviderID != provider.ID {
			t.Fatalf("ids = %d/%d", got.CreatedByID, got.ProviderID)
		}
		if got.Provider == nil || got.Provider.ID != provider.ID || got.Provider.Name != provider.Name {
			t.Fatalf("provider = %+v", got.Provider)
		}
		if got.OrderDate.IsZero() || time.Since(got.OrderDate) > time.Minute {
			t.Fatalf("order_date = %v, want now", got.OrderDate)
		}
		if _, err := repos.purchaseOrders.Get(ctx, order.ID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing order error = %v, want ErrNotFound", err)
		}

		var rawStatus, rawPayment string
		if err := repos.gorm.Raw(
			"SELECT status, payment_status FROM purchase_orders WHERE id = ?", order.ID,
		).Row().Scan(&rawStatus, &rawPayment); err != nil {
			t.Fatal(err)
		}
		if rawStatus != "PENDING" || rawPayment != "PENDING" {
			t.Fatalf("stored statuses = %q/%q, want PENDING/PENDING", rawStatus, rawPayment)
		}
		if repos.gorm.Dialector.Name() == "sqlite" {
			var cents int64
			if err := repos.gorm.Raw("SELECT total_amount FROM purchase_orders WHERE id = ?", order.ID).Row().Scan(&cents); err != nil {
				t.Fatal(err)
			}
			if cents != 2100 {
				t.Fatalf("sqlite total = %d cents, want 2100", cents)
			}
		} else {
			var text string
			if err := repos.gorm.Raw("SELECT total_amount::text FROM purchase_orders WHERE id = ?", order.ID).Row().Scan(&text); err != nil {
				t.Fatal(err)
			}
			if text != "21.00" {
				t.Fatalf("postgres total = %q, want 21.00", text)
			}
		}
	})

	t.Run("items order and names", func(t *testing.T) {
		items, hasNext, err := repos.purchaseOrders.ListItems(ctx, order.ID, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !hasNext || len(items) != 1 {
			t.Fatalf("first page = %+v has_next=%v", items, hasNext)
		}
		all, err := repos.purchaseOrders.ListAllItems(ctx, order.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 2 || all[0].ID >= all[1].ID {
			t.Fatalf("items = %+v, want id ASC", all)
		}
		first := all[0]
		if first.ProductName != hammer.Name || first.ProductSKU != hammer.SKU ||
			first.BrandName == nil || *first.BrandName != brand.Name ||
			first.MaterialName == nil || *first.MaterialName != material.Name ||
			first.CategoryName == nil || *first.CategoryName != category.Name {
			t.Fatalf("item names = %+v", first)
		}
		if !first.UnitCost.Equal(decimal.RequireFromString("10.25")) || !first.Subtotal.Equal(decimal.RequireFromString("20.50")) {
			t.Fatalf("item money = %+v", first)
		}
		if first.SupplierSKU == nil || *first.SupplierSKU != sku {
			t.Fatalf("supplier sku = %v", first.SupplierSKU)
		}
		byProduct, err := repos.purchaseOrders.GetItemByProduct(ctx, order.ID, nails.ID)
		if err != nil || byProduct.ID != all[1].ID {
			t.Fatalf("item by product = %+v / %v", byProduct, err)
		}
		if _, err := repos.purchaseOrders.GetItem(ctx, order.ID, all[1].ID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing item error = %v, want ErrNotFound", err)
		}
	})

	t.Run("item mutations recalculate totals", func(t *testing.T) {
		added, err := service.AddItem(ctx, order.ID, inbound.AddOrderItemInput{
			ProductID: screwdriver.ID, Quantity: 2, UnitCost: decimal.RequireFromString("1.50"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !added.TotalAmount.Equal(decimal.RequireFromString("24.00")) {
			t.Fatalf("total after add = %s, want 24.00", added.TotalAmount)
		}
		if _, err := service.AddItem(ctx, order.ID, inbound.AddOrderItemInput{
			ProductID: hammer.ID, Quantity: 1, UnitCost: decimal.RequireFromString("1"),
		}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate error = %v, want conflict", err)
		}
		items, err := service.ListItems(ctx, order.ID, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		var addedID int64
		for _, item := range items.Items {
			if item.ProductID == screwdriver.ID {
				addedID = item.ID
			}
		}
		if addedID == 0 {
			t.Fatal("added item not found")
		}
		updated, err := service.UpdateItem(ctx, order.ID, addedID, inbound.UpdateOrderItemInput{
			Quantity: inbound.Optional[int]{Set: true, Value: contractInt(1)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !updated.TotalAmount.Equal(decimal.RequireFromString("22.50")) {
			t.Fatalf("total after update = %s, want 22.50", updated.TotalAmount)
		}
		deleted, err := service.DeleteItem(ctx, order.ID, addedID)
		if err != nil {
			t.Fatal(err)
		}
		if !deleted.TotalAmount.Equal(decimal.RequireFromString("21.00")) {
			t.Fatalf("total after delete = %s, want 21.00", deleted.TotalAmount)
		}
	})

	t.Run("filters and pagination", func(t *testing.T) {
		second, err := service.Create(ctx, inbound.CreatePurchaseOrderInput{
			ProviderID:    otherProvider.ID,
			Status:        domain.OrderStatusSent,
			PaymentStatus: domain.PurchasePaymentStatusPaid,
			CreatedByID:   user.ID,
			Items:         []inbound.CreateOrderItemInput{{ProductID: nails.ID, Quantity: 1, UnitCost: decimal.RequireFromString("0.20")}},
		})
		if err != nil {
			t.Fatal(err)
		}

		list, hasNext, err := repos.purchaseOrders.List(ctx, outbound.PurchaseOrderFilter{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(list) < 2 || list[0].ID != second.ID {
			t.Fatalf("orders list = %+v has_next=%v, want id DESC starting %d", list, hasNext, second.ID)
		}

		byProvider, _, err := repos.purchaseOrders.List(ctx, outbound.PurchaseOrderFilter{Limit: 100, ProviderID: &provider.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(byProvider) != 1 || byProvider[0].ID != order.ID {
			t.Fatalf("provider filter = %+v", byProvider)
		}

		sent, _, err := repos.purchaseOrders.List(ctx, outbound.PurchaseOrderFilter{Limit: 100, Status: domain.OrderStatusSent})
		if err != nil {
			t.Fatal(err)
		}
		if len(sent) != 1 || sent[0].ID != second.ID {
			t.Fatalf("status filter = %+v", sent)
		}

		paid, _, err := repos.purchaseOrders.List(ctx, outbound.PurchaseOrderFilter{Limit: 100, PaymentStatus: domain.PurchasePaymentStatusPaid})
		if err != nil {
			t.Fatal(err)
		}
		if len(paid) != 1 || paid[0].ID != second.ID {
			t.Fatalf("payment filter = %+v", paid)
		}

		distinct := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
		if err := repos.gorm.Exec(
			"UPDATE purchase_orders SET order_date = ? WHERE id = ?",
			gormstore.Timestamp(distinct), second.ID,
		).Error; err != nil {
			t.Fatal(err)
		}
		start := distinct.Add(-time.Minute)
		end := distinct.Add(time.Minute)
		inRange, _, err := repos.purchaseOrders.List(ctx, outbound.PurchaseOrderFilter{Limit: 100, MinDate: &start, MaxDate: &end})
		if err != nil {
			t.Fatal(err)
		}
		if len(inRange) != 1 || inRange[0].ID != second.ID {
			t.Fatalf("date filter = %+v", inRange)
		}

		page, hasNext, err := repos.purchaseOrders.List(ctx, outbound.PurchaseOrderFilter{Limit: 1, Skip: 1})
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(page) != 1 || page[0].ID != order.ID {
			t.Fatalf("page = %+v has_next=%v", page, hasNext)
		}

		if _, err := service.Delete(ctx, second.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("receiving increments stock once and upserts catalog", func(t *testing.T) {
		received := domain.OrderStatusReceived
		updated, err := service.Update(ctx, order.ID, inbound.UpdatePurchaseOrderInput{
			Status: inbound.Optional[string]{Set: true, Value: &received},
		})
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != domain.OrderStatusReceived {
			t.Fatalf("status = %q", updated.Status)
		}
		if stock := contractStock(t, repos, hammer.ID); stock != 12 {
			t.Fatalf("hammer stock = %d, want 12", stock)
		}
		if stock := contractStock(t, repos, nails.ID); stock != 11 {
			t.Fatalf("nails stock = %d, want 11", stock)
		}

		catalogs, _, err := repos.supplierCatalogs.List(ctx, 0, 10, &provider.ID, &hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalogs) != 1 {
			t.Fatalf("catalogs = %+v, want one", catalogs)
		}
		catalog := catalogs[0]
		if !catalog.LastQuotedCost.Equal(decimal.RequireFromString("10.25")) ||
			catalog.SupplierSKU == nil || *catalog.SupplierSKU != sku ||
			catalog.LeadTime == nil || *catalog.LeadTime < 0 {
			t.Fatalf("catalog = %+v", catalog)
		}

		if _, err := service.Update(ctx, order.ID, inbound.UpdatePurchaseOrderInput{
			Status: inbound.Optional[string]{Set: true, Value: &received},
		}); err != nil {
			t.Fatal(err)
		}
		if stock := contractStock(t, repos, hammer.ID); stock != 12 {
			t.Fatalf("hammer stock after second receive = %d, want 12", stock)
		}
		catalogs, _, err = repos.supplierCatalogs.List(ctx, 0, 10, &provider.ID, &hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalogs) != 1 {
			t.Fatalf("catalog rows after second receive = %d, want 1", len(catalogs))
		}

		// Existing supplier SKU survives an upsert that omits it.
		cost := decimal.RequireFromString("11.75")
		if err := repos.supplierCatalogs.Upsert(ctx, &domain.SupplierCatalog{
			ProductID: hammer.ID, ProviderID: provider.ID, LastQuotedCost: cost, LeadTime: catalog.LeadTime,
		}); err != nil {
			t.Fatal(err)
		}
		catalogs, _, err = repos.supplierCatalogs.List(ctx, 0, 10, &provider.ID, &hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalogs) != 1 || !catalogs[0].LastQuotedCost.Equal(cost) ||
			catalogs[0].SupplierSKU == nil || *catalogs[0].SupplierSKU != sku {
			t.Fatalf("upserted catalog = %+v", catalogs)
		}
	})

	t.Run("received item mutation does not touch stock and delete keeps stock", func(t *testing.T) {
		before := contractStock(t, repos, hammer.ID)
		catalogsBefore, _, err := repos.supplierCatalogs.List(ctx, 0, 10, &provider.ID, &hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		items, err := service.ListItems(ctx, order.ID, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.UpdateItem(ctx, order.ID, items.Items[0].ID, inbound.UpdateOrderItemInput{
			Quantity: inbound.Optional[int]{Set: true, Value: contractInt(3)},
		}); err != nil {
			t.Fatal(err)
		}
		if got := contractStock(t, repos, hammer.ID); got != before {
			t.Fatalf("stock after received item update = %d, want %d", got, before)
		}
		catalogsAfter, _, err := repos.supplierCatalogs.List(ctx, 0, 10, &provider.ID, &hammer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalogsAfter) != len(catalogsBefore) {
			t.Fatalf("catalogs after item update = %d, want %d", len(catalogsAfter), len(catalogsBefore))
		}

		if _, err := service.Delete(ctx, order.ID); err != nil {
			t.Fatal(err)
		}
		if got := contractStock(t, repos, hammer.ID); got != before {
			t.Fatalf("stock after delete = %d, want %d", got, before)
		}
		if _, err := repos.purchaseOrders.Get(ctx, order.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("deleted order error = %v, want ErrNotFound", err)
		}
	})
}

func contractStock(t *testing.T, repos *repositories, productID int64) int {
	t.Helper()
	var stock int
	if err := repos.gorm.Raw("SELECT stock_quantity FROM products WHERE id = ?", productID).Row().Scan(&stock); err != nil {
		t.Fatal(err)
	}
	return stock
}

func contractInt(value int) *int { return &value }

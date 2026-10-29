package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

func TestDashboardRepositoryContract(t *testing.T) {
	backends := []struct {
		name  string
		setup func(*testing.T) *repositories
	}{
		{"sqlite", openSQLiteRepositories},
		{"postgres", openPostgresRepositories},
	}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			testDashboard(t, backend.setup(t))
		})
	}
}

func testDashboard(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()
	todayStart := time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC)
	trendStart := todayStart.AddDate(0, 0, -6)
	trendEnd := todayStart.AddDate(0, 0, 1)
	topSince := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)

	t.Run("empty database", func(t *testing.T) {
		summary, err := repos.dashboard.Summary(ctx, todayStart)
		if err != nil {
			t.Fatal(err)
		}
		if !summary.InventoryValue.IsZero() || !summary.TotalSalesToday.IsZero() ||
			!summary.PendingPaymentSales.IsZero() || summary.PendingOrders != 0 || summary.DeliveredOrders != 0 {
			t.Fatalf("empty summary = %+v, want all zeros", summary)
		}

		trend, err := repos.dashboard.SalesTrend(ctx, trendStart, trendEnd)
		if err != nil {
			t.Fatal(err)
		}
		if len(trend) != 0 {
			t.Fatalf("empty trend = %+v, want no rows", trend)
		}

		products, err := repos.dashboard.TopProducts(ctx, topSince, 10)
		if err != nil {
			t.Fatal(err)
		}
		if products == nil || len(products) != 0 {
			t.Fatalf("empty top products = %#v, want an empty slice", products)
		}

		distribution, err := repos.dashboard.OrderStatusDistribution(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if distribution == nil || len(distribution) != 0 {
			t.Fatalf("empty distribution = %#v, want an empty slice", distribution)
		}
	})

	category, err := repos.categories.Create(ctx, &domain.Category{Name: "Dashboard Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "Dashboard Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "Dashboard Material"})
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
	productA := newProduct("DASH-1", "Dashboard Hammer", "10.00", 3)
	productB := newProduct("DASH-2", "Dashboard Nails", "2.50", 2)
	productC := newProduct("DASH-3", "Dashboard Loose", "5.00", 1)
	if err := repos.gorm.Exec(
		"UPDATE products SET brand_id = NULL, category_id = NULL, material_id = NULL WHERE id = ?", productC.ID,
	).Error; err != nil {
		t.Fatal(err)
	}

	sale := func(createdAt time.Time, total, paid string, paymentStatus, deliveryStatus string, items []*domain.SaleItem) *domain.Sale {
		t.Helper()
		created, err := repos.sales.Create(ctx, &domain.Sale{
			CreatedAt:      createdAt,
			TotalAmount:    saleDecimal(total),
			AmountPaid:     saleDecimal(paid),
			PaymentStatus:  paymentStatus,
			DeliveryStatus: deliveryStatus,
		}, items)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}

	// Exactly at Bogota midnight: counts as today.
	sale(time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC),
		"28.50", "8.50", domain.PaymentStatusPartial, domain.DeliveryStatusDelivered,
		[]*domain.SaleItem{
			{ProductID: productA.ID, Quantity: 2, DeliveredQuantity: 2, UnitPrice: saleDecimal("10.50"), Subtotal: saleDecimal("21.00")},
			{ProductID: productB.ID, Quantity: 1, DeliveredQuantity: 0, UnitPrice: saleDecimal("2.50"), Subtotal: saleDecimal("2.50")},
			{ProductID: productC.ID, Quantity: 1, DeliveredQuantity: 1, UnitPrice: saleDecimal("5.00"), Subtotal: saleDecimal("5.00")},
		},
	)
	// One second before Bogota midnight, and overpaid but already paid.
	sale(time.Date(2026, time.October, 24, 4, 59, 59, 0, time.UTC),
		"300.00", "310.00", domain.PaymentStatusPaid, domain.DeliveryStatusPending,
		[]*domain.SaleItem{
			{ProductID: productA.ID, Quantity: 1, DeliveredQuantity: 0, UnitPrice: saleDecimal("100.00"), Subtotal: saleDecimal("100.00")},
			{ProductID: productB.ID, Quantity: 1, DeliveredQuantity: 0, UnitPrice: saleDecimal("200.00"), Subtotal: saleDecimal("200.00")},
		},
	)
	// A second delivered sale outside today: delivered_orders counts sales, so
	// the expected value differs from the single received purchase order below.
	sale(time.Date(2026, time.October, 22, 12, 0, 0, 0, time.UTC),
		"0.00", "0.00", domain.PaymentStatusPaid, domain.DeliveryStatusDelivered, nil,
	)

	provider, err := repos.providers.Create(ctx, &domain.Provider{Name: "Dashboard Provider"})
	if err != nil {
		t.Fatal(err)
	}
	newOrder := func(status, paymentStatus string, orderDate time.Time, total string) {
		t.Helper()
		if _, err := repos.purchaseOrders.Create(ctx, &domain.PurchaseOrder{
			ProviderID:    provider.ID,
			Status:        status,
			PaymentStatus: paymentStatus,
			OrderDate:     orderDate,
			TotalAmount:   saleDecimal(total),
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	today := time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	threeDaysAgo := time.Date(2026, time.October, 21, 12, 0, 0, 0, time.UTC)
	newOrder(domain.OrderStatusPending, domain.PurchasePaymentStatusPending, today, "10.00")
	newOrder(domain.OrderStatusPending, domain.PurchasePaymentStatusPending, today, "20.00")
	newOrder(domain.OrderStatusReceived, domain.PurchasePaymentStatusPaid, today, "30.00")
	newOrder(domain.OrderStatusSent, domain.PurchasePaymentStatusPending, yesterday, "40.00")
	newOrder(domain.OrderStatusSent, domain.PurchasePaymentStatusPending, yesterday, "50.00")
	newOrder(domain.OrderStatusPending, domain.PurchasePaymentStatusPending, threeDaysAgo, "70.00")

	t.Run("summary money counts and boundaries", func(t *testing.T) {
		summary, err := repos.dashboard.Summary(ctx, todayStart)
		if err != nil {
			t.Fatal(err)
		}
		assertDashboardMoney(t, "inventory value", summary.InventoryValue, "40.00")
		assertDashboardMoney(t, "sales today", summary.TotalSalesToday, "28.50")
		assertDashboardMoney(t, "pending payment", summary.PendingPaymentSales, "20.00")
		if summary.PendingOrders != 3 || summary.DeliveredOrders != 2 {
			t.Fatalf("counts = pending %d delivered %d, want 3/2 (delivered sales)", summary.PendingOrders, summary.DeliveredOrders)
		}
	})

	t.Run("sales trend groups Bogota days", func(t *testing.T) {
		points, err := repos.dashboard.SalesTrend(ctx, trendStart, trendEnd)
		if err != nil {
			t.Fatal(err)
		}
		byDate := make(map[string]domain.SalesTrendPoint, len(points))
		for _, point := range points {
			byDate[point.Date] = point
		}
		assertDashboardMoney(t, "Oct 23 sales", byDate["2026-10-23"].SalesAmount, "300.00")
		assertDashboardMoney(t, "Oct 23 orders", byDate["2026-10-23"].OrdersCost, "90.00")
		assertDashboardMoney(t, "Oct 24 sales", byDate["2026-10-24"].SalesAmount, "28.50")
		assertDashboardMoney(t, "Oct 24 orders", byDate["2026-10-24"].OrdersCost, "60.00")
		assertDashboardMoney(t, "Oct 21 orders", byDate["2026-10-21"].OrdersCost, "70.00")
		if _, ok := byDate["2026-10-24"]; !ok {
			t.Fatal("missing Bogota today bucket")
		}
	})

	t.Run("top products rank by subtotal and use delivered quantity", func(t *testing.T) {
		products, err := repos.dashboard.TopProducts(ctx, topSince, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(products) != 3 {
			t.Fatalf("products = %d, want 3", len(products))
		}
		if products[0].ProductID != productB.ID || products[1].ProductID != productA.ID || products[2].ProductID != productC.ID {
			t.Fatalf("order = %d,%d,%d, want %d,%d,%d",
				products[0].ProductID, products[1].ProductID, products[2].ProductID,
				productB.ID, productA.ID, productC.ID)
		}
		assertDashboardMoney(t, "product B amount", products[0].TotalSalesAmount, "202.50")
		if products[0].TotalQuantitySold != 0 {
			t.Fatalf("product B quantity = %d, want delivered quantity 0", products[0].TotalQuantitySold)
		}
		assertDashboardMoney(t, "product A amount", products[1].TotalSalesAmount, "121.00")
		if products[1].TotalQuantitySold != 2 {
			t.Fatalf("product A quantity = %d, want 2", products[1].TotalQuantitySold)
		}
		if products[2].BrandName != nil || products[2].CategoryName != nil || products[2].MaterialName != nil {
			t.Fatalf("null relations = %v/%v/%v, want nil",
				products[2].BrandName, products[2].CategoryName, products[2].MaterialName)
		}

		limited, err := repos.dashboard.TopProducts(ctx, topSince, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(limited) != 2 || limited[0].ProductID != productB.ID || limited[1].ProductID != productA.ID {
			t.Fatalf("limited = %+v, want the top two", limited)
		}
	})

	t.Run("order status distribution counts and ordering", func(t *testing.T) {
		rows, err := repos.dashboard.OrderStatusDistribution(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 {
			t.Fatalf("rows = %d, want 3", len(rows))
		}
		want := []struct {
			status string
			count  int64
		}{
			{"pending", 3},
			{"sent", 2},
			{"received", 1},
		}
		for i, expected := range want {
			if rows[i].Status != expected.status || rows[i].Count != expected.count {
				t.Fatalf("row %d = %q/%d, want %q/%d", i, rows[i].Status, rows[i].Count, expected.status, expected.count)
			}
		}
	})
}

func assertDashboardMoney(t *testing.T, label string, got decimal.Decimal, want string) {
	t.Helper()
	expected := decimal.RequireFromString(want)
	if !got.Equal(expected) {
		t.Fatalf("%s = %s, want %s", label, got, expected)
	}
}

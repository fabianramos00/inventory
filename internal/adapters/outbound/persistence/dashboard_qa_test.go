package persistence_test

import (
	"context"
	"testing"
	"time"

	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
)

func dashboardBackends() []struct {
	name  string
	setup func(*testing.T) *repositories
} {
	return []struct {
		name  string
		setup func(*testing.T) *repositories
	}{
		{"sqlite", openSQLiteRepositories},
		{"postgres", openPostgresRepositories},
	}
}

func bogotaDate(t time.Time) string {
	return t.In(time.FixedZone("America/Bogota", -5*60*60)).Format("2006-01-02")
}

// Empty PostgreSQL and SQLite databases must produce zeros, seven ordered
// zero-filled trend days, and empty arrays through the real service.
func TestDashboardEmptyStateIntegration(t *testing.T) {
	for _, backend := range dashboardBackends() {
		t.Run(backend.name, func(t *testing.T) {
			repos := backend.setup(t)
			service := application.NewDashboardService(repos.dashboard)
			ctx := context.Background()

			before := bogotaDate(time.Now())
			summary, err := service.Summary(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !summary.InventoryValue.IsZero() || !summary.TotalSalesToday.IsZero() ||
				!summary.PendingPaymentSales.IsZero() || summary.PendingOrders != 0 || summary.DeliveredOrders != 0 {
				t.Fatalf("empty summary = %+v, want all zeros", summary)
			}

			points, err := service.SalesTrend(ctx)
			if err != nil {
				t.Fatal(err)
			}
			after := bogotaDate(time.Now())
			if len(points) != 7 {
				t.Fatalf("trend length = %d, want 7", len(points))
			}
			for i, point := range points {
				if i > 0 && point.Date <= points[i-1].Date {
					t.Fatalf("trend not oldest-first at %d: %s after %s", i, point.Date, points[i-1].Date)
				}
				if !point.SalesAmount.IsZero() || !point.OrdersCost.IsZero() {
					t.Fatalf("point %s = %+v, want zeros", point.Date, point)
				}
			}
			if last := points[len(points)-1].Date; last != before && last != after {
				t.Fatalf("last trend date = %s, want Bogota today %s", last, before)
			}

			products, err := service.TopProducts(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			if products == nil || len(products) != 0 {
				t.Fatalf("empty top products = %#v, want an empty non-nil slice", products)
			}

			distribution, err := service.OrderStatusDistribution(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if distribution == nil || len(distribution) != 0 {
				t.Fatalf("empty distribution = %#v, want an empty non-nil slice", distribution)
			}
		})
	}
}

// The top-products window is inclusive at `since` and excludes anything older,
// matching the legacy `now - 7 days` filter.
func TestDashboardTopProductsWindowBoundary(t *testing.T) {
	for _, backend := range dashboardBackends() {
		t.Run(backend.name, func(t *testing.T) {
			repos := backend.setup(t)
			ctx := context.Background()
			category, brand, material := dashboardCatalog(t, repos)

			inside := dashboardProduct(t, repos, "DASH-WINDOW-IN", "Inside Window", "10.00", 1, category.ID, brand.ID, material.ID)
			outside := dashboardProduct(t, repos, "DASH-WINDOW-OUT", "Outside Window", "99.00", 1, category.ID, brand.ID, material.ID)

			since := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)
			dashboardSale(t, repos, since, "10.00", []*domain.SaleItem{{
				ProductID: inside.ID, Quantity: 1, DeliveredQuantity: 1,
				UnitPrice: saleDecimal("10.00"), Subtotal: saleDecimal("10.00"),
			}})
			dashboardSale(t, repos, since.Add(-time.Second), "99.00", []*domain.SaleItem{{
				ProductID: outside.ID, Quantity: 1, DeliveredQuantity: 1,
				UnitPrice: saleDecimal("99.00"), Subtotal: saleDecimal("99.00"),
			}})

			products, err := repos.dashboard.TopProducts(ctx, since, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(products) != 1 || products[0].ProductID != inside.ID {
				t.Fatalf("top products = %+v, want only product %d", products, inside.ID)
			}
		})
	}
}

// Inventory value uses sale price and exact cents must survive both adapters.
func TestDashboardMoneyIsExactOnBothBackends(t *testing.T) {
	for _, backend := range dashboardBackends() {
		t.Run(backend.name, func(t *testing.T) {
			repos := backend.setup(t)
			ctx := context.Background()
			category, brand, material := dashboardCatalog(t, repos)

			product := dashboardProduct(t, repos, "DASH-CENTS", "Cent Product", "0.10", 3, category.ID, brand.ID, material.ID)
			todayStart := time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC)
			dashboardSale(t, repos, time.Date(2026, time.October, 24, 6, 0, 0, 0, time.UTC), "0.07", []*domain.SaleItem{{
				ProductID: product.ID, Quantity: 1, DeliveredQuantity: 1,
				UnitPrice: saleDecimal("0.07"), Subtotal: saleDecimal("0.07"),
			}}, func(sale *domain.Sale) {
				sale.AmountPaid = saleDecimal("0.01")
				sale.PaymentStatus = domain.PaymentStatusPartial
			})

			summary, err := repos.dashboard.Summary(ctx, todayStart)
			if err != nil {
				t.Fatal(err)
			}
			assertDashboardMoney(t, "inventory value", summary.InventoryValue, "0.30")
			assertDashboardMoney(t, "sales today", summary.TotalSalesToday, "0.07")
			assertDashboardMoney(t, "pending payment", summary.PendingPaymentSales, "0.06")
		})
	}
}

func dashboardCatalog(t *testing.T, repos *repositories) (*domain.Category, *domain.Brand, *domain.Material) {
	t.Helper()
	ctx := context.Background()
	category, err := repos.categories.Create(ctx, &domain.Category{Name: "Dashboard QA Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "Dashboard QA Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "Dashboard QA Material"})
	if err != nil {
		t.Fatal(err)
	}
	return category, brand, material
}

func dashboardProduct(t *testing.T, repos *repositories, sku, name, price string, stock int, categoryID, brandID, materialID int64) *domain.Product {
	t.Helper()
	product, err := repos.products.Create(context.Background(), &domain.Product{
		SKU:               sku,
		Name:              name,
		Price:             saleDecimal(price),
		Cost:              saleDecimal("999.99"),
		StockQuantity:     stock,
		LowStockThreshold: 0,
		BrandID:           brandID,
		CategoryID:        categoryID,
		MaterialID:        materialID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return product
}

func dashboardSale(t *testing.T, repos *repositories, createdAt time.Time, total string, items []*domain.SaleItem, mutators ...func(*domain.Sale)) {
	t.Helper()
	sale := &domain.Sale{
		CreatedAt:      createdAt,
		TotalAmount:    saleDecimal(total),
		AmountPaid:     saleDecimal(total),
		PaymentStatus:  domain.PaymentStatusPaid,
		DeliveryStatus: domain.DeliveryStatusDelivered,
	}
	for _, mutate := range mutators {
		mutate(sale)
	}
	if _, err := repos.sales.Create(context.Background(), sale, items); err != nil {
		t.Fatal(err)
	}
}

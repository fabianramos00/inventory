package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

func testSupplierCatalogs(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()
	str := func(value string) *string { return &value }

	category, err := repos.categories.Create(ctx, &domain.Category{Name: "Catalog Contract Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "Catalog Contract Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "Catalog Contract Material"})
	if err != nil {
		t.Fatal(err)
	}
	createProduct := func(sku, name string) *domain.Product {
		t.Helper()
		product, err := repos.products.Create(ctx, &domain.Product{
			SKU:               sku,
			Name:              name,
			Price:             decimal.RequireFromString("19.90"),
			Cost:              decimal.RequireFromString("10.00"),
			StockQuantity:     4,
			LowStockThreshold: 5,
			BrandID:           brand.ID,
			CategoryID:        category.ID,
			MaterialID:        material.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return product
	}

	providerOne, err := repos.providers.Create(ctx, &domain.Provider{
		Name:        "Catalog Provider One",
		ContactInfo: str("Contact One"),
		Email:       str("one@example.com"),
		Phone:       str("3001"),
	})
	if err != nil {
		t.Fatal(err)
	}
	providerTwo, err := repos.providers.Create(ctx, &domain.Provider{Name: "Catalog Provider Two"})
	if err != nil {
		t.Fatal(err)
	}
	productOne := createProduct("CATALOG-CONTRACT-1", "Catalog Hammer")
	productTwo := createProduct("CATALOG-CONTRACT-2", "Catalog Screwdriver")

	insertCatalog := func(providerID, productID int64, sku, cost string, leadTime *int64) int64 {
		t.Helper()
		money := any(cost)
		if repos.gorm.Dialector.Name() == "sqlite" {
			money = decimal.RequireFromString(cost).Shift(2).IntPart()
		}
		var id int64
		if err := repos.gorm.Raw(
			"INSERT INTO supplier_catalogs (provider_id, product_id, supplier_sku, last_quoted_cost, lead_time) VALUES (?, ?, ?, ?, ?) RETURNING id",
			providerID, productID, sku, money, leadTime,
		).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}

	leadTime := int64(7)
	firstID := insertCatalog(providerOne.ID, productOne.ID, "PROV-001", "12.34", &leadTime)
	secondID := insertCatalog(providerOne.ID, productTwo.ID, "PROV-002", "0.10", nil)
	thirdID := insertCatalog(providerTwo.ID, productOne.ID, "OTRO-001", "99.99", nil)

	t.Run("get relations and money", func(t *testing.T) {
		catalog, err := repos.supplierCatalogs.Get(ctx, firstID)
		if err != nil {
			t.Fatal(err)
		}
		if catalog.ID != firstID || catalog.ProductID != productOne.ID || catalog.ProviderID != providerOne.ID {
			t.Fatalf("catalog ids = %+v", catalog)
		}
		if catalog.SupplierSKU == nil || *catalog.SupplierSKU != "PROV-001" {
			t.Fatalf("supplier_sku = %v", catalog.SupplierSKU)
		}
		if !catalog.LastQuotedCost.Equal(decimal.RequireFromString("12.34")) {
			t.Fatalf("last_quoted_cost = %s, want 12.34", catalog.LastQuotedCost)
		}
		if catalog.LeadTime == nil || *catalog.LeadTime != 7 {
			t.Fatalf("lead_time = %v", catalog.LeadTime)
		}
		if catalog.Provider == nil || catalog.Provider.ID != providerOne.ID || catalog.Provider.Name != providerOne.Name ||
			catalog.Provider.ContactInfo == nil || *catalog.Provider.ContactInfo != "Contact One" {
			t.Fatalf("provider = %+v", catalog.Provider)
		}
		if catalog.Product == nil || catalog.Product.ID != productOne.ID || catalog.Product.SKU != productOne.SKU ||
			catalog.Product.Name != productOne.Name || catalog.Product.StockQuantity != 4 ||
			!catalog.Product.Price.Equal(decimal.RequireFromString("19.90")) ||
			!catalog.Product.Cost.Equal(decimal.RequireFromString("10.00")) {
			t.Fatalf("product = %+v", catalog.Product)
		}
		second, err := repos.supplierCatalogs.Get(ctx, secondID)
		if err != nil {
			t.Fatal(err)
		}
		if !second.LastQuotedCost.Equal(decimal.RequireFromString("0.10")) || second.LeadTime != nil {
			t.Fatalf("second catalog cost/lead_time = %s/%v, want 0.10/nil", second.LastQuotedCost, second.LeadTime)
		}
		if _, err := repos.supplierCatalogs.Get(ctx, firstID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get missing error = %v, want ErrNotFound", err)
		}
	})

	t.Run("list filters pagination and order", func(t *testing.T) {
		all, hasNext, err := repos.supplierCatalogs.List(ctx, 0, 100, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(all) != 3 {
			t.Fatalf("all = %d items has_next=%v, want 3 and false", len(all), hasNext)
		}
		if all[0].ID != thirdID || all[1].ID != secondID || all[2].ID != firstID {
			t.Fatalf("list order = %d/%d/%d, want %d/%d/%d", all[0].ID, all[1].ID, all[2].ID, thirdID, secondID, firstID)
		}

		providerOneID := providerOne.ID
		byProvider, hasNext, err := repos.supplierCatalogs.List(ctx, 0, 100, &providerOneID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(byProvider) != 2 || byProvider[0].ID != secondID || byProvider[1].ID != firstID {
			t.Fatalf("provider filter = %+v has_next=%v", byProvider, hasNext)
		}

		productOneID := productOne.ID
		byProduct, hasNext, err := repos.supplierCatalogs.List(ctx, 0, 100, nil, &productOneID)
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(byProduct) != 2 || byProduct[0].ID != thirdID || byProduct[1].ID != firstID {
			t.Fatalf("product filter = %+v has_next=%v", byProduct, hasNext)
		}

		both, hasNext, err := repos.supplierCatalogs.List(ctx, 0, 100, &providerOneID, &productOneID)
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(both) != 1 || both[0].ID != firstID {
			t.Fatalf("combined filter = %+v has_next=%v", both, hasNext)
		}

		zero := int64(0)
		ignored, hasNext, err := repos.supplierCatalogs.List(ctx, 0, 100, &zero, &zero)
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(ignored) != 3 {
			t.Fatalf("non-positive filters must be ignored: %d items has_next=%v", len(ignored), hasNext)
		}

		firstPage, hasNext, err := repos.supplierCatalogs.List(ctx, 0, 2, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !hasNext || len(firstPage) != 2 || firstPage[0].ID != thirdID || firstPage[1].ID != secondID {
			t.Fatalf("first page = %+v has_next=%v", firstPage, hasNext)
		}
		lastPage, hasNext, err := repos.supplierCatalogs.List(ctx, 2, 2, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(lastPage) != 1 || lastPage[0].ID != firstID {
			t.Fatalf("last page = %+v has_next=%v", lastPage, hasNext)
		}
	})

	if repos.gorm.Dialector.Name() == "sqlite" {
		var cents int64
		if err := repos.gorm.Raw("SELECT last_quoted_cost FROM supplier_catalogs WHERE id = ?", firstID).Row().Scan(&cents); err != nil {
			t.Fatal(err)
		}
		if cents != 1234 {
			t.Fatalf("stored last_quoted_cost = %d cents, want 1234", cents)
		}
	}
}

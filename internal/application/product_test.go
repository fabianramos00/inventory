package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

type fakeProductStore struct {
	getResult *domain.ProductDetail
	getErr    error

	listItems   []*domain.ProductDetail
	listHasNext bool
	listErr     error
	listFilter  outbound.ProductFilter

	createErrs  []error
	createCalls int
	created     *domain.Product

	updateID  int64
	updated   *domain.Product
	updateErr error

	deleteResult *domain.Product
	deleteErr    error

	stats    *domain.ProductStats
	statsErr error
}

func (f *fakeProductStore) Get(_ context.Context, _ int64) (*domain.ProductDetail, error) {
	return f.getResult, f.getErr
}

func (f *fakeProductStore) List(_ context.Context, filter outbound.ProductFilter) ([]*domain.ProductDetail, bool, error) {
	f.listFilter = filter
	return f.listItems, f.listHasNext, f.listErr
}

func (f *fakeProductStore) Create(_ context.Context, product *domain.Product) (*domain.Product, error) {
	f.createCalls++
	if f.createCalls <= len(f.createErrs) && f.createErrs[f.createCalls-1] != nil {
		return nil, f.createErrs[f.createCalls-1]
	}
	product.ID = 99
	f.created = product
	return product, nil
}

func (f *fakeProductStore) Update(_ context.Context, id int64, product *domain.Product) error {
	f.updateID, f.updated = id, product
	return f.updateErr
}

func (f *fakeProductStore) Delete(_ context.Context, _ int64) (*domain.Product, error) {
	return f.deleteResult, f.deleteErr
}

func (f *fakeProductStore) Stats(context.Context) (*domain.ProductStats, error) {
	return f.stats, f.statsErr
}

func validProductInput() inbound.CreateProductInput {
	return inbound.CreateProductInput{
		Name:       "Hammer",
		Price:      decimal.RequireFromString("10"),
		Cost:       decimal.RequireFromString("5"),
		BrandID:    1,
		CategoryID: 2,
		MaterialID: 3,
	}
}

func intPtr(value int) *int {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func TestProductCreateDefaultsAndGeneratedSKU(t *testing.T) {
	store := &fakeProductStore{}
	service := NewProductService(store)

	product, err := service.Create(context.Background(), validProductInput())
	if err != nil {
		t.Fatal(err)
	}
	if product.StockQuantity != 0 || product.LowStockThreshold != 10 {
		t.Fatalf("defaults = stock %d threshold %d, want 0/10", product.StockQuantity, product.LowStockThreshold)
	}
	if !regexp.MustCompile(`^SKU-[0-9A-F]{8}$`).MatchString(product.SKU) {
		t.Fatalf("generated sku = %q, want SKU-XXXXXXXX", product.SKU)
	}
	if product.Name != "Hammer" || product.ID != 99 {
		t.Fatalf("product = %+v", product)
	}
}

func TestProductCreateExplicitValuesAndRounding(t *testing.T) {
	store := &fakeProductStore{}
	service := NewProductService(store)

	input := validProductInput()
	input.SKU = ptr("  ABC-1  ")
	input.SizeValue = ptr("")
	input.MeasurementUnitID = int64Ptr(4)
	input.Price = decimal.RequireFromString("10.005")
	input.Cost = decimal.RequireFromString("5.678")
	input.StockQuantity = inbound.Optional[int]{Set: true, Value: intPtr(7)}
	input.LowStockThreshold = inbound.Optional[int]{Set: true, Value: intPtr(2)}

	product, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if product.SKU != "ABC-1" {
		t.Errorf("sku = %q, want trimmed ABC-1", product.SKU)
	}
	if product.SizeValue != nil || product.MeasurementUnitID == nil || *product.MeasurementUnitID != 4 {
		t.Errorf("optional fields = %v/%v", product.SizeValue, product.MeasurementUnitID)
	}
	if !product.Price.Equal(decimal.RequireFromString("10.01")) {
		t.Errorf("price = %s, want 10.01", product.Price)
	}
	if !product.Cost.Equal(decimal.RequireFromString("5.68")) {
		t.Errorf("cost = %s, want 5.68", product.Cost)
	}
	if product.StockQuantity != 7 || product.LowStockThreshold != 2 {
		t.Errorf("stock = %d/%d, want 7/2", product.StockQuantity, product.LowStockThreshold)
	}
}

func TestProductCreateValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*inbound.CreateProductInput)
	}{
		{name: "empty name", mutate: func(in *inbound.CreateProductInput) { in.Name = "" }},
		{name: "blank name", mutate: func(in *inbound.CreateProductInput) { in.Name = "   " }},
		{name: "name too long", mutate: func(in *inbound.CreateProductInput) { in.Name = repeat("a", 151) }},
		{name: "sku too long", mutate: func(in *inbound.CreateProductInput) { in.SKU = ptr(repeat("s", 101)) }},
		{name: "size too long", mutate: func(in *inbound.CreateProductInput) { in.SizeValue = ptr(repeat("x", 21)) }},
		{name: "zero price", mutate: func(in *inbound.CreateProductInput) { in.Price = decimal.Zero }},
		{name: "price rounds to zero", mutate: func(in *inbound.CreateProductInput) { in.Price = decimal.RequireFromString("0.004") }},
		{name: "negative price", mutate: func(in *inbound.CreateProductInput) { in.Price = decimal.RequireFromString("-1") }},
		{name: "price above max", mutate: func(in *inbound.CreateProductInput) { in.Price = decimal.RequireFromString("10000000000.00") }},
		{name: "price rounds above max", mutate: func(in *inbound.CreateProductInput) { in.Price = decimal.RequireFromString("9999999999.995") }},
		{name: "zero cost", mutate: func(in *inbound.CreateProductInput) { in.Cost = decimal.Zero }},
		{name: "negative stock", mutate: func(in *inbound.CreateProductInput) {
			in.StockQuantity = inbound.Optional[int]{Set: true, Value: intPtr(-1)}
		}},
		{name: "null stock", mutate: func(in *inbound.CreateProductInput) {
			in.StockQuantity = inbound.Optional[int]{Set: true}
		}},
		{name: "negative threshold", mutate: func(in *inbound.CreateProductInput) {
			in.LowStockThreshold = inbound.Optional[int]{Set: true, Value: intPtr(-1)}
		}},
		{name: "null threshold", mutate: func(in *inbound.CreateProductInput) {
			in.LowStockThreshold = inbound.Optional[int]{Set: true}
		}},
		{name: "zero brand", mutate: func(in *inbound.CreateProductInput) { in.BrandID = 0 }},
		{name: "zero category", mutate: func(in *inbound.CreateProductInput) { in.CategoryID = 0 }},
		{name: "zero material", mutate: func(in *inbound.CreateProductInput) { in.MaterialID = 0 }},
		{name: "zero measurement unit", mutate: func(in *inbound.CreateProductInput) { in.MeasurementUnitID = int64Ptr(0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validProductInput()
			tt.mutate(&input)
			store := &fakeProductStore{}
			service := NewProductService(store)
			if _, err := service.Create(context.Background(), input); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
			if store.createCalls != 0 {
				t.Fatal("repository was called for invalid input")
			}
		})
	}
}

func TestProductCreateSKURetry(t *testing.T) {
	t.Run("generated collision retries", func(t *testing.T) {
		store := &fakeProductStore{createErrs: []error{domain.ErrConflict, domain.ErrConflict}}
		service := NewProductService(store)
		product, err := service.Create(context.Background(), validProductInput())
		if err != nil {
			t.Fatal(err)
		}
		if store.createCalls != 3 {
			t.Fatalf("create calls = %d, want 3", store.createCalls)
		}
		if !regexp.MustCompile(`^SKU-[0-9A-F]{8}$`).MatchString(product.SKU) {
			t.Fatalf("sku = %q", product.SKU)
		}
	})

	t.Run("generated collision gives up", func(t *testing.T) {
		store := &fakeProductStore{createErrs: []error{
			domain.ErrConflict, domain.ErrConflict, domain.ErrConflict, domain.ErrConflict, domain.ErrConflict,
		}}
		service := NewProductService(store)
		if _, err := service.Create(context.Background(), validProductInput()); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("error = %v, want ErrConflict", err)
		}
		if store.createCalls != 5 {
			t.Fatalf("create calls = %d, want 5", store.createCalls)
		}
	})

	t.Run("user sku collision is not retried", func(t *testing.T) {
		store := &fakeProductStore{createErrs: []error{domain.ErrConflict}}
		service := NewProductService(store)
		input := validProductInput()
		input.SKU = ptr("DUP-1")
		if _, err := service.Create(context.Background(), input); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("error = %v, want ErrConflict", err)
		}
		if store.createCalls != 1 {
			t.Fatalf("create calls = %d, want 1", store.createCalls)
		}
	})
}

func existingProduct() *domain.ProductDetail {
	return &domain.ProductDetail{
		Product: domain.Product{
			ID:                7,
			SKU:               "OLD",
			Name:              "Old name",
			SizeValue:         ptr("M"),
			MeasurementUnitID: int64Ptr(3),
			Price:             decimal.RequireFromString("10.00"),
			Cost:              decimal.RequireFromString("4.00"),
			StockQuantity:     5,
			LowStockThreshold: 1,
			BrandID:           1,
			CategoryID:        2,
			MaterialID:        3,
		},
	}
}

func TestProductPartialUpdate(t *testing.T) {
	t.Run("omitted fields stay and null clears optional", func(t *testing.T) {
		store := &fakeProductStore{getResult: existingProduct()}
		service := NewProductService(store)
		product, err := service.Update(context.Background(), 7, inbound.UpdateProductInput{
			SizeValue:         inbound.Optional[string]{Set: true},
			MeasurementUnitID: inbound.Optional[int64]{Set: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if store.updateID != 7 || store.updated != product {
			t.Fatalf("repository update = id %d product %p, want id 7 product %p", store.updateID, store.updated, product)
		}
		if product.Name != "Old name" || product.Price.String() != "10" || product.StockQuantity != 5 {
			t.Fatalf("omitted fields changed: %+v", product)
		}
		if product.SizeValue != nil || product.MeasurementUnitID != nil {
			t.Fatalf("cleared fields = %v/%v, want nil", product.SizeValue, product.MeasurementUnitID)
		}
	})

	t.Run("empty size clears", func(t *testing.T) {
		store := &fakeProductStore{getResult: existingProduct()}
		service := NewProductService(store)
		product, err := service.Update(context.Background(), 7, inbound.UpdateProductInput{
			SizeValue: inbound.Optional[string]{Set: true, Value: ptr("")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if product.SizeValue != nil {
			t.Fatalf("size = %v, want nil", product.SizeValue)
		}
	})

	t.Run("money and name update rounds", func(t *testing.T) {
		store := &fakeProductStore{getResult: existingProduct()}
		service := NewProductService(store)
		price := decimal.RequireFromString("12.345")
		product, err := service.Update(context.Background(), 7, inbound.UpdateProductInput{
			Name:  inbound.Optional[string]{Set: true, Value: ptr("  New name  ")},
			Price: inbound.Optional[decimal.Decimal]{Set: true, Value: &price},
		})
		if err != nil {
			t.Fatal(err)
		}
		if product.Name != "New name" || !product.Price.Equal(decimal.RequireFromString("12.35")) {
			t.Fatalf("updated product = %+v", product)
		}
	})

	t.Run("explicit null on required fields is rejected", func(t *testing.T) {
		tests := []struct {
			name  string
			input inbound.UpdateProductInput
		}{
			{name: "name", input: inbound.UpdateProductInput{Name: inbound.Optional[string]{Set: true}}},
			{name: "sku", input: inbound.UpdateProductInput{SKU: inbound.Optional[string]{Set: true}}},
			{name: "price", input: inbound.UpdateProductInput{Price: inbound.Optional[decimal.Decimal]{Set: true}}},
			{name: "cost", input: inbound.UpdateProductInput{Cost: inbound.Optional[decimal.Decimal]{Set: true}}},
			{name: "stock", input: inbound.UpdateProductInput{StockQuantity: inbound.Optional[int]{Set: true}}},
			{name: "threshold", input: inbound.UpdateProductInput{LowStockThreshold: inbound.Optional[int]{Set: true}}},
			{name: "brand", input: inbound.UpdateProductInput{BrandID: inbound.Optional[int64]{Set: true}}},
			{name: "category", input: inbound.UpdateProductInput{CategoryID: inbound.Optional[int64]{Set: true}}},
			{name: "material", input: inbound.UpdateProductInput{MaterialID: inbound.Optional[int64]{Set: true}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := &fakeProductStore{getResult: existingProduct()}
				service := NewProductService(store)
				if _, err := service.Update(context.Background(), 7, tt.input); !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
			})
		}
	})

	t.Run("empty sku is rejected", func(t *testing.T) {
		store := &fakeProductStore{getResult: existingProduct()}
		service := NewProductService(store)
		if _, err := service.Update(context.Background(), 7, inbound.UpdateProductInput{
			SKU: inbound.Optional[string]{Set: true, Value: ptr("  ")},
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("error = %v, want ErrValidation", err)
		}
	})

	t.Run("missing product propagates", func(t *testing.T) {
		store := &fakeProductStore{getErr: domain.ErrNotFound}
		service := NewProductService(store)
		if _, err := service.Update(context.Background(), 7, inbound.UpdateProductInput{}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func TestProductListPassThrough(t *testing.T) {
	items := []*domain.ProductDetail{{Product: domain.Product{ID: 2}}, {Product: domain.Product{ID: 1}}}
	store := &fakeProductStore{listItems: items, listHasNext: true}
	service := NewProductService(store)

	result, err := service.List(context.Background(), inbound.ProductFilter{
		Skip:        5,
		Limit:       10,
		Search:      "  hammer  ",
		CategoryID:  int64Ptr(2),
		BrandID:     int64Ptr(1),
		MaterialID:  int64Ptr(3),
		StockStatus: inbound.StockStatusLowStock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.listFilter.Search != "hammer" {
		t.Errorf("search = %q, want trimmed", store.listFilter.Search)
	}
	if store.listFilter.Skip != 5 || store.listFilter.Limit != 10 || store.listFilter.StockStatus != inbound.StockStatusLowStock {
		t.Errorf("filter = %+v", store.listFilter)
	}
	if result.Skip != 5 || result.Limit != 10 || !result.HasNext || len(result.Items) != 2 {
		t.Errorf("result = %+v", result)
	}

	if _, err := service.List(context.Background(), inbound.ProductFilter{StockStatus: "bogus"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid stock_status error = %v, want ErrValidation", err)
	}
}

func TestProductStatsAndDeletePassThrough(t *testing.T) {
	stats := &domain.ProductStats{TotalProducts: 2, TotalStock: 5, LowStockCount: 1, TotalInventoryValue: decimal.RequireFromString("0.30")}
	deleted := &domain.Product{ID: 4}
	store := &fakeProductStore{stats: stats, deleteResult: deleted}
	service := NewProductService(store)

	got, err := service.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != stats {
		t.Fatalf("stats = %+v, want %+v", got, stats)
	}
	product, err := service.Delete(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if product != deleted {
		t.Fatalf("delete = %+v, want %+v", product, deleted)
	}
}

func TestProductUpdateConflictPropagates(t *testing.T) {
	store := &fakeProductStore{getResult: existingProduct(), updateErr: domain.ErrConflict}
	service := NewProductService(store)
	if _, err := service.Update(context.Background(), 7, inbound.UpdateProductInput{}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
}

func TestProductSKUFormat(t *testing.T) {
	sku, err := generateProductSKU()
	if err != nil {
		t.Fatal(err)
	}
	if len(sku) != 12 || !strings.HasPrefix(sku, "SKU-") || strings.ToUpper(sku) != sku {
		t.Fatalf("sku = %q, want SKU- plus 8 uppercase hex characters", sku)
	}
}

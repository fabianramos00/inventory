package inbound

import (
	"context"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

const (
	StockStatusInStock    = "in_stock"
	StockStatusLowStock   = "low_stock"
	StockStatusOutOfStock = "out_of_stock"
)

// Optional distinguishes an omitted field from an explicit null in partial
// updates. Set is true when the key appeared in the request.
type Optional[T any] struct {
	Set   bool
	Value *T
}

type ProductFilter struct {
	Skip        int
	Limit       int
	Search      string
	CategoryID  *int64
	BrandID     *int64
	MaterialID  *int64
	StockStatus string
}

type ProductList struct {
	Items   []*domain.ProductDetail
	HasNext bool
	Skip    int
	Limit   int
}

type CreateProductInput struct {
	Name              string
	SKU               *string
	SizeValue         *string
	MeasurementUnitID *int64
	Price             decimal.Decimal
	Cost              decimal.Decimal
	StockQuantity     Optional[int]
	LowStockThreshold Optional[int]
	BrandID           int64
	CategoryID        int64
	MaterialID        int64
}

type UpdateProductInput struct {
	Name              Optional[string]
	SKU               Optional[string]
	SizeValue         Optional[string]
	MeasurementUnitID Optional[int64]
	Price             Optional[decimal.Decimal]
	Cost              Optional[decimal.Decimal]
	StockQuantity     Optional[int]
	LowStockThreshold Optional[int]
	BrandID           Optional[int64]
	CategoryID        Optional[int64]
	MaterialID        Optional[int64]
}

type ProductUseCase interface {
	List(ctx context.Context, filter ProductFilter) (*ProductList, error)
	Get(ctx context.Context, id int64) (*domain.ProductDetail, error)
	Create(ctx context.Context, input CreateProductInput) (*domain.Product, error)
	Update(ctx context.Context, id int64, input UpdateProductInput) (*domain.Product, error)
	Delete(ctx context.Context, id int64) (*domain.Product, error)
	Stats(ctx context.Context) (*domain.ProductStats, error)
}

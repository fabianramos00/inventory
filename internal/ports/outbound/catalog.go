package outbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

// CatalogRepository is the outbound port shared by the catalog entities.
type CatalogRepository[T any] interface {
	Get(ctx context.Context, id int64) (*T, error)
	List(ctx context.Context, skip, limit int, search string) ([]*T, bool, error)
	Create(ctx context.Context, entity *T) (*T, error)
	Update(ctx context.Context, id int64, entity *T) error
	Delete(ctx context.Context, id int64) (*T, error)
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

type ProductRepository interface {
	List(ctx context.Context, filter ProductFilter) ([]*domain.ProductDetail, bool, error)
	Get(ctx context.Context, id int64) (*domain.ProductDetail, error)
	Create(ctx context.Context, product *domain.Product) (*domain.Product, error)
	Update(ctx context.Context, id int64, product *domain.Product) error
	Delete(ctx context.Context, id int64) (*domain.Product, error)
	Stats(ctx context.Context) (*domain.ProductStats, error)
}

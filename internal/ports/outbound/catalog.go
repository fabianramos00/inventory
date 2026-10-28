package outbound

import "context"

// CatalogRepository is the outbound port shared by the catalog entities.
type CatalogRepository[T any] interface {
	Get(ctx context.Context, id int64) (*T, error)
	List(ctx context.Context, skip, limit int, search string) ([]*T, bool, error)
	Create(ctx context.Context, entity *T) (*T, error)
	Update(ctx context.Context, id int64, entity *T) error
	Delete(ctx context.Context, id int64) (*T, error)
}

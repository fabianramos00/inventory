package outbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type ProviderRepository interface {
	List(ctx context.Context, skip, limit int, search string) ([]*domain.Provider, bool, error)
	Get(ctx context.Context, id int64) (*domain.Provider, error)
	Create(ctx context.Context, provider *domain.Provider) (*domain.Provider, error)
	Update(ctx context.Context, id int64, provider *domain.Provider) error
	Delete(ctx context.Context, id int64) (*domain.Provider, error)
}

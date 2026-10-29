package outbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type ClientRepository interface {
	List(ctx context.Context, skip, limit int, search string) ([]*domain.Client, bool, error)
	Get(ctx context.Context, id int64) (*domain.Client, error)
	Create(ctx context.Context, client *domain.Client) (*domain.Client, error)
	Update(ctx context.Context, id int64, client *domain.Client) error
	Delete(ctx context.Context, id int64) (*domain.Client, error)
}

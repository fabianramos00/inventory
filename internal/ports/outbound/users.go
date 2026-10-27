package outbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

// UserRepository is the outbound port for user persistence.
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	List(ctx context.Context, skip, limit int, search string) ([]*domain.User, bool, error)
	Create(ctx context.Context, user *domain.User) error
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id int64) (*domain.User, error)
}

package inbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type ProviderFilter struct {
	Skip   int
	Limit  int
	Search string
}

type ProviderList struct {
	Items   []*domain.Provider
	HasNext bool
	Skip    int
	Limit   int
}

type CreateProviderInput struct {
	Name        string
	ContactInfo *string
	Email       *string
	Phone       *string
	CreatedByID *int64
}

type UpdateProviderInput struct {
	Name        Optional[string]
	ContactInfo Optional[string]
	Email       Optional[string]
	Phone       Optional[string]
}

type ProviderUseCase interface {
	List(ctx context.Context, filter ProviderFilter) (*ProviderList, error)
	Get(ctx context.Context, id int64) (*domain.Provider, error)
	Create(ctx context.Context, input CreateProviderInput) (*domain.Provider, error)
	Update(ctx context.Context, id int64, input UpdateProviderInput) (*domain.Provider, error)
	Delete(ctx context.Context, id int64) (*domain.Provider, error)
}

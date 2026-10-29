package inbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type ClientFilter struct {
	Skip   int
	Limit  int
	Search string
}

type ClientList struct {
	Items   []*domain.Client
	HasNext bool
	Skip    int
	Limit   int
}

type CreateClientInput struct {
	Name         string
	IdentityCard *string
	Email        *string
	Phone        *string
}

type UpdateClientInput struct {
	Name         Optional[string]
	IdentityCard Optional[string]
	Email        Optional[string]
	Phone        Optional[string]
}

type ClientUseCase interface {
	List(ctx context.Context, filter ClientFilter) (*ClientList, error)
	Get(ctx context.Context, id int64) (*domain.Client, error)
	Create(ctx context.Context, input CreateClientInput) (*domain.Client, error)
	Update(ctx context.Context, id int64, input UpdateClientInput) (*domain.Client, error)
	Delete(ctx context.Context, id int64) (*domain.Client, error)
}

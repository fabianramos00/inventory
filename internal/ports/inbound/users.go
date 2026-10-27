package inbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type UserList struct {
	Items   []*domain.User
	HasNext bool
	Skip    int
	Limit   int
}

type CreateUserInput struct {
	Email       string
	Password    string
	FullName    string
	IsActive    *bool
	IsSuperuser *bool
}

type UpdateUserInput struct {
	Email       *string
	Password    *string
	FullName    *string
	IsActive    *bool
	IsSuperuser *bool
}

// UserUseCase is the inbound port implemented by the user application service.
type UserUseCase interface {
	List(ctx context.Context, skip, limit int, search string) (*UserList, error)
	Get(ctx context.Context, id int64) (*domain.User, error)
	Create(ctx context.Context, input CreateUserInput) (*domain.User, error)
	Update(ctx context.Context, id int64, input UpdateUserInput) (*domain.User, error)
	Delete(ctx context.Context, id int64) (*domain.User, error)
}

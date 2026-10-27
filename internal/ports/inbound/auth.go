package inbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type LoginResult struct {
	AccessToken  string
	RefreshToken string
	User         *domain.User
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// AuthUseCase is the inbound port implemented by the auth application service.
type AuthUseCase interface {
	Login(ctx context.Context, email, password string) (*LoginResult, error)
	Refresh(ctx context.Context, rawToken string) (*TokenPair, error)
	Logout(ctx context.Context, rawToken string) error
	AuthenticateAccessToken(ctx context.Context, rawToken string) (*domain.User, error)
}

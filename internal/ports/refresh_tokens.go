package ports

import (
	"context"
	"time"

	"inventory-desktop/internal/domain"
)

type RefreshTokenRepository interface {
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	FindValid(ctx context.Context, tokenHash string, now time.Time) (*domain.RefreshToken, error)
	Rotate(ctx context.Context, oldHash, newHash string, now, expiresAt time.Time) error
	Revoke(ctx context.Context, tokenHash string) error
}

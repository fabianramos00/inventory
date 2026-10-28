package outbound

import (
	"context"
	"time"
)

// RefreshTokenRepository is the outbound port for refresh-token persistence.
type RefreshTokenRepository interface {
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	FindValid(ctx context.Context, tokenHash string, now time.Time) (int64, error)
	Rotate(ctx context.Context, oldHash, newHash string, now, expiresAt time.Time) error
	Revoke(ctx context.Context, tokenHash string) error
}

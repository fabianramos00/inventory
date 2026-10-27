package gormstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

var _ outbound.RefreshTokenRepository = (*RefreshTokenStore)(nil)

// RefreshTokenStore is the GORM implementation of outbound.RefreshTokenRepository.
type RefreshTokenStore struct {
	db *gorm.DB
}

func NewRefreshTokenStore(db *gorm.DB) *RefreshTokenStore {
	return &RefreshTokenStore{db: db}
}

func (r *RefreshTokenStore) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	record := refreshTokenModel{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: Timestamp(expiresAt),
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokenStore) FindValid(ctx context.Context, tokenHash string, now time.Time) (*domain.RefreshToken, error) {
	var record refreshTokenModel
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND revoked = ? AND expires_at > ?", tokenHash, false, Timestamp(now)).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	return &domain.RefreshToken{
		ID:        record.ID,
		UserID:    record.UserID,
		TokenHash: record.TokenHash,
		ExpiresAt: record.ExpiresAt.Time(),
		Revoked:   record.Revoked,
	}, nil
}

func (r *RefreshTokenStore) Rotate(ctx context.Context, oldHash, newHash string, now, expiresAt time.Time) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var userID int64
		row := tx.Raw(`
			UPDATE refresh_tokens SET revoked = ?
			WHERE token_hash = ? AND revoked = ? AND expires_at > ?
			RETURNING user_id`,
			true, oldHash, false, Timestamp(now),
		).Row()
		if err := row.Scan(&userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("revoke rotated refresh token: %w", err)
		}

		record := refreshTokenModel{
			UserID:    userID,
			TokenHash: newHash,
			ExpiresAt: Timestamp(expiresAt),
		}
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("create rotated refresh token: %w", err)
		}
		return nil
	})
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokenStore) Revoke(ctx context.Context, tokenHash string) error {
	result := r.db.WithContext(ctx).Model(&refreshTokenModel{}).
		Where("token_hash = ? AND revoked = ?", tokenHash, false).
		Update("revoked", true)
	if result.Error != nil {
		return fmt.Errorf("revoke refresh token: %w", result.Error)
	}
	return nil
}

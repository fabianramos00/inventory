package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports"
)

var _ ports.RefreshTokenRepository = (*RefreshTokenRepository)(nil)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked)
		VALUES (?, ?, ?, 0)`,
		userID, tokenHash, formatTimestamp(expiresAt),
	)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) FindValid(ctx context.Context, tokenHash string, now time.Time) (*domain.RefreshToken, error) {
	var token domain.RefreshToken
	var expiresAt string
	var revoked int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked
		FROM refresh_tokens
		WHERE token_hash = ? AND revoked = 0 AND expires_at > ?`,
		tokenHash, formatTimestamp(now),
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &expiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	parsed, err := parseTimestamp(expiresAt)
	if err != nil {
		return nil, err
	}
	token.ExpiresAt = parsed
	token.Revoked = revoked != 0
	return &token, nil
}

func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldHash, newHash string, now, expiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE refresh_tokens SET revoked = 1
		WHERE token_hash = ? AND revoked = 0 AND expires_at > ?
		RETURNING user_id`,
		oldHash, formatTimestamp(now),
	).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke rotated refresh token: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked)
		VALUES (?, ?, ?, 0)`,
		userID, newHash, formatTimestamp(expiresAt),
	); err != nil {
		return fmt.Errorf("create rotated refresh token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit refresh rotation: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked = 1
		WHERE token_hash = ? AND revoked = 0`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

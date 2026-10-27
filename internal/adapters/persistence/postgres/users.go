package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports"
)

const userColumns = `id, COALESCE(full_name, ''), email, hashed_password, COALESCE(is_active, true), COALESCE(is_superuser, false)`

var _ ports.UserRepository = (*UserRepository)(nil)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email)
	return scanUser(row)
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (full_name, email, hashed_password, is_active, is_superuser)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		user.FullName, user.Email, user.HashedPassword, user.IsActive, user.IsSuperuser,
	).Scan(&user.ID)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users
		SET full_name = $2, email = $3, hashed_password = $4, is_active = $5, is_superuser = $6
		WHERE id = $1`,
		user.ID, user.FullName, user.Email, user.HashedPassword, user.IsActive, user.IsSuperuser,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanUser(row pgx.Row) (*domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.FullName, &user.Email, &user.HashedPassword, &user.IsActive, &user.IsSuperuser)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &user, nil
}

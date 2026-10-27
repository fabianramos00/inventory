package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports"
)

const userColumns = `id, COALESCE(full_name, ''), email, hashed_password, is_active, is_superuser`

var _ ports.UserRepository = (*UserRepository)(nil)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO users (full_name, email, hashed_password, is_active, is_superuser)
		VALUES (?, ?, ?, ?, ?)`,
		user.FullName, user.Email, user.HashedPassword, boolToInt(user.IsActive), boolToInt(user.IsSuperuser),
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read created user id: %w", err)
	}
	user.ID = id
	return nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET full_name = ?, email = ?, hashed_password = ?, is_active = ?, is_superuser = ?
		WHERE id = ?`,
		user.FullName, user.Email, user.HashedPassword, boolToInt(user.IsActive), boolToInt(user.IsSuperuser), user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated user count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var user domain.User
	var isActive, isSuperuser int64
	err := row.Scan(&user.ID, &user.FullName, &user.Email, &user.HashedPassword, &isActive, &isSuperuser)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	user.IsActive = isActive != 0
	user.IsSuperuser = isSuperuser != 0
	return &user, nil
}

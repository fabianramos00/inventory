package gormstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

var _ outbound.UserRepository = (*UserStore)(nil)

// UserStore is the GORM implementation of outbound.UserRepository.
type UserStore struct {
	db *gorm.DB
}

func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

func (r *UserStore) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	var record userModel
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	return scanUser(record, err)
}

func (r *UserStore) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var record userModel
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&record).Error
	return scanUser(record, err)
}

func (r *UserStore) List(ctx context.Context, skip, limit int, search string) ([]*domain.User, bool, error) {
	query := r.db.WithContext(ctx).Model(&userModel{})
	if search = strings.ToLower(strings.TrimSpace(search)); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("LOWER(full_name) LIKE ? OR LOWER(email) LIKE ?", pattern, pattern)
	}
	var records []userModel
	if err := query.Order("id").Offset(skip).Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, false, userStoreError("list users", err)
	}
	hasNext := len(records) > limit
	if hasNext {
		records = records[:limit]
	}
	users := make([]*domain.User, len(records))
	for i := range records {
		users[i] = records[i].toDomain()
	}
	return users, hasNext, nil
}

func (r *UserStore) Create(ctx context.Context, user *domain.User) error {
	isActive := user.IsActive
	isSuperuser := user.IsSuperuser
	record := userModel{
		FullName:       &user.FullName,
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
		IsActive:       &isActive,
		IsSuperuser:    &isSuperuser,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return userStoreError("create user", err)
	}
	user.ID = record.ID
	return nil
}

func (r *UserStore) Update(ctx context.Context, user *domain.User) error {
	result := r.db.WithContext(ctx).Model(&userModel{}).Where("id = ?", user.ID).Updates(map[string]any{
		"full_name":       user.FullName,
		"email":           user.Email,
		"hashed_password": user.HashedPassword,
		"is_active":       user.IsActive,
		"is_superuser":    user.IsSuperuser,
	})
	if result.Error != nil {
		return userStoreError("update user", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserStore) Delete(ctx context.Context, id int64) (*domain.User, error) {
	var record userModel
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, userStoreError("load user for delete", err)
	}
	result := r.db.WithContext(ctx).Delete(&record)
	if result.Error != nil {
		return nil, userStoreError("delete user", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return record.toDomain(), nil
}

func userStoreError(op string, err error) error {
	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return domain.ErrConflict
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return domain.ErrForeignKeyViolation
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

func scanUser(record userModel, err error) (*domain.User, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return record.toDomain(), nil
}

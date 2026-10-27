package gormstore

import (
	"context"
	"errors"
	"fmt"

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
		return fmt.Errorf("create user: %w", err)
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
		return fmt.Errorf("update user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
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

package gormstore

import "inventory-desktop/internal/domain"

type userModel struct {
	ID             int64   `gorm:"column:id;primaryKey;autoIncrement"`
	FullName       *string `gorm:"column:full_name"`
	Email          string  `gorm:"column:email"`
	HashedPassword string  `gorm:"column:hashed_password"`
	IsActive       *bool   `gorm:"column:is_active"`
	IsSuperuser    *bool   `gorm:"column:is_superuser"`
}

func (userModel) TableName() string {
	return "users"
}

func (m userModel) toDomain() *domain.User {
	fullName := ""
	if m.FullName != nil {
		fullName = *m.FullName
	}
	isActive := true
	if m.IsActive != nil {
		isActive = *m.IsActive
	}
	isSuperuser := false
	if m.IsSuperuser != nil {
		isSuperuser = *m.IsSuperuser
	}
	return &domain.User{
		ID:             m.ID,
		FullName:       fullName,
		Email:          m.Email,
		HashedPassword: m.HashedPassword,
		IsActive:       isActive,
		IsSuperuser:    isSuperuser,
	}
}

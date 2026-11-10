package httpapi

import "inventory-desktop/internal/domain"

type userResponse struct {
	ID          int64  `json:"id"`
	FullName    string `json:"full_name"`
	Email       string `json:"email"`
	IsActive    bool   `json:"is_active"`
	IsSuperuser bool   `json:"is_superuser"`
}

type userOptionResponse struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

func newUserOptionResponse(option *domain.UserOption) *userOptionResponse {
	return &userOptionResponse{ID: option.ID, FullName: option.FullName}
}

func newUserResponse(user *domain.User) *userResponse {
	return &userResponse{
		ID:          user.ID,
		FullName:    user.FullName,
		Email:       user.Email,
		IsActive:    user.IsActive,
		IsSuperuser: user.IsSuperuser,
	}
}

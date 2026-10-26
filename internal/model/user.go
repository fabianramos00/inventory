package model

import "time"

type User struct {
	ID             int64  `json:"id"`
	FullName       string `json:"full_name"`
	Email          string `json:"email"`
	HashedPassword string `json:"-"`
	IsActive       bool   `json:"is_active"`
	IsSuperuser    bool   `json:"is_superuser"`
}

type RefreshToken struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

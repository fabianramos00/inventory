package domain

import "time"

type User struct {
	ID             int64
	FullName       string
	Email          string
	HashedPassword string
	IsActive       bool
	IsSuperuser    bool
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	Revoked   bool
}

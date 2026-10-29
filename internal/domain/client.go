package domain

import "time"

type Client struct {
	ID           int64
	Name         string
	IdentityCard *string
	Email        *string
	Phone        *string
	CreatedAt    time.Time
	UpdatedAt    *time.Time
}

package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type Client struct {
	ID           int64
	Name         string
	IdentityCard *string
	Email        *string
	Phone        *string
	DebtAmount   decimal.Decimal
	CreatedAt    time.Time
	UpdatedAt    *time.Time
}

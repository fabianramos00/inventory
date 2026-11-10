package inbound

import (
	"context"

	"github.com/shopspring/decimal"
)

type CreateClientPaymentInput struct {
	ClientID      int64
	Amount        decimal.Decimal
	PaymentMethod string
	CreatedByID   int64
}

type ClientPaymentAllocation struct {
	SaleID int64
	Amount decimal.Decimal
}

type ClientPaymentResult struct {
	ClientID      int64
	Amount        decimal.Decimal
	RemainingDebt decimal.Decimal
	Allocations   []ClientPaymentAllocation
}

type ClientPaymentUseCase interface {
	CreateClientPayment(ctx context.Context, input CreateClientPaymentInput) (*ClientPaymentResult, error)
}

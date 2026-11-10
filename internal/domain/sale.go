package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	PaymentStatusPending = "pending"
	PaymentStatusPartial = "partial"
	PaymentStatusPaid    = "paid"

	DeliveryStatusPending   = "pending"
	DeliveryStatusPartial   = "partial"
	DeliveryStatusDelivered = "delivered"

	PaymentMethodCash   = "cash"
	PaymentMethodCredit = "credit"
	PaymentMethodDebit  = "debit"

	// MaxMoneyAmount is the numeric(12,2) ceiling, shared by request validation.
	MaxMoneyAmount = "9999999999.99"
)

type Sale struct {
	ID             int64
	CreatedAt      time.Time
	UpdatedAt      *time.Time
	TotalAmount    decimal.Decimal
	AmountPaid     decimal.Decimal
	DebtAmount     decimal.Decimal
	PaymentStatus  string
	DeliveryStatus string
	ClientID       *int64
	CreatedByID    int64
	CreatedBy      *User
	Client         *Client
}

type SalePayment struct {
	ID            int64
	SaleID        int64
	Amount        decimal.Decimal
	PaymentMethod *string
	CreatedAt     time.Time
	CreatedByID   *int64
	CreatedBy     *User
}

type SaleItem struct {
	ID                int64
	SaleID            int64
	ProductID         int64
	Quantity          int
	DeliveredQuantity int
	UnitPrice         decimal.Decimal
	Subtotal          decimal.Decimal
	ProductName       string
	ProductSKU        string
	BrandName         *string
	MaterialName      *string
	CategoryName      *string
}

type SaleStats struct {
	UnpaidCount      int64
	UndeliveredCount int64
	TotalAmountSum   decimal.Decimal
	AmountPaidSum    decimal.Decimal
}

func ValidPaymentStatus(value string) bool {
	switch value {
	case "", PaymentStatusPending, PaymentStatusPartial, PaymentStatusPaid:
		return true
	default:
		return false
	}
}

func ValidDeliveryStatus(value string) bool {
	switch value {
	case "", DeliveryStatusPending, DeliveryStatusPartial, DeliveryStatusDelivered:
		return true
	default:
		return false
	}
}

func ValidPaymentMethod(value string) bool {
	switch value {
	case PaymentMethodCash, PaymentMethodCredit, PaymentMethodDebit:
		return true
	default:
		return false
	}
}

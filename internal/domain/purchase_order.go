package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	OrderStatusPending   = "pending"
	OrderStatusSent      = "sent"
	OrderStatusReceived  = "received"
	OrderStatusCancelled = "cancelled"

	PurchasePaymentStatusPending = "pending"
	PurchasePaymentStatusPaid    = "paid"
)

type PurchaseOrder struct {
	ID            int64
	ProviderID    int64
	Status        string
	PaymentStatus string
	OrderDate     time.Time
	TotalAmount   decimal.Decimal
	CreatedByID   int64
	Provider      *Provider
}

type OrderItem struct {
	ID           int64
	OrderID      int64
	ProductID    int64
	Quantity     int
	UnitCost     decimal.Decimal
	Subtotal     decimal.Decimal
	SupplierSKU  *string
	ProductName  string
	ProductSKU   string
	BrandName    *string
	MaterialName *string
	CategoryName *string
}

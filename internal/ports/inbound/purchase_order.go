package inbound

import (
	"context"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type PurchaseOrderFilter = outbound.PurchaseOrderFilter

type PurchaseOrderList struct {
	Items   []*domain.PurchaseOrder
	HasNext bool
	Skip    int
	Limit   int
}

type OrderItemList struct {
	Items   []*domain.OrderItem
	HasNext bool
	Skip    int
	Limit   int
}

type CreateOrderItemInput struct {
	ProductID   int64
	Quantity    int
	UnitCost    decimal.Decimal
	SupplierSKU *string
}

type CreatePurchaseOrderInput struct {
	ProviderID    int64
	Status        string
	PaymentStatus string
	Items         []CreateOrderItemInput
	CreatedByID   int64
}

type UpdatePurchaseOrderInput struct {
	ProviderID    Optional[int64]
	Status        Optional[string]
	PaymentStatus Optional[string]
}

type AddOrderItemInput struct {
	ProductID   int64
	Quantity    int
	UnitCost    decimal.Decimal
	SupplierSKU *string
}

type UpdateOrderItemInput struct {
	Quantity    Optional[int]
	UnitCost    Optional[decimal.Decimal]
	SupplierSKU Optional[string]
}

type PurchaseOrderUseCase interface {
	List(ctx context.Context, filter PurchaseOrderFilter) (*PurchaseOrderList, error)
	Create(ctx context.Context, input CreatePurchaseOrderInput) (*domain.PurchaseOrder, error)
	Get(ctx context.Context, id int64) (*domain.PurchaseOrder, error)
	Update(ctx context.Context, id int64, input UpdatePurchaseOrderInput) (*domain.PurchaseOrder, error)
	Delete(ctx context.Context, id int64) (*domain.PurchaseOrder, error)
	ListItems(ctx context.Context, orderID int64, skip, limit int) (*OrderItemList, error)
	AddItem(ctx context.Context, orderID int64, input AddOrderItemInput) (*domain.PurchaseOrder, error)
	UpdateItem(ctx context.Context, orderID, itemID int64, input UpdateOrderItemInput) (*domain.PurchaseOrder, error)
	DeleteItem(ctx context.Context, orderID, itemID int64) (*domain.PurchaseOrder, error)
}

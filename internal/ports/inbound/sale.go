package inbound

import (
	"context"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type SaleFilter = outbound.SaleFilter

type SaleList struct {
	Items   []*domain.Sale
	HasNext bool
	Skip    int
	Limit   int
}

type SaleItemList struct {
	Items   []*domain.SaleItem
	HasNext bool
	Skip    int
	Limit   int
}

type CreateSaleItemInput struct {
	ProductID         int64
	Quantity          int
	DeliveredQuantity int
	UnitPrice         *decimal.Decimal
}

type CreateSaleInput struct {
	PaymentMethod *string
	AmountPaid    decimal.Decimal
	ClientID      *int64
	Items         []CreateSaleItemInput
	CreatedByID   int64
}

type UpdateSaleInput struct {
	PaymentMethod Optional[string]
	AmountPaid    Optional[decimal.Decimal]
	ClientID      Optional[int64]
}

type AddSaleItemInput struct {
	ProductID         int64
	Quantity          int
	DeliveredQuantity int
	UnitPrice         *decimal.Decimal
}

type UpdateSaleItemInput struct {
	Quantity          Optional[int]
	DeliveredQuantity Optional[int]
	UnitPrice         Optional[decimal.Decimal]
}

type SaleUseCase interface {
	List(ctx context.Context, filter SaleFilter) (*SaleList, error)
	Create(ctx context.Context, input CreateSaleInput) (*domain.Sale, error)
	Stats(ctx context.Context) (*domain.SaleStats, error)
	Get(ctx context.Context, id int64) (*domain.Sale, error)
	Update(ctx context.Context, id int64, input UpdateSaleInput) (*domain.Sale, error)
	Delete(ctx context.Context, id int64) (*domain.Sale, error)
	ListItems(ctx context.Context, saleID int64, skip, limit int) (*SaleItemList, error)
	AddItem(ctx context.Context, saleID int64, input AddSaleItemInput) (*domain.Sale, error)
	UpdateItem(ctx context.Context, saleID, itemID int64, input UpdateSaleItemInput) (*domain.Sale, error)
	DeleteItem(ctx context.Context, saleID, itemID int64) (*domain.Sale, error)
}
